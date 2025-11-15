package config

import (
	"go-llm-server/pkg/logger"
	"os"
	"regexp"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ModelRoute 模型路由配置
type ModelRoute struct {
	URLs []string `yaml:"urls"`
}

type RateLimitConfig struct {
	Rate  int `yaml:"rate"`
	Burst int `yaml:"burst"`
}

// Config 应用配置结构
type Config struct {
	ProxyURL    string                 `yaml:"proxy_url"`
	TargetMap   map[string]string      `yaml:"target_map"`
	ModelRoutes map[string]interface{} `yaml:"model_routes"` // 支持字符串或ModelRoute
	Port        int                    `yaml:"port"`
	RateLimit   RateLimitConfig        `yaml:"rate_limit"`
	LogBody     bool                   `yaml:"log_body"` // 是否记录请求体
	Database    DatabaseConfig         `yaml:"database"`
	Redis       RedisConfig            `yaml:"redis"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	DBName          string `yaml:"dbname"`
	SSLMode         string `yaml:"sslmode"`           // disable, require, verify-ca, verify-full
	MaxOpenConns    int    `yaml:"max_open_conns"`    // 最大打开连接数
	MaxIdleConns    int    `yaml:"max_idle_conns"`    // 最大空闲连接数
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"` // 连接最长生命周期（秒）
}

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

var envPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(:-([^}]+))?}`)

// expandEnvWithDefault implements ${VAR:-default}
func expandEnvWithDefault(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		groups := envPattern.FindStringSubmatch(m)

		key := groups[1]
		def := groups[3] // may be empty

		val, exists := os.LookupEnv(key)
		if exists && val != "" {
			return val
		}
		return def
	})
}

// LoadConfig 加载配置文件
func LoadConfig(configFile string) (*Config, error) {
	// 如果未指定配置文件，使用默认值
	if configFile == "" {
		configFile = "configs/config.yml"
		logger.Info("loading default config file", zap.String("file", configFile))
	}

	file, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	// Expand env vars with default
	expanded := expandEnvWithDefault(string(file))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, err
	}

	return &config, nil
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
