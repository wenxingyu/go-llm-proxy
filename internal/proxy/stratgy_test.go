package proxy

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestModelSpecifyStrategy_ShouldApply 测试模型指定策略的应用条件
func TestModelSpecifyStrategy_ShouldApply(t *testing.T) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "聊天完成路径应该应用",
			path:     "/chat/completions",
			expected: true,
		},
		{
			name:     "包含embeddings的路径应该应用",
			path:     "/v1/embeddings",
			expected: true,
		},
		{
			name:     "复杂路径包含embeddings应该应用",
			path:     "/api/v1/models/text-embedding-ada-002/embeddings",
			expected: true,
		},
		{
			name:     "其他路径不应该应用",
			path:     "/models",
			expected: false,
		},
		{
			name:     "空路径不应该应用",
			path:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldApply(tt.path)
			if result != tt.expected {
				t.Errorf("ShouldApply(%s) = %v, 期望 %v", tt.path, result, tt.expected)
			}
		})
	}
}

// TestModelSpecifyStrategy_ExtractModelFromRequest 测试从请求中提取模型
func TestModelSpecifyStrategy_ExtractModelFromRequest(t *testing.T) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)

	tests := []struct {
		name        string
		requestBody string
		expected    string
		expectError bool
	}{
		{
			name:        "有效的模型名称",
			requestBody: `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}]}`,
			expected:    "gpt-4",
			expectError: false,
		},
		{
			name:        "空模型名称",
			requestBody: `{"model": "", "messages": [{"role": "user", "content": "hello"}]}`,
			expected:    "",
			expectError: true,
		},
		{
			name:        "缺少模型字段",
			requestBody: `{"messages": [{"role": "user", "content": "hello"}]}`,
			expected:    "",
			expectError: true,
		},
		{
			name:        "无效JSON",
			requestBody: `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"]`,
			expected:    "",
			expectError: true,
		},
		{
			name:        "复杂模型名称",
			requestBody: `{"model": "gpt-4-1106-preview", "messages": [{"role": "user", "content": "hello"}]}`,
			expected:    "gpt-4-1106-preview",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/chat/completions", strings.NewReader(tt.requestBody))
			if err != nil {
				t.Fatalf("创建请求失败: %v", err)
			}

			model, err := strategy.extractModelFromRequest(req)
			if tt.expectError {
				if err == nil {
					t.Errorf("期望错误但没有得到")
				}
			} else {
				if err != nil {
					t.Errorf("意外错误: %v", err)
				}
				if model != tt.expected {
					t.Errorf("期望模型 %s, 得到 %s", tt.expected, model)
				}
			}
		})
	}
}

// TestModelSpecifyStrategy_GetLoadBalancedURL 测试负载均衡URL获取
func TestModelSpecifyStrategy_GetLoadBalancedURL(t *testing.T) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)

	// 添加测试负载均衡器
	lbManager.AddLoadBalancer("gpt-4", []string{"https://api1.example.com", "https://api2.example.com"})
	lbManager.AddLoadBalancer("gpt-3.5-turbo", []string{"https://api3.example.com"})

	req, _ := http.NewRequest("POST", "/chat/completions", nil)
	req.Header.Set("X-Request-ID", "test-request-id")

	tests := []struct {
		name         string
		model        string
		fallbackURL  string
		expectedURLs []string
	}{
		{
			name:         "存在的模型应该使用负载均衡器",
			model:        "gpt-4",
			fallbackURL:  "https://fallback.example.com",
			expectedURLs: []string{"https://api1.example.com", "https://api2.example.com"},
		},
		{
			name:         "单个URL的模型",
			model:        "gpt-3.5-turbo",
			fallbackURL:  "https://fallback.example.com",
			expectedURLs: []string{"https://api3.example.com"},
		},
		{
			name:         "不存在的模型应该使用回退URL",
			model:        "non-existent-model",
			fallbackURL:  "https://fallback.example.com",
			expectedURLs: []string{"https://fallback.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.getLoadBalancedURL(tt.model, tt.fallbackURL, req)

			found := false
			for _, expectedURL := range tt.expectedURLs {
				if result == expectedURL {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("期望其中一个 %v, 得到 %s", tt.expectedURLs, result)
			}
		})
	}
}

// TestModelSpecifyStrategy_GetTargetURL 测试模型指定策略的目标URL获取
func TestModelSpecifyStrategy_GetTargetURL(t *testing.T) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)

	// Add test load balancers
	lbManager.AddLoadBalancer("gpt-4", []string{"https://api1.example.com"})
	lbManager.AddLoadBalancer("gpt-3.5-turbo", []string{"https://api2.example.com:8080"})

	tests := []struct {
		name        string
		requestBody string
		baseURL     string
		path        string
		expectError bool
		expectedURL string
	}{
		{
			name:        "valid request, baseURL and lbURL same",
			requestBody: `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}]}`,
			baseURL:     "https://api1.example.com",
			path:        "/chat/completions",
			expectError: false,
			expectedURL: "https://api1.example.com/chat/completions",
		},
		{
			name:        "valid request, baseURL and lbURL different",
			requestBody: `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "hello"}]}`,
			baseURL:     "https://fallback.example.com",
			path:        "/v1/embeddings",
			expectError: false,
			expectedURL: "https://api2.example.com:8080/v1/embeddings",
		},
		{
			name:        "invalid JSON",
			requestBody: `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"]`,
			baseURL:     "https://api1.example.com",
			path:        "/chat/completions",
			expectError: true,
			expectedURL: "",
		},
		{
			name:        "empty model",
			requestBody: `{"model": "", "messages": [{"role": "user", "content": "hello"}]}`,
			baseURL:     "https://api1.example.com",
			path:        "/chat/completions",
			expectError: true,
			expectedURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", tt.path, strings.NewReader(tt.requestBody))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			result, err := strategy.GetTargetURL(req, tt.baseURL)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil URL")
				} else if result.String() != tt.expectedURL {
					t.Errorf("expected %s, got %s", tt.expectedURL, result.String())
				}
			}
		})
	}
}

