# 负载均衡功能说明

## 概述

go-llm-proxy 现在支持为模型配置多个 baseurl，并使用 RoundRobin 策略进行负载均衡。同时采用策略模式设计，支持可扩展的URL路由策略。

## 架构设计

### 策略模式
系统使用策略模式实现URL路由，通过 `URLRouteStrategy` 接口定义不同的路由策略：

```go
type URLRouteStrategy interface {
    ShouldApply(path string) bool
    GetTargetURL(request *http.Request, baseURL string) (*url.URL, error)
}
```

### 内置策略
- **ModelSpecifyStrategy**: 处理聊天完成和嵌入模型请求，支持负载均衡
- **DefaultStrategy**: 处理其他所有请求的默认策略

## 配置方式

### 1. 单个URL配置（向后兼容）

```yaml
model_routes:
  "gpt-4": "https://api.openai.com/v1"
  "gpt-3.5-turbo": "https://api.openai.com/v1"
```

### 2. 多个URL配置（负载均衡）

```yaml
model_routes:
  "glm-4":
    urls:
      - "https://open.bigmodel.cn/api/paas"
      - "https://open.bigmodel.cn/api/paas/v2"
      - "https://open.bigmodel.cn/api/paas/v3"
  
  "deepseek-v3-250324":
    urls:
      - "https://ark.cn-beijing.volces.com/api/v3"
      - "https://ark.cn-beijing.volces.com/api/v4"
      - "https://ark.cn-beijing.volces.com/api/v5"
```

## 负载均衡策略

### RoundRobin（轮询）

- 系统使用 RoundRobin 策略在配置的多个 URL 之间轮询
- 每个请求会按顺序分配到下一个可用的 URL
- 使用原子操作确保线程安全

### 策略选择逻辑

1. **路径匹配**: 系统首先检查请求路径是否在 `TargetMap` 中
2. **策略应用**: 遍历所有策略，找到第一个匹配的策略
3. **URL获取**: 使用匹配的策略获取目标URL
4. **负载均衡**: 如果配置了多个URL，使用RoundRobin选择

## 工作原理

1. **初始化阶段**：系统启动时，解析配置文件中的模型路由
2. **策略注册**：注册所有可用的URL路由策略
3. **负载均衡器创建**：为每个配置了多个URL的模型创建RoundRobin负载均衡器
4. **请求处理**：当收到请求时，按策略优先级处理
5. **URL选择**：负载均衡器返回下一个可用的URL
6. **请求转发**：将请求转发到选定的URL

## 错误处理

### 404错误处理
- 当请求路径在 `TargetMap` 中不存在时，返回404状态码
- 记录详细的警告日志，包含路径和方法信息
- 便于调试和监控

### 策略错误处理
- 当策略无法获取目标URL时，记录错误日志
- 返回适当的错误状态码

## 日志记录

系统会记录以下信息：

- 负载均衡器初始化日志
- 每次请求使用的目标URL
- 策略选择和路由决策
- 错误和警告信息

### 日志格式
使用结构化的JSON格式，包含以下字段：
- `requestId`: 请求唯一标识
- `model`: 模型名称
- `target`: 目标URL
- `path`: 请求路径
- `method`: HTTP方法
- `duration`: 请求处理时间

## 示例日志

```
INFO    Initialized load balancer for model    {"model": "glm-4", "urls": ["https://open.bigmodel.cn/api/paas", "https://open.bigmodel.cn/api/paas/v2", "https://open.bigmodel.cn/api/paas/v3"]}
INFO    Using load-balanced model route        {"requestId": "xxx", "model": "glm-4", "target": "https://open.bigmodel.cn/api/paas"}
WARN    Path not found, returning 404          {"path": "/not-exist", "method": "GET"}
```

## 性能优势

- **高可用性**：多个URL提供冗余，提高系统可用性
- **负载分散**：请求分散到多个端点，避免单点过载
- **可扩展性**：策略模式支持轻松添加新的路由策略
- **线程安全**：使用原子操作和读写锁确保并发安全

## 配置建议

1. **URL多样性**：配置多个不同的URL端点，避免单点故障
2. **地理位置**：考虑配置不同地理位置的URL，提高访问速度
3. **定期检查**：定期验证配置的URL是否仍然有效
4. **策略优化**：根据业务需求选择合适的路由策略

## 扩展新策略

### 实现步骤
1. 实现 `URLRouteStrategy` 接口
2. 在 `stratgy.go` 中添加新策略
3. 在 `handler.go` 中注册新策略

### 示例策略
```go
type CustomStrategy struct {
    // 策略特定字段
}

func (s *CustomStrategy) ShouldApply(path string) bool {
    // 判断是否应该应用此策略
    return strings.Contains(path, "custom")
}

func (s *CustomStrategy) GetTargetURL(request *http.Request, baseURL string) (*url.URL, error) {
    // 实现自定义的URL选择逻辑
    return url.Parse(baseURL)
}
```

## 注意事项

1. **向后兼容**：现有的单个 URL 配置仍然有效
2. **线程安全**：负载均衡器使用原子操作和读写锁确保线程安全
3. **配置验证**：系统会验证配置文件的格式，确保 URL 列表有效
4. **策略优先级**：策略按注册顺序执行，第一个匹配的策略会被使用
5. **错误处理**：确保所有策略都有适当的错误处理机制

## 监控和调试

### 关键指标
- 请求成功率
- 响应时间
- 负载均衡分布
- 错误率

### 调试工具
- 详细的日志记录
- 请求ID追踪
- 策略选择日志
- 性能监控 