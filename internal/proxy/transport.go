package proxy

import (
	"go-llm-server/internal/config"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/logger"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
)

var (
	directTransport = &http.Transport{
		Proxy:                 nil,
		ResponseHeaderTimeout: 900 * time.Second,
		MaxConnsPerHost:       200,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
	}

	proxyTransport = &http.Transport{
		Proxy:                 nil,
		ResponseHeaderTimeout: 900 * time.Second,
		MaxConnsPerHost:       200,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
	}
)

type TransportWithProxyAutoDetected struct {
}

func (t *TransportWithProxyAutoDetected) RoundTrip(r *http.Request) (*http.Response, error) {
	startTime := time.Now()
	useProxy := utils.ShouldUseProxy(r.URL.Hostname())
	trans := directTransport
	if useProxy {
		trans = proxyTransport
	}
	response, err := trans.RoundTrip(r)
	duration := time.Since(startTime)
	requestId := utils.GetRequestID(r)
	if err != nil {
		logger.Warn("Transport error occurred",
			zap.String("requestId", requestId),
			zap.Error(err),
			zap.Duration("duration", duration))
		return response, err
	}
	logger.Info("Receive response",
		zap.String("requestId", requestId),
		zap.Int("status", response.StatusCode),
		zap.Int("Content-Length", int(response.ContentLength)),
		zap.Duration("duration", duration))
	response.Header.Set("X-Request-ID", requestId)
	return response, err
}

func InitHttpProxyTransport(cfg *config.Config) {
	if cfg.ProxyURL == "" {
		logger.Warn("Proxy url is empty, use direct request")
		return
	}
	proxyURLParsed, err := url.Parse(cfg.ProxyURL)
	if err != nil {
		logger.Fatal("Invalid proxy", zap.String("proxy", cfg.ProxyURL), zap.Error(err))
	}
	proxyTransport.Proxy = http.ProxyURL(proxyURLParsed)
}
