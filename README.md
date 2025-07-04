# Go LLM Proxy

一个高性能的Go语言编写的LLM（大语言模型）代理服务器，用于统一管理各种LLM和嵌入模型的HTTP访问端点。

## 🚀 功能特性

### 核心功能
- **统一代理**: 通过单一HTTP端点访问多种LLM服务
- **智能路由**: 基于模型名称自动路由到对应的API服务
- **负载均衡**: 支持多个API端点的轮询负载均衡
- **路径映射**: 支持自定义路径到目标服务的映射
- **智能代理选择**: 自动判断是否需要使用代理（内网直连，外网代理）

### 高级特性
- **DNS缓存**: 智能DNS缓存机制，减少DNS查询开销
- **日志滚动**: 自动日志文件滚动和压缩，最多保留20个100MB文件
- **请求追踪**: 完整的请求ID追踪和性能监控
- **错误处理**: 完善的错误处理和日志记录
- **YAML配置**: 支持YAML配置文件

## 📋 支持的模型和服务

### 聊天完成模型
- **OpenAI**: GPT-4, GPT-3.5-turbo
- **Anthropic**: Claude-3-Opus-20240229
- **智谱AI**: GLM-4
- **DeepSeek**: DeepSeek-v3-250324
- **自定义**: 支持自定义嵌入服务端点

### 嵌入模型
- **智谱AI**: 文本嵌入服务
- **FireCrawl**: 搜索服务
- **自定义**: 支持自定义嵌入服务端点

## 🛠️ 安装和运行

### 环境要求
- Go 1.24 建议
- 网络连接（用于访问LLM API）

### 快速开始

1. **克隆项目**
```bash
git clone <repository-url>
cd go-llm-proxy
```

2. **安装依赖**
```bash
go mod tidy
```

3. **配置服务**
编辑 `configs/config.yml` 文件，配置您的API端点和模型路由。

4. **运行服务**
```bash
# 使用默认配置文件
go run cmd/server/main.go

# 或指定配置文件
go run cmd/server/main.go -f configs/custom-config.yml

# 编译后运行
go build -o go-llm-proxy cmd/server/main.go
./go-llm-proxy
```

## ⚙️ 配置说明

### 配置文件结构 (`configs/config.yml`)

```yaml
# proxy_url: "http://proxy.example.com:8080"  # 可选：HTTP代理地址
port: 8000                                    # 服务监听端口
rate_limit:                                   # 可选：限流配置
  rate: 5                                     # 每秒允许的请求数
  burst: 10                                   # 令牌桶最大突发数

target_map:
  "/chat/completions": "https://ark.cn-beijing.volces.com/api/v3"
  "/v1/search": "https://api.firecrawl.dev"
  "/embeddings": "https://open.bigmodel.cn/api/paas/v4"
  "/v1/embeddings": "http://10.236.50.39:10032"

model_routes:
  "gpt-4": "https://api.openai.com/v1"
  "gpt-3.5-turbo": "https://api.openai.com/v1"
  "claude-3-opus-20240229": "https://api.anthropic.com/v1"
  "embedding-2":
    urls:
      - "https://open.bigmodel.cn/api/paas/v3"
      - "https://open.bigmodel.cn/api/paas/v4"
```

### 配置参数说明

| 参数         | 类型   | 说明                         | 默认值 |
|--------------|--------|------------------------------|--------|
| `port`       | int    | 服务监听端口                 | 8000   |
| `proxy_url`  | string | HTTP代理地址（可选）         | ""     |
| `rate_limit` | map    | 限流配置（可选）             | -      |
| └─ `rate`    | int    | 每秒允许的请求数             | 0      |
| └─ `burst`   | int    | 令牌桶最大突发数             | 0      |
| `target_map` | map    | 路径到目标服务的映射         | -      |
| `model_routes`| map   | 模型到API服务的路由          | -      |

### 模型路由配置

#### 单个URL配置
```yaml
model_routes:
  "gpt-4": "https://api.openai.com/v1"
  "gpt-3.5-turbo": "https://api.openai.com/v1"
```

#### 多个URL负载均衡配置
```yaml
model_routes:
  "embedding-2":
    urls:
      - "https://open.bigmodel.cn/api/paas/v3"
      - "https://open.bigmodel.cn/api/paas/v4"
```

当配置多个URL时，系统会自动进行负载均衡，轮询分发请求到不同的API端点。

## 🧪 测试命令

本项目包含丰富的单元测试和集成测试，推荐在开发和提交前运行全部测试。

