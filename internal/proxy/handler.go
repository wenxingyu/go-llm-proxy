package proxy

import (
	"go-llm-server/internal/config"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/logger"
	"net/http"
	"net/http/httputil"
	"net/url"

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

	ipLimiters sync.Map // map[string]*rate.Limiter
}

// NewHandler 创建新的代理处理器，并初始化 ReverseProxy 实例
func NewHandler(cfg *config.Config) *Handler {
	manager := NewLoadBalancerManager()
	h := &Handler{
		cfg:       cfg,
		lbManager: manager,
		strategies: []URLRouteStrategy{
			NewModelSpecifyStrategy(manager),
			NewDefaultStrategy(),
		},
	}

	// 构造单例 ReverseProxy
	h.proxy = &httputil.ReverseProxy{
		Director:     h.director,
		ErrorHandler: h.errorHandler,
		Transport:    &TransportWithProxyAutoDetected{},
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

	if h.cfg.LogBody && r.Body != nil {
		bodyBytes, err := utils.ReadRequestBody(r)
		if err == nil {
			logFields = append(logFields, zap.String("requestBody", string(bodyBytes)))
		} else {
			logger.Warn("Failed to read request body", zap.Error(err))
		}
	}

	logger.Info("Request received", logFields...)

	if r.Method == "OPTIONS" {
		w.Header().Set("Vary", "Origin,Access-Control-Request-Method,Access-Control-Request-Headers")
		w.Header().Set("Allow", "POST,OPTIONS")
		w.WriteHeader(http.StatusOK)
		return
	}

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

	// 交给同一个 ReverseProxy 实例处理
	h.proxy.ServeHTTP(w, r)
}
