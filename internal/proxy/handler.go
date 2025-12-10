package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"go-llm-server/internal/config"
	stor "go-llm-server/internal/storage"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/db"
	"go-llm-server/pkg/logger"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fmt"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ensureJSONFormat 确保输入是有效的 JSON 格式，如果不是则尝试解析或包装
func ensureJSONFormat(input string) (json.RawMessage, error) {
	// 尝试解析为 JSON，如果成功直接返回
	var jsonVal interface{}
	if err := json.Unmarshal([]byte(input), &jsonVal); err == nil {
		// 是有效的 JSON，直接返回
		return json.RawMessage(input), nil
	}
	// 如果不是有效的 JSON，将其包装为 JSON 字符串
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(jsonBytes), nil
}

type cacheStorage interface {
	GetEmbedding(ctx context.Context, inputText, modelName string) (*db.EmbeddingRecord, error)
	UpsertEmbedding(ctx context.Context, rec *db.EmbeddingRecord) error
	GetLLM(ctx context.Context, request, modelName string) (*db.LLMRecord, error)
	UpsertLLM(ctx context.Context, rec *db.LLMRecord) error
}

// Handler 代理处理器
type Handler struct {
	cfg        *config.Config
	lbManager  *LoadBalancerManager
	strategies []URLRouteStrategy
	proxy      *httputil.ReverseProxy
	storage    cacheStorage

	ipLimiters sync.Map // map[string]*rate.Limiter
}

type cacheContextKey struct{}

var llmCacheContextKey = cacheContextKey{}
var embeddingCacheContextKey = cacheContextKey{}

const llmCacheBypassHeader = "X-LLM-Cache-Bypass"

type llmCacheMetadata struct {
	prompt      string
	model       string
	temperature *float32
	maxTokens   *int
	stream      bool
	startTime   time.Time
	requestID   string
}

// NewHandler 创建新的代理处理器，并初始化 ReverseProxy 实例
func NewHandler(cfg *config.Config) *Handler {
	manager := NewLoadBalancerManager()
	var storageInstance cacheStorage
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

	// 创建一个新的 context，与客户端断开连接时不会立即取消
	// 使用 900 秒超时，与 transport 的 ResponseHeaderTimeout 保持一致
	// 这样可以确保代理请求不会因为客户端断开而立即取消
	ctx := request.Context()
	newCtx, _ := context.WithTimeout(context.Background(), 900*time.Second)

	// 如果原 context 中有 LLM cache metadata，保留它
	if meta := ctx.Value(llmCacheContextKey); meta != nil {
		newCtx = context.WithValue(newCtx, llmCacheContextKey, meta)
	}

	// 更新请求的 context，使其不受客户端断开影响
	*request = *request.WithContext(newCtx)

	request.URL = targetURL
	request.Host = targetURL.Host
}

// errorHandler 处理代理错误
func (h *Handler) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	requestId := utils.GetRequestID(r)

	// 检查是否是上下文取消错误（使用 errors.Is 以处理可能的错误包装）
	if errors.Is(err, context.Canceled) {
		// 记录为警告，因为客户端断开连接或请求超时是常见情况
		// 这通常发生在客户端断开连接或请求超时时
		logger.Warn("Proxy request context canceled",
			zap.String("requestId", requestId),
			zap.Error(err),
			zap.String("errorType", "context_canceled"))
	} else if errors.Is(err, context.DeadlineExceeded) {
		logger.Warn("Proxy request timeout",
			zap.String("requestId", requestId),
			zap.Error(err),
			zap.String("errorType", "context_timeout"))
	} else {
		logger.Error("Proxy error",
			zap.String("requestId", requestId),
			zap.Error(err))
	}

	// 尝试发送错误响应（如果响应还未发送，httputil 会尝试发送）
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

	if h.shouldUseEmbeddingCache(r) {
		handled, meta := h.handleEmbeddingCachePreProxy(w, r)
		if handled {
			return
		}
		if meta != nil {
			r = r.WithContext(context.WithValue(r.Context(), embeddingCacheContextKey, meta))
		}
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
	if h.storage == nil || r == nil {
		return false
	}

	if r.Header.Get(llmCacheBypassHeader) != "" {
		return false
	}

	return r.URL.Path == "/chat/completions" && r.Method == http.MethodPost
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

	request := string(bodyBytes)
	rec, err := h.storage.GetLLM(r.Context(), request, model)
	if err != nil {
		logger.Warn("LLM cache lookup failed",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.String("model", model),
			zap.Error(err))
	} else if rec != nil && len(rec.Response) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-LLM-Cache", "HIT")
		_, _ = w.Write(rec.Response)
		logger.Info("Served response from LLM cache",
			zap.String("requestId", utils.GetRequestID(r)),
			zap.String("model", model))
		return true, nil
	}

	return false, &llmCacheMetadata{
		prompt:      request,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		stream:      false,
		startTime:   time.Now(),
		requestID:   utils.GetRequestID(r),
	}
}

