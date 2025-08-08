package utils

import (
	"fmt"
	"io"
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

// ReadRequestBody 读取请求体并返回字节数组，同时保持请求体可重复读取
func ReadRequestBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, fmt.Errorf("request or request body is nil")
	}

	// 读取请求体
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// 重新设置请求体，使其可以重复读取
	r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	return bodyBytes, nil
}
