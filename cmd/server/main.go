package main

import (
	"flag"
	"fmt"
	"go-llm-server/internal/config"
	"go-llm-server/internal/proxy"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/logger"
	"net/http"

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
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: handler,
	}

	logger.Info("Server starting...", zap.Int("binding port", cfg.Port))
	if err := server.ListenAndServe(); err != nil {
		logger.Fatal("Server failed to start", zap.Error(err))
	}
}