func (h *Handler) modifyResponse(resp *http.Response) error {
	if h.storage == nil || resp == nil || resp.Request == nil {
		return nil
	}

	if meta, _ := resp.Request.Context().Value(llmCacheContextKey).(*llmCacheMetadata); meta != nil && !meta.stream {
		return h.handleLLMCachePostResponse(resp, meta)
	}

	if meta, _ := resp.Request.Context().Value(embeddingCacheContextKey).(*embeddingCacheMetadata); meta != nil {
		return h.handleEmbeddingCachePostResponse(resp, meta)
	}

	return nil
}

func (h *Handler) handleLLMCachePostResponse(resp *http.Response, meta *llmCacheMetadata) error {
	if resp.StatusCode != http.StatusOK || resp.Body == nil {
		return nil
	}

	resp.Header.Set("X-LLM-Cache", "MISS")

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

	var totalTokensPtr, promptTokensPtr, completionTokensPtr *int
	var responsePayload struct {
		Usage *struct {
			TotalTokens      *int `json:"total_tokens"`
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(bodyToStore, &responsePayload); err == nil {
		if responsePayload.Usage != nil {
			if responsePayload.Usage.TotalTokens != nil {
				total := *responsePayload.Usage.TotalTokens
				totalTokensPtr = &total
			}
			if responsePayload.Usage.PromptTokens != nil {
				prompt := *responsePayload.Usage.PromptTokens
				promptTokensPtr = &prompt
			}
			if responsePayload.Usage.CompletionTokens != nil {
				completion := *responsePayload.Usage.CompletionTokens
				completionTokensPtr = &completion
			}
		}
	}

	promptJSON, err := ensureJSONFormat(meta.prompt)
	if err != nil {
		logger.Warn("Failed to convert prompt to JSON format",
			zap.String("model", meta.model),
			zap.Error(err))
		promptJSON = json.RawMessage(fmt.Sprintf(`"%s"`, strings.ReplaceAll(meta.prompt, `"`, `\"`)))
	}

	responseJSON, err := ensureJSONFormat(string(bodyToStore))
	if err != nil {
		logger.Warn("Failed to convert response to JSON format",
			zap.String("model", meta.model),
			zap.Error(err))
		responseJSON = json.RawMessage(fmt.Sprintf(`"%s"`, strings.ReplaceAll(string(bodyToStore), `"`, `\"`)))
	}

	endTime := time.Now()
	startTime := meta.startTime

	llmRecord := &db.LLMRecord{
		RequestID:        meta.requestID,
		Request:          promptJSON,
		ModelName:        meta.model,
		Temperature:      meta.temperature,
		MaxTokens:        meta.maxTokens,
		Response:         responseJSON,
		TotalTokens:      totalTokensPtr,
		PromptTokens:     promptTokensPtr,
		CompletionTokens: completionTokensPtr,
		StartTime:        &startTime,
		EndTime:          &endTime,
	}

	if err := h.storage.UpsertLLM(resp.Request.Context(), llmRecord); err != nil {
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
