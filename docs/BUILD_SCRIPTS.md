# 构建脚本说明

本项目提供了多种构建脚本，支持不同平台和构建需求。项目采用策略模式设计，支持负载均衡和404错误处理。

## 📁 脚本文件

### Windows 构建脚本
- **文件**: `scripts/build_windows.bat`
- **用途**: Windows平台专用构建脚本
- **特点**: 
  - 批处理格式，适合Windows环境
  - 自动检查Go环境
  - 构建Windows可执行文件
  - 复制配置文件和文档

### Linux 构建脚本
- **文件**: `scripts/build_linux.sh`
- **用途**: Linux平台专用构建脚本
- **特点**:
  - Bash脚本，适合Linux/macOS环境
  - 彩色输出，友好的用户体验
  - 支持创建tar.gz发布包
  - 自动设置可执行权限

### 跨平台构建脚本
- **文件**: `scripts/build.sh`
- **用途**: 跨平台构建脚本
- **特点**:
  - 支持多平台构建
  - 交互式选择构建平台
  - 自动检测当前平台
  - 支持Windows、Linux、macOS

### Makefile
- **文件**: `scripts/Makefile`
- **用途**: 统一的构建管理
- **特点**:
  - 支持多种构建目标
  - 自动化依赖管理
  - 支持测试、格式化、代码检查
  - 创建发布包

## 🚀 使用方法

### Windows 用户

```cmd
# 基本构建
scripts\build_windows.bat

# 指定版本构建
scripts\build_windows.bat 1.1.0
```

### Linux/macOS 用户

```bash
# 给脚本执行权限
chmod +x scripts/build_linux.sh
chmod +x scripts/build.sh

# 基本构建
./scripts/build_linux.sh

# 指定版本构建
./scripts/build_linux.sh 1.1.0

# 创建发布包
./scripts/build_linux.sh 1.1.0 package

# 跨平台构建
./scripts/build.sh 1.1.0
```

### 使用 Makefile

```bash
# 进入scripts目录
cd scripts

# 显示帮助
make help

# 构建当前平台
make build

# 构建所有平台
make build-all

# 构建特定平台
make build-win
make build-linux
make build-mac

# 清理构建文件
make clean

# 运行测试
make test

# 运行应用程序
make run

# 创建发布包
make package

# 开发模式（热重载）
make dev

# 代码检查
make lint

# 格式化代码
make fmt
```

## 📋 构建输出

### 目录结构
```
dist/
├── go-llm-proxy.exe          # Windows可执行文件
├── go-llm-proxy              # Linux/macOS可执行文件
├── go-llm-proxy-linux-amd64  # Linux AMD64
├── go-llm-proxy-linux-arm64  # Linux ARM64
├── go-llm-proxy-darwin-amd64 # macOS AMD64
├── go-llm-proxy-darwin-arm64 # macOS ARM64
├── configs/                  # 配置文件
├── README.md                 # 说明文档
├── LICENSE                   # 许可证
└── logs/                     # 日志目录
```

### 发布包
```
release/
├── go-llm-proxy-1.0.0-linux-amd64.tar.gz
├── go-llm-proxy-1.0.0-linux-arm64.tar.gz
├── go-llm-proxy-1.0.0-windows-amd64.zip
├── go-llm-proxy-1.0.0-darwin-amd64.tar.gz
└── go-llm-proxy-1.0.0-darwin-arm64.tar.gz
```

## ⚙️ 构建配置

### 版本信息
- 版本号可以通过参数指定
- 构建时间自动生成
- Go版本信息自动获取

### 构建标志
```bash
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime='${BUILD_TIME}' -s -w"
```

- `-X main.Version`: 注入版本信息
- `-X main.BuildTime`: 注入构建时间
- `-s`: 去除符号表
- `-w`: 去除调试信息

### 环境变量
- `GOOS`: 目标操作系统
- `GOARCH`: 目标架构
- `CGO_ENABLED`: CGO支持（通常设为0）

## 🔧 自定义构建

### 添加新的构建目标

在Makefile中添加新的构建目标：

```makefile
.PHONY: build-custom
build-custom:
	@echo "Building custom target..."
	GOOS=linux GOARCH=arm go build -ldflags "$(LDFLAGS)" -o dist/$(APP_NAME)-linux-arm cmd/server/main.go
```

### 修改构建参数

编辑脚本文件中的变量：

```bash
# 修改应用名称
APP_NAME="my-custom-app"

# 修改构建标志
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime='${BUILD_TIME}' -s -w -X main.CustomVar=value"
```

### 添加新的平台支持

在跨平台构建脚本中添加新平台：

```bash
# 添加FreeBSD支持
build_for_platform "freebsd" "amd64" ""
build_for_platform "freebsd" "arm64" ""
```

## 🆕 新功能特性

### 策略模式设计
- 支持可扩展的URL路由策略
- 内置ModelSpecifyStrategy和DefaultStrategy
- 易于添加新的路由策略

### 404错误处理
- 当请求路径不存在时自动返回404
- 详细的错误日志记录
- 便于调试和监控

### 增强的日志系统
- 显示真实的调用位置（文件名和行号）
- 结构化JSON日志格式
- 彩色控制台输出
- 自动日志滚动和压缩

### 负载均衡功能
- RoundRobin轮询策略
- 支持多URL配置
- 线程安全的负载均衡管理

## 🧪 测试构建

### 运行单元测试
```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/proxy -v

# 运行负载均衡器测试
go test ./internal/proxy -run TestLoadBalancer
```

### 集成测试
```bash
# 构建并运行集成测试
make build
make test-integration
```

## 📦 部署说明

### 生产环境部署
1. 使用构建脚本创建发布包
2. 解压到目标服务器
3. 配置环境变量和配置文件
4. 启动服务

### 开发环境
```bash
# 开发模式构建
make dev

# 热重载开发
make run-dev
```

### Docker部署
```bash
# 构建Docker镜像
docker build -t go-llm-proxy .

# 运行容器
docker run -p 8080:8080 go-llm-proxy
```

## 🐛 故障排除

### 常见问题

1. **权限错误**
   ```bash
   chmod +x scripts/*.sh
   ```

2. **Go环境问题**
   ```bash
   go version
   go env GOOS GOARCH
   ```

3. **构建失败**
   ```bash
   go mod tidy
   go clean -cache
   ```

4. **跨平台构建问题**
   ```bash
   # 确保设置了正确的环境变量
   export GOOS=linux
   export GOARCH=amd64
   export CGO_ENABLED=0
   ```

### 调试构建

```bash
# 显示详细构建信息
go build -v -ldflags "$(LDFLAGS)" -o dist/app cmd/server/main.go

# 显示构建的符号表
go tool nm dist/app

# 检查文件信息
file dist/app
```

## 📝 最佳实践

1. **版本管理**: 使用语义化版本号
2. **构建环境**: 在干净的环境中构建
3. **测试**: 构建后运行测试确保质量
4. **文档**: 更新版本说明和变更日志
5. **发布**: 使用发布包进行分发

这些构建脚本提供了完整的构建流程，支持开发、测试和发布的不同需求。 