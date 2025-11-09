package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"go-llm-server/internal/config"
	stor "go-llm-server/internal/storage"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/logger"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"sync"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Handler 代理处理器
type Handler struct {
	cfg        *config.Config
	lbManager  *LoadBalancerManager
	strategies []URLRouteStrategy
	proxy      *httputil.ReverseProxy
	storage    *stor.Storage

	ipLimiters sync.Map // map[string]*rate.Limiter
}

type cacheContextKey struct{}

var llmCacheContextKey = cacheContextKey{}

type llmCacheMetadata struct {
	prompt      string
	model       string
	temperature *float32
	maxTokens   *int
	stream      bool
}

// NewHandler 创建新的代理处理器，并初始化 ReverseProxy 实例
func NewHandler(cfg *config.Config) *Handler {
	manager := NewLoadBalancerManager()
	var storageInstance *stor.Storage
	if cfg != nil {
		if s, err := stor.NewStorage(cfg); err != nil {
			logger.Warn("Failed to initialize storage, LLM cache disabled", zap.Error(err))
		} else {
			storageInstance = s
		}
	}
	h := &Handler{
		cfg:       cfg,
		lbManager: manager,
		strategies: []URLRouteStrategy{
			NewModelSpecifyStrategy(manager),
			NewDefaultStrategy(),
		},
		storage: storageInstance,
	}

	// 构造单例 ReverseProxy
	h.proxy = &httputil.ReverseProxy{
		Director:     h.director,
		ErrorHandler: h.errorHandler,
		Transport:    &TransportWithProxyAutoDetected{},
		ModifyResponse: func(resp *http.Response) error {
			return h.modifyResponse(resp)
		},
	}

	return h
}

// InitLoadBalancers 初始化负载均衡器
func (h *Handler) InitLoadBalancers() {
	for model := range h.cfg.ModelRoutes {
		if urls, exists := h.cfg.GetModelURLs(model); exists {
			h.lbManager.AddLoadBalancer(model, urls)
			logger.Info("Initialized load balancer for model",
				zap.String("model", model),
				zap.Strings("urls", urls))
		}
	}
}

// director 为 ReverseProxy 设置目标请求
func (h *Handler) director(request *http.Request) {
	targetURL, ok := h.getTargetURL(request)
	if !ok {
		// 如果没有匹配到路径，就不修改 URL，后续会返回 404
		return
	}

	request.URL = targetURL
	request.Host = targetURL.Host
}

// errorHandler 处理代理错误
func (h *Handler) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	logger.Error("Proxy error", zap.String("requestId", utils.GetRequestID(r)), zap.Error(err))
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

// getTargetURL 按策略选取后端 URL
func (h *Handler) getTargetURL(request *http.Request) (*url.URL, bool) {
	path := request.URL.Path
	base := h.cfg.TargetMap[path]
	for _, strategy := range h.strategies {
		if strategy.ShouldApply(path) {
			target, err := strategy.GetTargetURL(request, base)
			if err != nil {
				logger.Error("Strategy failed to get target URL",
					zap.String("requestId", utils.GetRequestID(request)),
					zap.String("path", path),
					zap.Error(err),
				)
				return nil, false
			}
			return target, true
		}
	}

	return nil, false
}

