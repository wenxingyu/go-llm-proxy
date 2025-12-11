package proxy

import (
	"bytes"
	"context"
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
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type cacheStorage interface {
	GetEmbedding(ctx context.Context, inputText, modelName string, dimensions *int) (*db.EmbeddingRecord, error)
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

// NewHandler 创建新的代理处理器，并初始化 ReverseProxy 实例
func NewHandler(cfg *config.Config) *Handler {
	manager := NewLoadBalancerManager()
	var storageInstance cacheStorage
	if cfg != nil {
		if s, err := stor.NewStorage(cfg); err != nil {
			logger.Warn("Failed to initialize storage, cache disabled", zap.Error(err))
		} else {
			storageInstance = s
		}
	}
	h := &Handler{
		cfg:       cfg,
		lbManager: manager,
		strategies: []URLRouteStrategy{
			NewModelSpecifyStrategy(manager, cfg),
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

	for alias, canonical := range h.cfg.ModelAlias {
		urls, exists := h.cfg.GetModelURLs(canonical)
		if !exists {
			logger.Warn("Alias has no target model routes configured",
				zap.String("alias", alias),
				zap.String("canonical", canonical))
			continue
		}
		h.lbManager.AddLoadBalancer(alias, urls)
		logger.Info("Initialized load balancer for alias",
			zap.String("alias", alias),
			zap.String("canonical", canonical),
			zap.Strings("urls", urls))
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
