# 日志滚动功能说明

## 功能特性

本项目已集成日志滚动功能，使用 `lumberjack` 库实现，具有以下特性：

### 滚动策略
- **文件大小限制**: 每个日志文件最大 100MB
- **备份文件数量**: 最多保留 20 个备份文件
- **压缩功能**: 当达到备份文件数量上限时，旧文件会被自动压缩为 `.gz` 格式
- **时间设置**: 使用本地时间戳

### 日志格式
- **结构化日志**: 使用JSON格式记录日志，便于解析和分析
- **真实调用位置**: 显示真实的调用文件名和行号，而不是日志封装函数的位置
- **彩色控制台输出**: 控制台输出使用彩色格式，便于阅读
- **双输出**: 同时输出到文件和控制台

### 配置参数

在 `pkg/logger/logger.go` 的 `InitLogger()` 函数中，日志滚动配置如下：

```go
rollingFile := &lumberjack.Logger{
    Filename:   "logs/go-llm-proxy.log", // 日志文件名
    MaxSize:    100,                      // 每个日志文件最大100MB
    MaxBackups: 20,                       // 最多保留20个备份文件
    MaxAge:     0,                        // 不按时间删除，只按数量删除
    Compress:   true,                     // 压缩旧日志文件
    LocalTime:  true,                     // 使用本地时间
}
```

### 编码器配置

```go
encoderConfig := zapcore.EncoderConfig{
    TimeKey:        "time",
    LevelKey:       "level",
    NameKey:        "logger",
    CallerKey:      "caller",
    MessageKey:     "msg",
    StacktraceKey:  "stacktrace",
    LineEnding:     zapcore.DefaultLineEnding,
    EncodeLevel:    zapcore.CapitalLevelEncoder,
    EncodeTime:     zapcore.ISO8601TimeEncoder,
    EncodeDuration: zapcore.SecondsDurationEncoder,
    EncodeCaller:   zapcore.ShortCallerEncoder,
}
```

### 文件命名规则

- `logs/go-llm-proxy.log` - 当前正在写入的日志文件
- `logs/go-llm-proxy.log.1` - 第一个备份文件
- `logs/go-llm-proxy.log.2` - 第二个备份文件
- ...
- `logs/go-llm-proxy.log.20` - 第20个备份文件
- `logs/go-llm-proxy.log.1.gz` - 压缩后的备份文件

### 工作原理

1. 当当前日志文件达到 100MB 时，会自动创建新的日志文件
2. 旧的日志文件会被重命名为 `.1`、`.2` 等后缀
3. 当备份文件数量超过 20 个时，最旧的文件会被压缩为 `.gz` 格式
4. 压缩后的文件会占用更少的磁盘空间

### 日志示例

#### 控制台输出（彩色）
```
2025-06-26T23:57:07.544+0800    INFO    这是来自testFunction1的日志    caller=test_log.go:18
2025-06-26T23:57:07.549+0800    ERROR   这是来自testFunction2的错误日志    caller=test_log.go:22
```

#### 文件输出（JSON格式）
```json
{"level":"INFO","time":"2025-06-26T23:57:07.544+0800","caller":"test_log.go:18","msg":"这是来自testFunction1的日志"}
{"level":"ERROR","time":"2025-06-26T23:57:07.549+0800","caller":"test_log.go:22","msg":"这是来自testFunction2的错误日志"}
```

### 请求日志配置

#### 请求体记录控制

在 `configs/config.yml` 中可以通过 `log_body` 参数控制是否记录请求体：

```yaml
log_body: false  # 默认不记录请求体
```

- **`log_body: true`**: 记录请求体内容到日志中，便于调试
- **`log_body: false`**: 不记录请求体，保护敏感信息（默认值）

#### 请求日志字段

当记录请求时，日志包含以下字段：

```json
{
  "level": "INFO",
  "time": "2025-06-26T23:57:07.544+0800",
  "caller": "handler.go:125",
  "msg": "Request received",
  "requestId": "550e8400-e29b-41d4-a716-446655440000",
  "clientIp": "192.168.1.100",
  "path": "/chat/completions",
  "method": "POST",
  "targetUrl": "http://localhost:8000/chat/completions",
  "Content-length": 256,
  "requestBody": "{\"model\":\"gpt-4\",\"messages\":[...]}"  // 仅当 log_body: true 时记录
}
```

**注意**: 在生产环境中建议设置 `log_body: false` 以避免记录敏感信息。

### 调用位置显示

系统使用 `zap.AddCallerSkip(1)` 配置，确保日志显示真实的调用位置：

```go
logger = zap.New(core, zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
```

这样日志会显示业务代码的真实文件名和行号，而不是日志封装函数的位置。

### 优势

- **磁盘空间管理**: 自动控制日志文件大小，防止磁盘空间耗尽
- **性能优化**: 压缩旧日志文件，减少磁盘占用
- **易于管理**: 固定数量的备份文件，便于管理和清理
- **实时滚动**: 无需重启服务即可进行日志滚动
- **调试友好**: 显示真实的调用位置，便于调试
- **结构化**: JSON格式便于日志分析和监控

### 依赖库

本项目使用以下库实现日志功能：

```bash
go get go.uber.org/zap
go get gopkg.in/natefinch/lumberjack.v2
```

### 日志级别

- **DEBUG**: 调试信息
- **INFO**: 一般信息
- **WARN**: 警告信息
- **ERROR**: 错误信息
- **FATAL**: 致命错误

### 使用示例

```go
import "go-llm-server/pkg/logger"

// 初始化日志
logger.InitLogger()
defer logger.Sync()

// 记录不同级别的日志
logger.Info("服务器启动成功", zap.Int("port", 8080))
logger.Error("连接失败", zap.Error(err))
logger.Warn("配置项缺失", zap.String("key", "database_url"))
```

### 注意事项

- 日志滚动是自动进行的，无需手动干预
- 压缩功能会在后台自动执行，不会影响服务性能
- 建议定期检查日志文件大小和数量，确保符合预期
- 日志目录会自动创建，无需手动创建
- 确保应用有足够的磁盘空间用于日志存储 