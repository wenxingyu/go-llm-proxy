package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *zap.Logger

// InitLogger 初始化日志系统
func InitLogger() {
	// 创建自定义编码器配置
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

	// 确保日志目录存在
	if err := os.MkdirAll("logs", 0755); err != nil {
		panic("Failed to create logs directory: " + err.Error())
	}

	// 创建滚动文件写入器
	rollingFile := &lumberjack.Logger{
		Filename:   "logs/go-llm-proxy.log", // 日志文件名
		MaxSize:    100,                     // 每个日志文件最大100MB
		MaxBackups: 20,                      // 最多保留20个备份文件
		MaxAge:     0,                       // 不按时间删除，只按数量删除
		Compress:   true,                    // 压缩旧日志文件
		LocalTime:  true,                    // 使用本地时间
	}

	// 创建核心，同时输出到控制台和文件
	core := zapcore.NewTee(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(rollingFile),
			zapcore.InfoLevel,
		),
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.AddSync(os.Stdout),
			zapcore.InfoLevel,
		),
	)

	// 创建日志器
	logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
}

// Sync 同步日志
func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}

// Info 记录信息日志
func Info(msg string, fields ...zap.Field) {
	if logger != nil {
		logger.Info(msg, fields...)
	}
}

// Error 记录错误日志
func Error(msg string, fields ...zap.Field) {
	if logger != nil {
		logger.Error(msg, fields...)
	}
}

// Fatal 记录致命错误日志
func Fatal(msg string, fields ...zap.Field) {
	if logger != nil {
		logger.Fatal(msg, fields...)
	}
}

// Warn 记录警告日志
func Warn(msg string, fields ...zap.Field) {
	if logger != nil {
		logger.Warn(msg, fields...)
	}
}

// Debug 记录调试日志
func Debug(msg string, fields ...zap.Field) {
	if logger != nil {
		logger.Debug(msg, fields...)
	}
}
