package utils

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	gocache "github.com/patrickmn/go-cache"
)

// Global URL cache
var urlCache = gocache.New(gocache.NoExpiration, gocache.NoExpiration)

// GetTargetURLWithCache builds URL with caching
func GetTargetURLWithCache(baseURL, path string) (*url.URL, error) {
	cacheKey := baseURL + "|" + path
	if cachedURL, found := urlCache.Get(cacheKey); found {
		return cachedURL.(*url.URL), nil
	}

	targetURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target URL: %w", err)
	}

	resultURL := targetURL.JoinPath(path)
	urlCache.Set(cacheKey, resultURL, gocache.NoExpiration)
	return resultURL, nil
}

// GetRequestID extracts request ID from HTTP request headers
func GetRequestID(r *http.Request) string {
	if r != nil {
		return r.Header.Get("X-Request-ID")
	}
	return ""
}

// GetOrGenerateRequestID 判断客户端请求是否存在X-Request-ID，如果不存在使用UUID填充
func GetOrGenerateRequestID(r *http.Request) string {
	if r == nil {
		return uuid.New().String()
	}
	requestId := r.Header.Get("X-Request-ID")
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		requestId = uuid.New().String()
		r.Header.Set("X-Request-ID", requestId)
	}
	return requestId
}
