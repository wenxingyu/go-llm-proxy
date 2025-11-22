package main

import (
	"flag"
	"fmt"
	"go-llm-server/internal/config"
	"go-llm-server/internal/proxy"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/logger"
	"net/http"
	"time"

	"go.uber.org/zap"
)

var Version string
var BuildTime string

func main() {
	fmt.Printf("Version: %s, BuildTime: %s\n", Version, BuildTime)
	configFile := flag.String("f", "", "path to config file (default: configs/config.yml)")
	flag.Parse()

	// 初始化日志
	logger.InitLogger()
	defer func() {
		err := logger.Sync()
		if err != nil {
			logger.Warn("Failed to sync logger", zap.Error(err))
		}
	}()

	// 加载配置
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// 初始化代理传输层
	proxy.InitHttpProxyTransport(cfg)

	// 启动DNS缓存清理
	utils.StartDNSCacheCleanup()

	// 创建代理处理器
	handler := proxy.NewHandler(cfg)

	handler.InitLoadBalancers()

	// 创建HTTP服务器
	// 设置超时时间，确保与 transport 的 ResponseHeaderTimeout (900秒) 相匹配
	// ReadTimeout: 读取整个请求（包括body）的最大时间
	// WriteTimeout: 写入响应的最大时间
	// ReadHeaderTimeout: 读取请求头的最大时间
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadTimeout:       900 * time.Second, // 读取整个请求的最大时间
		WriteTimeout:      900 * time.Second, // 写入响应的最大时间
		ReadHeaderTimeout: 10 * time.Second,  // 读取请求头的最大时间
		IdleTimeout:       30 * time.Second,  // 空闲连接的超时时间
	}

	logger.Info("Server starting...", zap.Int("binding port", cfg.Port))
	if err := server.ListenAndServe(); err != nil {
		logger.Fatal("Server failed to start", zap.Error(err))
	}
}