### 运行所有测试
```bash
go test ./...
```

### 运行指定包的测试
```bash
go test ./internal/proxy -v
go test ./internal/config -v
go test ./internal/utils -v
```

### 运行指定测试函数
```bash
go test ./internal/proxy -run TestLoadBalancer
```

## 🔧 使用示例

### 聊天完成请求

```bash
curl -X POST http://localhost:8000/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'
```

### 嵌入请求

```bash
curl -X POST http://localhost:8000/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "input": "This is a test sentence for embedding."
  }'
```

### 搜索请求

```bash
curl -X POST http://localhost:8000/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "search query"
  }'
```

## 📊 日志和监控

### 日志配置
- **日志位置**: `logs/go-llm-proxy.log`
- **日志格式**: JSON格式（文件）+ 控制台格式（标准输出）
- **日志级别**: INFO及以上级别
- **日志滚动**: 自动滚动，每个文件最大100MB，最多保留20个文件

### 日志字段
- `request_id`: 请求唯一标识
- `client_ip`: 客户端IP地址
- `path`: 请求路径
- `method`: HTTP方法
- `status`: 响应状态码
- `latency`: 请求处理时间
- `model`: 使用的模型名称（如果适用）

### 监控指标
- 请求处理时间
- 错误率统计
- 客户端IP分布
- 模型使用情况

## 🔒 安全特性

### IP地址处理
- 支持X-Real-IP和X-Forwarded-For头部
- 自动识别客户端真实IP
- 内网IP直连，外网IP使用代理

### 代理智能选择
- 自动检测目标服务是否为内网地址
- 内网服务直连，外网服务使用代理
- DNS缓存减少网络开销

## 🏗️ 架构设计

### 项目结构
```
go-llm-proxy/
├── cmd/server/           # 主程序入口
├── configs/              # 配置文件
├── internal/             # 内部包
│   ├── config/          # 配置管理
│   ├── proxy/           # 代理功能
│   └── utils/           # 工具函数
├── pkg/                  # 公共包
│   └── logger/          # 日志管理
├── docs/                 # 文档
└── scripts/              # 脚本
```

### 核心组件
1. **ProxyPool**: 连接池管理，复用HTTP连接
2. **ProxyHandler**: 请求处理器，负责路由和转发
3. **DNS缓存**: 智能DNS缓存机制
4. **日志系统**: 结构化日志记录

### 性能优化
- 连接池复用HTTP连接
- DNS查询缓存（5分钟TTL）
- 对象池复用代理实例
- 异步日志写入

## 🐛 故障排除

### 常见问题

1. **端口被占用**
   ```bash
   # 检查端口占用
   netstat -tulpn | grep :8000
   # 修改配置文件中的端口
   ```

2. **代理连接失败**
   - 检查代理服务器是否可用
   - 验证代理配置是否正确
   - 查看日志文件中的错误信息

3. **API服务不可用**
   - 检查目标API服务状态
   - 验证API密钥和认证信息
   - 查看网络连接状态

### 日志分析
```bash
# 查看实时日志
tail -f logs/go-llm-proxy.log

# 搜索错误日志
grep "error" logs/go-llm-proxy.log

# 查看请求统计
grep "Request received" logs/go-llm-proxy.log | wc -l
```

## 📝 开发指南

### 项目结构
```
go-llm-proxy/
├── cmd/server/main.go    # 主程序入口
├── internal/config/      # 配置管理
├── internal/proxy/       # 代理功能
├── internal/utils/       # 工具函数
├── pkg/logger/          # 日志管理
├── configs/             # 配置文件
├── docs/                # 文档
└── scripts/             # 脚本
```

### 添加新的模型支持
1. 在 `configs/config.yml` 中添加模型路由
2. 确保目标API服务支持该模型
3. 测试请求转发功能

### 自定义路径映射
1. 在 `target_map` 中添加新的路径映射
2. 确保目标服务支持对应的API端点
3. 重启服务使配置生效

### 开发工作流
1. 在 `internal/` 下开发新功能
2. 在 `cmd/server/main.go` 中集成
3. 更新配置文件
4. 运行测试

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📞 支持

如果您遇到问题或有建议，请：
1. 查看 [故障排除](#故障排除) 部分
2. 搜索现有的 [Issues](../../issues)
3. 创建新的 Issue 描述您的问题

---

**注意**: 使用本代理服务时，请确保您有相应LLM服务的API访问权限，并遵守各服务的使用条款和限制。
