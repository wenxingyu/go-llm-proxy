# 项目目录结构

本项目采用Go语言标准项目布局，遵循清晰的分层架构设计，支持负载均衡功能和策略模式的路由管理。

## 📁 目录结构

```
go-llm-proxy/
├── cmd/                    # 应用程序入口点
│   └── server/            # 服务器主程序
│       └── main.go        # 主程序入口
├── configs/               # 配置文件
│   └── config.yml         # 应用配置文件（支持负载均衡配置）
├── internal/              # 私有应用程序和库代码
│   ├── config/           # 配置管理
│   │   └── config.go     # 配置加载和结构定义（支持多URL配置）
│   ├── proxy/            # 代理功能核心
│   │   ├── handler.go    # HTTP请求处理器（集成策略模式）
│   │   ├── stratgy.go    # URL路由策略实现（策略模式）
│   │   ├── loadbalancer.go # 负载均衡器实现（RoundRobin）
│   │   ├── loadbalancer_test.go # 负载均衡器测试
│   │   ├── transport.go  # 传输层处理
│   │   └── pool.go       # 代理连接池管理
│   ├── database/         # 数据库相关（预留）
│   └── utils/            # 工具函数
│       └── ip_utils.go   # IP处理和DNS缓存
├── pkg/                   # 可以被外部应用程序使用的库代码
│   └── logger/           # 日志管理
│       └── logger.go     # 日志初始化和接口
├── docs/                  # 文档
│   ├── BUILD_SCRIPTS.md  # 构建脚本说明
│   ├── LOAD_BALANCING.md # 负载均衡功能说明
│   ├── LOG_ROTATION.md   # 日志滚动说明
│   └── PROJECT_STRUCTURE.md # 项目结构说明
├── scripts/               # 脚本文件
│   ├── build.sh          # Linux构建脚本
│   ├── build_linux.sh    # Linux构建脚本
│   ├── build_windows.bat # Windows构建脚本
│   └── Makefile          # Make构建文件
├── logs/                  # 日志文件目录（运行时生成）
├── dist/                  # 构建输出目录（运行时生成）
├── go.mod                 # Go模块文件
├── go.sum                 # 依赖校验文件
├── README.md              # 项目说明文档
├── LICENSE                # 许可证文件
└── .gitignore            # Git忽略文件
```

## 🏗️ 架构说明