// TestDefaultStrategy_ShouldApply 测试默认策略的应用条件
func TestDefaultStrategy_ShouldApply(t *testing.T) {
	strategy := NewDefaultStrategy()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "任何路径都应该应用",
			path:     "/chat/completions",
			expected: true,
		},
		{
			name:     "空路径也应该应用",
			path:     "",
			expected: true,
		},
		{
			name:     "复杂路径也应该应用",
			path:     "/api/v1/models/text-embedding-ada-002/embeddings",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldApply(tt.path)
			if result != tt.expected {
				t.Errorf("ShouldApply(%s) = %v, 期望 %v", tt.path, result, tt.expected)
			}
		})
	}
}

// TestDefaultStrategy_GetTargetURL 测试默认策略目标URL获取
func TestDefaultStrategy_GetTargetURL(t *testing.T) {
	strategy := NewDefaultStrategy()

	tests := []struct {
		name        string
		path        string
		baseURL     string
		expected    string
		expectError bool
	}{
		{
			name:        "有效URL和路径",
			path:        "/chat/completions",
			baseURL:     "https://api.example.com",
			expected:    "https://api.example.com/chat/completions",
			expectError: false,
		},
		{
			name:        "带端口的URL",
			path:        "/v1/embeddings",
			baseURL:     "https://api.example.com:8080",
			expected:    "https://api.example.com:8080/v1/embeddings",
			expectError: false,
		},
		{
			name:        "无效URL",
			path:        "/chat/completions",
			baseURL:     "://invalid-url",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", tt.path, nil)
			if err != nil {
				t.Fatalf("创建请求失败: %v", err)
			}

			result, err := strategy.GetTargetURL(req, tt.baseURL)
			if tt.expectError {
				if err == nil {
					t.Errorf("期望错误但没有得到")
				}
			} else {
				if err != nil {
					t.Errorf("意外错误: %v", err)
				}
				if result.String() != tt.expected {
					t.Errorf("期望 %s, 得到 %s", tt.expected, result.String())
				}
			}
		})
	}
}

// TestNewModelSpecifyStrategy 测试模型指定策略构造函数
func TestNewModelSpecifyStrategy(t *testing.T) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)

	if strategy == nil {
		t.Error("期望非空策略")
	}
	if strategy.lbManager != lbManager {
		t.Error("期望lbManager被正确设置")
	}
}

// TestNewDefaultStrategy 测试默认策略构造函数
func TestNewDefaultStrategy(t *testing.T) {
	strategy := NewDefaultStrategy()

	if strategy == nil {
		t.Error("期望非空策略")
	}
}

// BenchmarkModelSpecifyStrategy_ShouldApply 基准测试：策略应用检查
func BenchmarkModelSpecifyStrategy_ShouldApply(b *testing.B) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)
	path := "/chat/completions"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.ShouldApply(path)
	}
}

// BenchmarkModelSpecifyStrategy_ExtractModelFromRequest 基准测试：模型提取
func BenchmarkModelSpecifyStrategy_ExtractModelFromRequest(b *testing.B) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)

	requestBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello world"}]}`
	req, _ := http.NewRequest("POST", "/chat/completions", strings.NewReader(requestBody))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.Body = io.NopCloser(strings.NewReader(requestBody))
		strategy.extractModelFromRequest(req)
	}
}

// BenchmarkModelSpecifyStrategy_GetLoadBalancedURL 基准测试：负载均衡URL获取
func BenchmarkModelSpecifyStrategy_GetLoadBalancedURL(b *testing.B) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)

	lbManager.AddLoadBalancer("gpt-4", []string{"https://api1.example.com", "https://api2.example.com"})
	req, _ := http.NewRequest("POST", "/chat/completions", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.getLoadBalancedURL("gpt-4", "https://fallback.example.com", req)
	}
}

// BenchmarkDefaultStrategy_ShouldApply 基准测试：默认策略应用检查
func BenchmarkDefaultStrategy_ShouldApply(b *testing.B) {
	strategy := NewDefaultStrategy()
	path := "/chat/completions"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.ShouldApply(path)
	}
}

// BenchmarkDefaultStrategy_GetTargetURL 基准测试：默认策略目标URL获取
func BenchmarkDefaultStrategy_GetTargetURL(b *testing.B) {
	strategy := NewDefaultStrategy()
	req, _ := http.NewRequest("POST", "/chat/completions", nil)
	baseURL := "https://api.example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.GetTargetURL(req, baseURL)
	}
}

// BenchmarkModelSpecifyStrategy_GetTargetURL_Complete 基准测试：模型指定策略完整目标URL获取
func BenchmarkModelSpecifyStrategy_GetTargetURL_Complete(b *testing.B) {
	lbManager := NewLoadBalancerManager()
	strategy := NewModelSpecifyStrategy(lbManager)

	lbManager.AddLoadBalancer("gpt-4", []string{"https://api1.example.com"})

	requestBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello world"}]}`
	req, _ := http.NewRequest("POST", "/chat/completions", strings.NewReader(requestBody))
	baseURL := "https://api1.example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.Body = io.NopCloser(strings.NewReader(requestBody))
		strategy.GetTargetURL(req, baseURL)
	}
}
