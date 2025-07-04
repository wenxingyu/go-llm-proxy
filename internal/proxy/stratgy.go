package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/logger"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"
)

// URLRouteStrategy URL路由策略接口
type URLRouteStrategy interface {
	ShouldApply(path string) bool
	GetTargetURL(request *http.Request, baseURL string) (*url.URL, error)
}

// ModelSpecifyStrategy 聊天完成路由策略
type ModelSpecifyStrategy struct {
	lbManager *LoadBalancerManager
}

func NewModelSpecifyStrategy(lbManager *LoadBalancerManager) *ModelSpecifyStrategy {
	return &ModelSpecifyStrategy{
		lbManager: lbManager,
	}
}

func (s *ModelSpecifyStrategy) ShouldApply(path string) bool {
	return path == "/chat/completions" || strings.Contains(path, "embeddings")
}

func (s *ModelSpecifyStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
	model, err := s.extractModelFromRequest(request)
	if err != nil {
		return nil, err
	}
	targetBaseURL := s.getLoadBalancedURL(model, baseURL, request)
	return utils.GetTargetURLWithCache(targetBaseURL, request.URL.Path)
}

// chatRequest 聊天请求结构
type chatRequest struct {
	Model string `json:"model"`
}

func (s *ModelSpecifyStrategy) extractModelFromRequest(request *http.Request) (string, error) {
	var buf bytes.Buffer
	tee := io.TeeReader(request.Body, &buf)
	dec := json.NewDecoder(tee)
	var chatReq chatRequest
	if err := dec.Decode(&chatReq); err != nil {
		return "", err
	}

	request.Body = io.NopCloser(io.MultiReader(&buf, request.Body))

	if chatReq.Model == "" {
		return "", fmt.Errorf("model field is required")
	}
	return chatReq.Model, nil
}

func (s *ModelSpecifyStrategy) getLoadBalancedURL(model, fallbackURL string, request *http.Request) string {
	if modelTarget, exists := s.lbManager.GetNextURL(model); exists {
		logger.Info("Using load-balanced model route",
			zap.String("requestId", utils.GetRequestID(request)),
			zap.String("model", model),
			zap.String("target", modelTarget))
		return modelTarget
	}
	logger.Warn("No load balancer found for model, using fallback URL",
		zap.String("requestId", utils.GetRequestID(request)),
		zap.String("model", model),
		zap.String("fallback", fallbackURL))
	return fallbackURL
}

// DefaultStrategy 默认路由策略
type DefaultStrategy struct {
}

func NewDefaultStrategy() *DefaultStrategy {
	return &DefaultStrategy{}
}

func (s *DefaultStrategy) ShouldApply(path string) bool {
	return true
}

func (s *DefaultStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
	return utils.GetTargetURLWithCache(baseURL, request.URL.Path)
}
