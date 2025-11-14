package config

import (
	"fmt"
	"go-llm-server/pkg/logger"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// ModelRoute 模型路由配置
type ModelRoute struct {
	URLs []string `mapstructure:"urls"`
}

type RateLimitConfig struct {
	Rate  int `mapstructure:"rate"`
	Burst int `mapstructure:"burst"`
}

type Config struct {
	ProxyURL    string                 `mapstructure:"proxy_url"`
	TargetMap   map[string]string      `mapstructure:"target_map"`
	ModelRoutes map[string]interface{} `mapstructure:"model_routes"`
	Port        int                    `mapstructure:"port"`
	RateLimit   RateLimitConfig        `mapstructure:"rate_limit"`
	LogBody     bool                   `mapstructure:"log_body"`
	Database    DatabaseConfig         `mapstructure:"database"`
	Redis       RedisConfig            `mapstructure:"redis"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	DBName          string `mapstructure:"dbname"`
	SSLMode         string `mapstructure:"sslmode"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// LoadConfig 加载配置文件
func LoadConfig(configFile string) (*Config, error) {
	// 如果未指定配置文件，使用默认值
	if configFile == "" {
		configFile = "configs/config.yml"
		logger.Info("loading default config file", zap.String("file", configFile))
	}

	v := viper.New()
	v.SetConfigFile(configFile)

	// 支持 ENV 覆盖，如 DATABASE_HOST → database.host
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.AllowEmptyEnv(true)

	// 读取文件
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	// Manually extract target_map and model_routes from raw YAML to preserve keys with dots
	// Viper treats dots as path separators, so we need to read YAML directly
	var targetMap map[string]string
	var modelRoutes map[string]interface{}
	fileData, err := os.ReadFile(configFile)
	if err == nil {
		var rawConfig map[string]interface{}
		if err := yaml.Unmarshal(fileData, &rawConfig); err == nil {
			// Extract target_map
			if rawTargetMap, ok := rawConfig["target_map"].(map[string]interface{}); ok {
				targetMap = make(map[string]string)
				for k, v := range rawTargetMap {
					if str, ok := v.(string); ok {
						targetMap[k] = str
					}
				}
			}
			// Extract model_routes
			if rawModelRoutes, ok := rawConfig["model_routes"].(map[string]interface{}); ok {
				modelRoutes = make(map[string]interface{})
				for k, v := range rawModelRoutes {
					modelRoutes[k] = v
				}
			}
		}
	}

	// 映射到 struct - use viper's Unmarshal to ensure environment variables are applied
	// First, try to unmarshal without target_map and model_routes to avoid the dot issue
	var cfg Config
	// Use a custom decoder to handle the nested structures with environment variables
	allSettings := v.AllSettings()
	delete(allSettings, "target_map")
	delete(allSettings, "model_routes")

	decoderConfig := &mapstructure.DecoderConfig{
		Result:           &cfg,
		ErrorUnused:      false,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	}
	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(allSettings); err != nil {
		return nil, fmt.Errorf("decoding failed: %w", err)
	}

	// Set the manually extracted target_map and model_routes
	if targetMap != nil {
		cfg.TargetMap = targetMap
	}
	if modelRoutes != nil {
		cfg.ModelRoutes = modelRoutes
	}

	return &cfg, nil
}

type RouteConfig struct {
	URLs []string
}

// GetModelURLs 获取模型的URL列表，支持单个URL和多个URL
func (c *Config) GetModelURLs(model string) ([]string, bool) {
	if route, exists := c.ModelRoutes[model]; exists {
		switch v := route.(type) {
		case string:
			// 单个URL的情况
			return []string{v}, true
		case map[string]interface{}:
			// 多个URL的情况
			if urls, ok := v["urls"]; ok {
				if urlList, ok := urls.([]interface{}); ok {
					result := make([]string, len(urlList))
					for i, url := range urlList {
						if urlStr, ok := url.(string); ok {
							result[i] = urlStr
						}
					}
					return result, true
				}
			}
		}
	}
	return nil, false
}

func (c *Config) HasRateLimit() bool {
	return c.RateLimit.Rate > 0 && c.RateLimit.Burst > 0
}
