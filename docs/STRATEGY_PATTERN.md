# 策略模式设计说明

## 概述

go-llm-proxy 采用策略模式实现URL路由，提供了灵活、可扩展的路由策略管理机制。

## 设计目标

- **可扩展性**: 支持轻松添加新的路由策略
- **可维护性**: 策略逻辑分离，便于维护和测试
- **灵活性**: 支持不同路径使用不同的路由策略
- **向后兼容**: 保持现有功能的兼容性

## 核心接口

### URLRouteStrategy 接口

```go
type URLRouteStrategy interface {
    ShouldApply(path string) bool
    GetTargetURL(request *http.Request, baseURL string) (*url.URL, error)
}
```

#### 方法说明

- **ShouldApply(path string) bool**: 判断策略是否适用于指定路径
- **GetTargetURL(request *http.Request, baseURL string) (*url.URL, error)**: 获取目标URL

## 内置策略

### 1. ModelSpecifyStrategy

处理聊天完成和嵌入模型请求的策略。

#### 适用路径
- `/chat/completions`
- 包含 `embeddings` 的路径

#### 功能特性
- 从请求体中提取模型信息
- 支持负载均衡
- 自动恢复请求体

#### 实现示例
```go
func (s *ModelSpecifyStrategy) ShouldApply(path string) bool {
    return path == "/chat/completions" || strings.Contains(path, "embeddings")
}

func (s *ModelSpecifyStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
    s.request = request
    model, err := s.extractModelFromRequest(request)
    if err != nil {
        return nil, err
    }
    targetURL := s.getLoadBalancedURL(model, baseURL)
    return s.buildTargetURL(targetURL, request.URL.Path)
}
```

### 2. DefaultStrategy

处理其他所有请求的默认策略。

#### 适用路径
- 所有其他路径

#### 功能特性
- 简单的URL构建
- 作为兜底策略

#### 实现示例
```go
func (s *DefaultStrategy) ShouldApply(path string) bool {
    return true
}

func (s *DefaultStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
    targetURL, err := url.Parse(baseURL)
    if err != nil {
        return nil, fmt.Errorf("failed to parse target URL: %w", err)
    }
    return targetURL.JoinPath(request.URL.Path), nil
}
```

## 策略注册和管理

### Handler 结构

```go
type Handler struct {
    cfg        *config.Config
    lbManager  *LoadBalancerManager
    strategies []URLRouteStrategy
}
```

### 策略注册

```go
func NewHandler(cfg *config.Config) *Handler {
    lbManager := NewLoadBalancerManager()
    return &Handler{
        cfg:       cfg,
        lbManager: lbManager,
        strategies: []URLRouteStrategy{
            NewModelSpecifyStrategy(lbManager),
            &DefaultStrategy{},
        },
    }
}
```

### 策略执行

```go
func (h *Handler) getTargetURL(request *http.Request) (*url.URL, bool) {
    path := request.URL.Path
    targetBase, exists := h.cfg.TargetMap[path]
    if !exists {
        return nil, false
    }
    
    for _, strategy := range h.strategies {
        if strategy.ShouldApply(path) {
            targetURL, err := strategy.GetTargetURL(request, targetBase)
            if err != nil {
                return nil, false
            }
            return targetURL, true
        }
    }
    
    return nil, false
}
```

## 扩展新策略

### 步骤1: 实现策略接口

```go
type CustomStrategy struct {
    lbManager *LoadBalancerManager
    request   *http.Request
}

func NewCustomStrategy(lbManager *LoadBalancerManager) *CustomStrategy {
    return &CustomStrategy{lbManager: lbManager}
}

func (s *CustomStrategy) ShouldApply(path string) bool {
    return strings.Contains(path, "custom")
}

func (s *CustomStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
    s.request = request
    
    // 实现自定义逻辑
    customURL := s.getCustomURL(baseURL)
    return s.buildTargetURL(customURL, request.URL.Path)
}

func (s *CustomStrategy) getCustomURL(fallbackURL string) string {
    // 自定义URL选择逻辑
    return fallbackURL
}

func (s *CustomStrategy) buildTargetURL(baseURL, path string) (*url.URL, error) {
    targetURL, err := url.Parse(baseURL)
    if err != nil {
        return nil, fmt.Errorf("failed to parse target URL: %w", err)
    }
    return targetURL.JoinPath(path), nil
}

func (s *CustomStrategy) getRequestID() string {
    if s.request != nil {
        return s.request.Header.Get("X-Request-ID")
    }
    return ""
}
```

### 步骤2: 注册新策略

在 `handler.go` 的 `NewHandler` 函数中注册新策略：

```go
strategies: []URLRouteStrategy{
    NewModelSpecifyStrategy(lbManager),
    NewCustomStrategy(lbManager),  // 添加新策略
    &DefaultStrategy{},
},
```

### 步骤3: 测试新策略

```go
func TestCustomStrategy(t *testing.T) {
    lbManager := NewLoadBalancerManager()
    strategy := NewCustomStrategy(lbManager)
    
    // 测试 ShouldApply
    assert.True(t, strategy.ShouldApply("/custom/path"))
    assert.False(t, strategy.ShouldApply("/other/path"))
    
    // 测试 GetTargetURL
    // ... 测试逻辑
}
```

## 策略优先级

策略按注册顺序执行，第一个匹配的策略会被使用：

1. **ModelSpecifyStrategy**: 处理聊天和嵌入请求
2. **CustomStrategy**: 处理自定义路径
3. **DefaultStrategy**: 处理所有其他请求

## 最佳实践

### 1. 策略设计原则

- **单一职责**: 每个策略只负责一种类型的路由逻辑
- **可测试性**: 策略应该是可独立测试的
- **错误处理**: 确保策略有适当的错误处理机制

### 2. 性能考虑

- **快速匹配**: `ShouldApply` 方法应该快速返回
- **资源管理**: 避免在策略中创建不必要的资源
- **缓存**: 考虑缓存常用的URL解析结果

### 3. 日志记录

- **请求追踪**: 记录策略选择和URL选择过程
- **错误日志**: 记录策略执行中的错误
- **性能监控**: 记录策略执行时间

### 4. 配置管理

- **策略配置**: 支持通过配置文件启用/禁用策略
- **参数配置**: 支持策略特定的配置参数
- **动态配置**: 支持运行时更新策略配置

## 示例场景

### 场景1: 基于用户的路由

```go
type UserBasedStrategy struct {
    userRoutes map[string]string
}

func (s *UserBasedStrategy) ShouldApply(path string) bool {
    return strings.Contains(path, "/user/")
}

func (s *UserBasedStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
    userID := extractUserID(request)
    if userRoute, exists := s.userRoutes[userID]; exists {
        return url.Parse(userRoute)
    }
    return url.Parse(baseURL)
}
```

### 场景2: 基于时间的路由

```go
type TimeBasedStrategy struct {
    timeRoutes map[string]string
}

func (s *TimeBasedStrategy) ShouldApply(path string) bool {
    return strings.Contains(path, "/time-sensitive/")
}

func (s *TimeBasedStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
    hour := time.Now().Hour()
    if hour >= 9 && hour <= 18 {
        return url.Parse(s.timeRoutes["business"])
    }
    return url.Parse(s.timeRoutes["off-hours"])
}
```

## 总结

策略模式为 go-llm-proxy 提供了灵活、可扩展的URL路由机制。通过实现 `URLRouteStrategy` 接口，可以轻松添加新的路由策略，满足不同的业务需求。同时，策略的分离设计使得代码更易维护和测试。 