package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-llm-server/internal/config"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/logger"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	cfg       *config.Config
}

func NewModelSpecifyStrategy(lbManager *LoadBalancerManager, cfg *config.Config) *ModelSpecifyStrategy {
	return &ModelSpecifyStrategy{
		lbManager: lbManager,
		cfg:       cfg,
	}
}

func (s *ModelSpecifyStrategy) ShouldApply(path string) bool {
	return path == "/chat/completions" || strings.Contains(path, "embeddings")
}

func (s *ModelSpecifyStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
	model, err := s.extractModelFromRequest(request)
	if err != nil {
		logger.Warn("Failed to extract model from request", zap.Error(err))
		return utils.GetTargetURLWithCache(baseURL, request.URL.Path)
	}
	targetBaseURL := s.getLoadBalancedURL(model, baseURL, request)
	return utils.GetTargetURLWithCache(targetBaseURL, request.URL.Path)
}

// chatRequest 聊天请求结构
type chatRequest struct {
	Model string `json:"model"`
}

func (s *ModelSpecifyStrategy) extractModelFromRequest(request *http.Request) (string, error) {
	// Handle OPTIONS requests (preflight CORS requests) which don't have a body
	if request.Method == "OPTIONS" {
		return "", fmt.Errorf("OPTIONS request - no model to extract")
	}

	// Check if request body is nil
	if request.Body == nil {
		return "", fmt.Errorf("request body is nil")
	}

	// Check if content length is 0
	if request.ContentLength == 0 {
		return "", fmt.Errorf("request body is empty")
	}

	// Read the entire body first
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read request body: %w", err)
	}

	// Close the original body
	request.Body.Close()

	// Decode the JSON
	var chatReq chatRequest
	if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {
		return "", fmt.Errorf("failed to decode request body: %w", err)
	}

	resolvedModel := s.resolveModelName(chatReq.Model)
	if resolvedModel == "" {
		return "", fmt.Errorf("model field is required")
	}

	// When alias resolves differently, update payload to use canonical model
	if resolvedModel != chatReq.Model {
		var payload map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &payload); err == nil {
			payload["model"] = resolvedModel
			if newBody, err := json.Marshal(payload); err == nil {
				bodyBytes = newBody
			}
		}
	}

	// Restore the request body for subsequent reads
	request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	request.ContentLength = int64(len(bodyBytes))
	if len(bodyBytes) > 0 {
		request.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	}

	return resolvedModel, nil
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

func (s *ModelSpecifyStrategy) resolveModelName(model string) string {
	if s == nil || s.cfg == nil {
		return model
	}
	return s.cfg.ResolveModel(model)
}