// getIPLimiter 获取或创建指定IP的限流器
func (h *Handler) getIPLimiter(ip string) *rate.Limiter {
	if !h.cfg.HasRateLimit() {
		return nil
	}
	v, ok := h.ipLimiters.Load(ip)
	if ok {
		return v.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(rate.Limit(h.cfg.RateLimit.Rate), h.cfg.RateLimit.Burst)
	h.ipLimiters.Store(ip, limiter)
	return limiter
}

// ServeHTTP 处理 HTTP 请求，复用已初始化的 ReverseProxy
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 注入请求 ID
	requestId := utils.GetOrGenerateRequestID(r)

	clientIP := utils.GetClientIP(r)
	limiter := h.getIPLimiter(clientIP)
	if limiter != nil && !limiter.Allow() {
		logger.Warn("Rate limit exceeded", zap.String("clientIp", clientIP), zap.String("requestId", requestId))
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	logFields := []zap.Field{
		zap.String("requestId", requestId),
		zap.String("clientIp", clientIP),
		zap.String("path", r.URL.Path),
		zap.String("method", r.Method),
		zap.String("targetUrl", r.URL.String()),
		zap.Int("Content-length", int(r.ContentLength)),
	}

	var requestBodyBuf bytes.Buffer
	if h.cfg.LogBody && r.Body != nil {
		r.Body = newTeeReadCloser(r.Body, &requestBodyBuf)
	}

	logger.Info("Request received", logFields...)

	// 校验路径
	if _, ok := h.cfg.TargetMap[r.URL.Path]; !ok {
		logger.Warn("Path not found, returning 404",
			zap.String("requestId", requestId),
			zap.String("path", r.URL.Path),
			zap.String("method", r.Method),
		)
		http.NotFound(w, r)
		return
	}

	if h.shouldUseLLMCache(r) {
		handled, meta := h.handleLLMCachePreProxy(w, r)
		if handled {
			return
		}
		if meta != nil {
			r = r.WithContext(context.WithValue(r.Context(), llmCacheContextKey, meta))
		}
	}

	var responseBodyBuf bytes.Buffer
	multiWriter := io.MultiWriter(w, &responseBodyBuf)
	w = &teeResponseWriter{ResponseWriter: w, writer: multiWriter}

	// 交给同一个 ReverseProxy 实例处理
	h.proxy.ServeHTTP(w, r)

	if h.cfg.LogBody {
		logFields = append(logFields, zap.String("requestBody", requestBodyBuf.String()))
		logger.Info("Request received", logFields...)
		logger.Info("Response received", zap.String("Content-Type", w.Header().Get("Content-Type")),
			zap.String("Content-Encoding", w.Header().Get("Content-Encoding")),
			zap.String("requestBody", responseBodyBuf.String()))
	}

}

func (h *Handler) shouldUseLLMCache(r *http.Request) bool {
	return h.storage != nil && r != nil && r.URL.Path == "/chat/completions" && r.Method == http.MethodPost
}

func (h *Handler) handleLLMCachePreProxy(w http.ResponseWriter, r *http.Request) (bool, *llmCacheMetadata) {
	if r == nil || r.Body == nil {
		return false, nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Warn("Failed to read request body for LLM cache lookup",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.Error(err))
		r.Body = io.NopCloser(bytes.NewReader(nil))
		return false, nil
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if len(bodyBytes) == 0 {
		return false, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		logger.Warn("Failed to unmarshal request body for LLM cache lookup",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.Error(err))
	}

	streamBool := false
	if streamVal, ok := payload["stream"]; ok {
		if v, ok := streamVal.(bool); ok {
			streamBool = v
		} else {
			return false, nil
		}
	}
	if streamBool {
		return false, nil
	}

	model, _ := payload["model"].(string)
	if model == "" {
		return false, nil
	}

	var temperature *float32
	if v, ok := payload["temperature"]; ok {
		switch t := v.(type) {
		case float64:
			temp := float32(t)
			temperature = &temp
		case float32:
			temp := t
			temperature = &temp
		}
	}

	var maxTokens *int
	if v, ok := payload["max_tokens"]; ok {
		switch t := v.(type) {
		case float64:
			mt := int(t)
			maxTokens = &mt
		case float32:
			mt := int(t)
			maxTokens = &mt
		case int:
			mt := t
			maxTokens = &mt
		}
	}

	prompt := string(bodyBytes)
	rec, err := h.storage.GetLLM(r.Context(), prompt, model, temperature, maxTokens)
	if err != nil {
		logger.Warn("LLM cache lookup failed",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.String("model", model),
			zap.Error(err))
	} else if rec != nil && rec.Response != "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-LLM-Cache", "HIT")
		_, _ = w.Write([]byte(rec.Response))
		logger.Info("Served response from LLM cache",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.String("model", model))
		return true, nil
	}

	return false, &llmCacheMetadata{
		prompt:      prompt,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		stream:      false,
	}
}

func (h *Handler) modifyResponse(resp *http.Response) error {
	if h.storage == nil || resp == nil || resp.Request == nil {
		return nil
	}

	meta, _ := resp.Request.Context().Value(llmCacheContextKey).(*llmCacheMetadata)
	if meta == nil || meta.stream {
		return nil
	}

	resp.Header.Set("X-LLM-Cache", "MISS")

	if resp.StatusCode != http.StatusOK || resp.Body == nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Warn("Failed to read response body for LLM cache storage",
			zap.Error(err))
		return nil
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if len(bodyBytes) > 0 {
		resp.ContentLength = int64(len(bodyBytes))
		resp.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	} else {
		resp.ContentLength = 0
		resp.Header.Del("Content-Length")
	}

	bodyToStore := bodyBytes
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" {
		if strings.Contains(strings.ToLower(encoding), "gzip") {
			gr, err := gzip.NewReader(bytes.NewReader(bodyBytes))
			if err != nil {
				logger.Warn("Failed to decompress gzip response for LLM cache",
					zap.String("model", meta.model),
					zap.Error(err))
			} else {
				var decompressed bytes.Buffer
				if _, err := io.Copy(&decompressed, gr); err != nil {
					logger.Warn("Failed to copy decompressed response for LLM cache",
						zap.String("model", meta.model),
						zap.Error(err))
				} else {
					bodyToStore = decompressed.Bytes()
				}
				_ = gr.Close()
			}
		}
	}

	if !utf8.Valid(bodyToStore) {
		logger.Warn("LLM response is not valid UTF-8, skipping cache",
			zap.String("model", meta.model))
		return nil
	}

	var tokensUsedPtr *int
	var responsePayload struct {
		Usage *struct {
			TotalTokens *int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(bodyToStore, &responsePayload); err == nil {
		if responsePayload.Usage != nil && responsePayload.Usage.TotalTokens != nil {
			total := *responsePayload.Usage.TotalTokens
			tokensUsedPtr = &total
		}
	}

	if err := h.storage.UpsertLLM(resp.Request.Context(), meta.prompt, meta.model, meta.temperature, meta.maxTokens, string(bodyToStore), tokensUsedPtr); err != nil {
		logger.Warn("Failed to store response in LLM cache",
			zap.String("model", meta.model),
			zap.Error(err))
	} else {
		resp.Header.Set("X-LLM-Cache", "MISS")
		logger.Info("Stored response in LLM cache",
			zap.String("model", meta.model))
	}

	return nil
}