### cmd/ - 应用程序入口
- **cmd/server/**: 包含服务器应用程序的主入口点
- 每个可执行文件都有自己的目录
- 主程序负责初始化和启动服务

### configs/ - 配置文件
- 存放所有配置文件
- 支持不同环境的配置（开发、测试、生产）
- 配置文件使用YAML格式
- **新增负载均衡配置支持**：可为模型配置多个baseurl

### internal/ - 内部包
- **internal/config/**: 配置管理，负责加载和解析配置文件
  - 支持单个URL和多个URL配置格式
  - 向后兼容原有配置
- **internal/proxy/**: 代理功能核心实现
  - `handler.go`: HTTP请求处理逻辑，集成策略模式
    - 支持404错误处理
    - 使用策略模式进行URL路由
  - `stratgy.go`: URL路由策略实现（策略模式）
    - `URLRouteStrategy` 接口定义
    - `ModelSpecifyStrategy`: 模型特定路由策略
    - `DefaultStrategy`: 默认路由策略
    - 支持扩展新的路由策略
  - `loadbalancer.go`: 负载均衡器实现
    - RoundRobin轮询策略
    - 线程安全的负载均衡管理
  - `loadbalancer_test.go`: 负载均衡器单元测试
  - `transport.go`: 传输层处理
    - 代理自动检测
    - 请求响应日志记录
  - `pool.go`: 连接池管理，提高性能
- **internal/database/**: 数据库相关功能（预留）
- **internal/utils/**: 工具函数
  - `ip_utils.go`: IP地址处理、DNS缓存等工具函数

### pkg/ - 公共包
- **pkg/logger/**: 日志管理包
  - 提供统一的日志接口
  - 支持日志滚动和压缩
  - 显示真实的调用位置（文件名和行号）
  - 可以被其他项目复用

### docs/ - 文档
- **BUILD_SCRIPTS.md**: 构建脚本使用说明
- **LOAD_BALANCING.md**: 负载均衡功能详细说明
- **LOG_ROTATION.md**: 日志滚动配置说明
- **PROJECT_STRUCTURE.md**: 项目结构说明

### scripts/ - 脚本
- **build.sh**: 通用构建脚本
- **build_linux.sh**: Linux平台构建脚本
- **build_windows.bat**: Windows平台构建脚本
- **Makefile**: Make构建文件

## 🔄 代码组织原则

### 1. 依赖方向
```
cmd/ → internal/ → pkg/
```
- `cmd/` 依赖 `internal/` 和 `pkg/`
- `internal/` 可以依赖 `pkg/`
- `pkg/` 不依赖 `internal/`

### 2. 包设计原则
- **单一职责**: 每个包只负责一个特定功能
- **高内聚**: 相关功能放在同一个包中
- **低耦合**: 包之间通过接口进行交互
- **策略模式**: 使用策略模式实现可扩展的URL路由

### 3. 命名规范
- 包名使用小写字母
- 文件名使用下划线分隔
- 结构体和方法使用驼峰命名

## 🚀 构建和运行

### 构建
```bash
# 使用Makefile构建
make build

# 构建服务器
go build -o go-llm-proxy.exe cmd/server/main.go

# 或者使用模块构建
go build -o go-llm-proxy.exe ./cmd/server
```

### 运行
```bash
# 使用默认配置
./go-llm-proxy.exe

# 指定配置文件
./go-llm-proxy.exe -f configs/custom-config.yml
```

## 🔧 负载均衡功能

### 配置示例
```yaml
model_routes:
  # 单个URL配置（向后兼容）
  "gpt-4": "https://api.openai.com/v1"
  
  # 多个URL配置（负载均衡）
  "glm-4":
    urls:
      - "https://open.bigmodel.cn/api/paas"
      - "https://open.bigmodel.cn/api/paas/v2"
      - "https://open.bigmodel.cn/api/paas/v3"
```

### 核心特性
- **RoundRobin轮询**: 在多个URL间轮询分发请求
- **线程安全**: 使用原子操作和读写锁
- **策略模式**: 支持多种URL路由策略

## 🎯 策略模式设计

### URLRouteStrategy 接口
```go
type URLRouteStrategy interface {
    ShouldApply(path string) bool
    GetTargetURL(request *http.Request, baseURL string) (*url.URL, error)
}
```

### 内置策略
- **ModelSpecifyStrategy**: 处理聊天完成和嵌入模型请求
- **DefaultStrategy**: 处理其他所有请求

### 扩展新策略
1. 实现 `URLRouteStrategy` 接口
2. 在 `stratgy.go` 中添加新策略
3. 在 `handler.go` 中注册新策略

## 📝 开发指南

### 添加新功能
1. 在 `internal/` 下创建相应的包
2. 在 `cmd/server/main.go` 中集成新功能
3. 更新配置文件（如需要）
4. 添加相应的测试

### 添加新的可执行文件
1. 在 `cmd/` 下创建新的目录
2. 创建 `main.go` 文件
3. 复用 `internal/` 和 `pkg/` 中的代码

### 配置管理
1. 在 `configs/` 中添加配置文件
2. 在 `internal/config/` 中定义配置结构
3. 在代码中使用配置

### 负载均衡扩展
1. 在 `internal/proxy/loadbalancer.go` 中添加新的负载均衡策略
2. 实现 `LoadBalancer` 接口
3. 在 `LoadBalancerManager` 中集成新策略
4. 添加相应的测试用例

### 路由策略扩展
1. 在 `internal/proxy/stratgy.go` 中实现新的策略
2. 实现 `URLRouteStrategy` 接口
3. 在 `Handler` 初始化时注册新策略
4. 添加相应的测试用例

## 🧪 测试

### 运行测试
```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/proxy -v

# 运行负载均衡器测试
go test ./internal/proxy -run TestLoadBalancer
```

### 测试覆盖
- 负载均衡器功能测试
- 配置解析测试
- 策略模式测试

## 🐛 错误处理

### 404错误处理
- 当请求路径在 `TargetMap` 中不存在时，返回404
- 记录详细的警告日志，包含路径和方法信息
- 便于调试和监控

### 日志记录
- 显示真实的调用位置（文件名和行号）
- 结构化的日志格式
- 支持日志滚动和压缩

这种目录结构提供了良好的代码组织、清晰的依赖关系、易于维护的架构，并支持高可用的负载均衡功能和可扩展的策略模式设计。 