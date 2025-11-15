package config

import (
	"os"
	"testing"
)

// TestLoadConfig tests configuration loading functionality
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		configFile  string
		configData  string
		expectError bool
	}{
		{
			name:       "valid YAML config",
			configFile: "test_config.yml",
			configData: `
proxy_url: "http://proxy.example.com:8080"
target_map:
  "gpt-3.5-turbo": "https://api.openai.com/v1"
model_routes:
  "gpt-3.5-turbo": "https://api.openai.com/v1"
  "gpt-4":
    urls:
      - "https://api.openai.com/v1"
      - "https://api.openai.com/v2"
port: 8080
`,
			expectError: false,
		},
		{
			name:        "empty config file",
			configFile:  "empty_config.yml",
			configData:  `{}`,
			expectError: false,
		},
		{
			name:        "non-existent file",
			configFile:  "non_existent.yml",
			configData:  "",
			expectError: true,
		},
		{
			name:       "invalid YAML",
			configFile: "invalid_config.yml",
			configData: `
proxy_url: "http://proxy.example.com:8080"
target_map:
  "gpt-3.5-turbo": "https://api.openai.com/v1"
  "invalid_yaml: [unclosed_bracket
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			if tt.configData != "" {
				err := os.WriteFile(tt.configFile, []byte(tt.configData), 0644)
				if err != nil {
					t.Fatalf("failed to create test config file: %v", err)
				}
				defer os.Remove(tt.configFile)
			}

			// Test LoadConfig
			config, err := LoadConfig(tt.configFile)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if config == nil {
				t.Errorf("expected config but got nil")
			}
		})
	}
}

// TestLoadConfigDefaultFile tests loading with default config file
func TestLoadConfigDefaultFile(t *testing.T) {
	// Test with empty config file (should use default)
	config, err := LoadConfig("")
	if err != nil {
		// This might fail if default config file doesn't exist, which is expected
		t.Logf("expected error when default config file doesn't exist: %v", err)
		return
	}

	if config == nil {
		t.Errorf("expected config but got nil")
	}
}

// TestGetModelURLs tests model URL retrieval functionality
func TestGetModelURLs(t *testing.T) {
	config := &Config{
		ModelRoutes: map[string]interface{}{
			"single-url": "https://api.example.com/v1",
			"multi-url": map[string]interface{}{
				"urls": []interface{}{
					"https://api.example.com/v1",
					"https://api.example.com/v2",
				},
			},
			"invalid-urls": map[string]interface{}{
				"urls": "not-an-array",
			},
			"no-urls": map[string]interface{}{
				"other_field": "value",
			},
		},
	}

	tests := []struct {
		name           string
		model          string
		expectedURLs   []string
		expectedExists bool
	}{
		{
			name:           "single URL model",
			model:          "single-url",
			expectedURLs:   []string{"https://api.example.com/v1"},
			expectedExists: true,
		},
		{
			name:           "multiple URLs model",
			model:          "multi-url",
			expectedURLs:   []string{"https://api.example.com/v1", "https://api.example.com/v2"},
			expectedExists: true,
		},
		{
			name:           "non-existent model",
			model:          "non-existent",
			expectedURLs:   nil,
			expectedExists: false,
		},
		{
			name:           "invalid URLs format",
			model:          "invalid-urls",
			expectedURLs:   nil,
			expectedExists: false,
		},
		{
			name:           "no URLs field",
			model:          "no-urls",
			expectedURLs:   nil,
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls, exists := config.GetModelURLs(tt.model)

			if exists != tt.expectedExists {
				t.Errorf("GetModelURLs(%s) exists = %v, expected %v", tt.model, exists, tt.expectedExists)
			}

			if len(urls) != len(tt.expectedURLs) {
				t.Errorf("GetModelURLs(%s) returned %d URLs, expected %d", tt.model, len(urls), len(tt.expectedURLs))
				return
			}

			for i, url := range urls {
				if url != tt.expectedURLs[i] {
					t.Errorf("GetModelURLs(%s) URL[%d] = %s, expected %s", tt.model, i, url, tt.expectedURLs[i])
				}
			}
		})
	}
}

// TestGetModelURLsWithMixedTypes tests URL retrieval with mixed data types
func TestGetModelURLsWithMixedTypes(t *testing.T) {
	config := &Config{
		ModelRoutes: map[string]interface{}{
			"mixed-urls": map[string]interface{}{
				"urls": []interface{}{
					"https://api.example.com/v1",
					123, // non-string value
					"https://api.example.com/v3",
				},
			},
		},
	}

	urls, exists := config.GetModelURLs("mixed-urls")
	if !exists {
		t.Errorf("expected model to exist")
	}

	expectedURLs := []string{"https://api.example.com/v1", "", "https://api.example.com/v3"}
	if len(urls) != len(expectedURLs) {
		t.Errorf("expected %d URLs, got %d", len(expectedURLs), len(urls))
	}

	for i, url := range urls {
		if url != expectedURLs[i] {
			t.Errorf("URL[%d] = %s, expected %s", i, url, expectedURLs[i])
		}
	}
}

// TestConfigStruct tests Config struct field access
func TestConfigStruct(t *testing.T) {
	config := &Config{
		ProxyURL: "http://proxy.example.com:8080",
		TargetMap: map[string]string{
			"gpt-3.5-turbo": "https://api.openai.com/v1",
		},
		ModelRoutes: map[string]interface{}{
			"test-model": "https://api.example.com/v1",
		},
		Port: 8080,
	}

	if config.ProxyURL != "http://proxy.example.com:8080" {
		t.Errorf("ProxyURL = %s, expected http://proxy.example.com:8080", config.ProxyURL)
	}

	if len(config.TargetMap) != 1 {
		t.Errorf("TargetMap length = %d, expected 1", len(config.TargetMap))
	}

	if config.TargetMap["gpt-3.5-turbo"] != "https://api.openai.com/v1" {
		t.Errorf("TargetMap[gpt-3.5-turbo] = %s, expected https://api.openai.com/v1", config.TargetMap["gpt-3.5-turbo"])
	}

	if len(config.ModelRoutes) != 1 {
		t.Errorf("ModelRoutes length = %d, expected 1", len(config.ModelRoutes))
	}

	if config.Port != 8080 {
		t.Errorf("Port = %d, expected 8080", config.Port)
	}
}

// TestLoadConfigComplexYAML tests loading complex YAML configuration
func TestLoadConfigComplexYAML(t *testing.T) {
	configData := `
proxy_url: "http://proxy.example.com:8080"
target_map:
  "gpt-3.5-turbo": "https://api.openai.com/v1"
  "gpt-4": "https://api.openai.com/v1"
  "claude-3": "https://api.anthropic.com/v1"
model_routes:
  "gpt-3.5-turbo": "https://api.openai.com/v1"
  "gpt-4":
    urls:
      - "https://api.openai.com/v1"
      - "https://api.openai.com/v2"
      - "https://api.openai.com/v3"
  "claude-3":
    urls:
      - "https://api.anthropic.com/v1"
      - "https://api.anthropic.com/v2"
port: 9090
`

	configFile := "complex_config.yml"
	err := os.WriteFile(configFile, []byte(configData), 0644)
	if err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}
	defer os.Remove(configFile)

	config, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Test proxy URL
	if config.ProxyURL != "http://proxy.example.com:8080" {
		t.Errorf("ProxyURL = %s, expected http://proxy.example.com:8080", config.ProxyURL)
	}

	// Test target map
	if len(config.TargetMap) != 3 {
		t.Errorf("TargetMap length = %d, expected 3", len(config.TargetMap))
	}

	// Test model routes
	if len(config.ModelRoutes) != 3 {
		t.Errorf("ModelRoutes length = %d, expected 3", len(config.ModelRoutes))
	}

	// Test port
	if config.Port != 9090 {
		t.Errorf("Port = %d, expected 9090", config.Port)
	}

	// Test GetModelURLs for single URL
	urls, exists := config.GetModelURLs("gpt-3.5-turbo")
	if !exists {
		t.Errorf("expected gpt-3.5-turbo to exist")
	}
	if len(urls) != 1 || urls[0] != "https://api.openai.com/v1" {
		t.Errorf("gpt-3.5-turbo URLs = %v, expected [https://api.openai.com/v1]", urls)
	}

	// Test GetModelURLs for multiple URLs
	urls, exists = config.GetModelURLs("gpt-4")
	if !exists {
		t.Errorf("expected gpt-4 to exist")
	}
	expectedURLs := []string{"https://api.openai.com/v1", "https://api.openai.com/v2", "https://api.openai.com/v3"}
	if len(urls) != len(expectedURLs) {
		t.Errorf("gpt-4 URLs length = %d, expected %d", len(urls), len(expectedURLs))
	}
	for i, url := range urls {
		if url != expectedURLs[i] {
			t.Errorf("gpt-4 URL[%d] = %s, expected %s", i, url, expectedURLs[i])
		}
	}
}

// BenchmarkLoadConfig benchmarks configuration loading performance
func BenchmarkLoadConfig(b *testing.B) {
	configData := `
proxy_url: "http://proxy.example.com:8080"
target_map:
  "gpt-3.5-turbo": "https://api.openai.com/v1"
model_routes:
  "gpt-3.5-turbo": "https://api.openai.com/v1"
port: 8080
`

	configFile := "benchmark_config.yml"
	err := os.WriteFile(configFile, []byte(configData), 0644)
	if err != nil {
		b.Fatalf("failed to create benchmark config file: %v", err)
	}
	defer os.Remove(configFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadConfig(configFile)
		if err != nil {
			b.Fatalf("failed to load config: %v", err)
		}
	}
}

// BenchmarkGetModelURLs benchmarks model URL retrieval performance
func BenchmarkGetModelURLs(b *testing.B) {
	config := &Config{
		ModelRoutes: map[string]interface{}{
			"single-url": "https://api.example.com/v1",
			"multi-url": map[string]interface{}{
				"urls": []interface{}{
					"https://api.example.com/v1",
					"https://api.example.com/v2",
					"https://api.example.com/v3",
				},
			},
		},
	}

	models := []string{"single-url", "multi-url", "non-existent"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model := models[i%len(models)]
		config.GetModelURLs(model)
	}
}

// TestRateLimitConfig tests rate_limit configuration
func TestRateLimitConfig(t *testing.T) {
	t.Run("no rate_limit field", func(t *testing.T) {
		configData := `
port: 8000
`
		configFile := "test_rate_limit_none.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if config.HasRateLimit() {
			t.Errorf("expected HasRateLimit to be false when rate_limit is missing")
		}
	})

	t.Run("rate_limit with zero values", func(t *testing.T) {
		configData := `
port: 8000
rate_limit:
  rate: 0
  burst: 0
`
		configFile := "test_rate_limit_zero.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if config.HasRateLimit() {
			t.Errorf("expected HasRateLimit to be false when rate and burst are 0")
		}
	})

	t.Run("rate_limit with normal values", func(t *testing.T) {
		configData := `
port: 8000
rate_limit:
  rate: 7
  burst: 15
`
		configFile := "test_rate_limit_normal.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if !config.HasRateLimit() {
			t.Errorf("expected HasRateLimit to be true when rate and burst are set")
		}
		if config.RateLimit.Rate != 7 || config.RateLimit.Burst != 15 {
			t.Errorf("unexpected rate_limit values: got rate=%d burst=%d", config.RateLimit.Rate, config.RateLimit.Burst)
		}
	})
}

// TestLoadConfigWithEnvVars tests environment variable overrides
func TestLoadConfigWithEnvVars(t *testing.T) {
	// Save original environment variables
	originalEnv := make(map[string]string)
	envVars := []string{
		"PORT", "PROXY_URL", "LOG_BODY",
		"DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DBNAME",
		"REDIS_ADDR", "REDIS_PASSWORD", "REDIS_DB",
		"RATE_LIMIT_RATE", "RATE_LIMIT_BURST",
	}
	for _, key := range envVars {
		if val := os.Getenv(key); val != "" {
			originalEnv[key] = val
		}
	}

	// Clean up environment variables after test
	defer func() {
		for _, key := range envVars {
			os.Unsetenv(key)
		}
		// Restore original values
		for key, val := range originalEnv {
			os.Setenv(key, val)
		}
	}()

	t.Run("override port with environment variable", func(t *testing.T) {
		configData := `
port: ${PORT:-8000}
`
		configFile := "test_env_port.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		os.Setenv("PORT", "9999")
		defer os.Unsetenv("PORT")

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if config.Port != 9999 {
			t.Errorf("Port = %d, expected 9999 (from environment variable)", config.Port)
		}
	})

	t.Run("override proxy_url with environment variable", func(t *testing.T) {
		configData := `
proxy_url: "${PROXY_URL:-http://default.proxy.com:8080}"
port: 8000
`
		configFile := "test_env_proxy.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		os.Setenv("PROXY_URL", "http://env.proxy.com:9090")
		defer os.Unsetenv("PROXY_URL")

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if config.ProxyURL != "http://env.proxy.com:9090" {
			t.Errorf("ProxyURL = %s, expected http://env.proxy.com:9090 (from environment variable)", config.ProxyURL)
		}
	})

	t.Run("override log_body with environment variable", func(t *testing.T) {
		configData := `
log_body: ${LOG_BODY:-false}
port: 8000
`
		configFile := "test_env_log_body.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		os.Setenv("LOG_BODY", "true")
		defer os.Unsetenv("LOG_BODY")

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if !config.LogBody {
			t.Errorf("LogBody = %v, expected true (from environment variable)", config.LogBody)
		}
	})

	t.Run("override database config with environment variables", func(t *testing.T) {
		configData := `
database:
  host: ${DB_HOST:-127.0.0.1}
  port: ${DB_PORT:-5432}
  user: ${DB_USER:-postgres}
  password: ${DB_PASSWORD:-changme}
  dbname: ${DB_NAME:-postgres}
  sslmode: ${DB_SSLMODE:-disable}
  max_open_conns: ${DB_MAX_OPEN:-10}
  max_idle_conns: ${DB_MAX_IDLE:-5}
  conn_max_lifetime: ${DB_CONN_TTL:-600}
port: 8000
`
		configFile := "test_env_database.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		os.Setenv("DB_HOST", "env.db.example.com")
		os.Setenv("DB_PORT", "3306")
		os.Setenv("DB_USER", "env_user")
		os.Setenv("DB_PASSWORD", "env_password")
		os.Setenv("DB_NAME", "env_database")
		defer func() {
			os.Unsetenv("DB_HOST")
			os.Unsetenv("DB_PORT")
			os.Unsetenv("DB_USER")
			os.Unsetenv("DB_PASSWORD")
			os.Unsetenv("DB_DBNAME")
		}()

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if config.Database.Host != "env.db.example.com" {
			t.Errorf("Database.Host = %s, expected env.db.example.com", config.Database.Host)
		}
		if config.Database.Port != 3306 {
			t.Errorf("Database.Port = %d, expected 3306", config.Database.Port)
		}
		if config.Database.User != "env_user" {
			t.Errorf("Database.User = %s, expected env_user", config.Database.User)
		}
		if config.Database.Password != "env_password" {
			t.Errorf("Database.Password = %s, expected env_password", config.Database.Password)
		}
		if config.Database.DBName != "env_database" {
			t.Errorf("Database.DBName = %s, expected env_database", config.Database.DBName)
		}
	})

	t.Run("override redis config with environment variables", func(t *testing.T) {
		configData := `
redis:
  addr: ${REDIS_ADDR:-127.0.0.1:6379}
  password: ${REDIS_PASSWORD:-changme}
  db: ${REDIS_DB:-0}
port: 8000
`
		configFile := "test_env_redis.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		os.Setenv("REDIS_ADDR", "env.redis.example.com:6380")
		os.Setenv("REDIS_PASSWORD", "env_redis_password")
		os.Setenv("REDIS_DB", "5")
		defer func() {
			os.Unsetenv("REDIS_ADDR")
			os.Unsetenv("REDIS_PASSWORD")
			os.Unsetenv("REDIS_DB")
		}()

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if config.Redis.Addr != "env.redis.example.com:6380" {
			t.Errorf("Redis.Addr = %s, expected env.redis.example.com:6380", config.Redis.Addr)
		}
		if config.Redis.Password != "env_redis_password" {
			t.Errorf("Redis.Password = %s, expected env_redis_password", config.Redis.Password)
		}
		if config.Redis.DB != 5 {
			t.Errorf("Redis.DB = %d, expected 5", config.Redis.DB)
		}
	})

	t.Run("override rate_limit with environment variables", func(t *testing.T) {
		configData := `
rate_limit:
  rate: ${RATE_LIMIT_RATE:-1000}
  burst: ${RATE_LIMIT_BURST:-1000}
port: 8000
`
		configFile := "test_env_rate_limit.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		os.Setenv("RATE_LIMIT_RATE", "500")
		os.Setenv("RATE_LIMIT_BURST", "1000")
		defer func() {
			os.Unsetenv("RATE_LIMIT_RATE")
			os.Unsetenv("RATE_LIMIT_BURST")
		}()

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if config.RateLimit.Rate != 500 {
			t.Errorf("RateLimit.Rate = %d, expected 500", config.RateLimit.Rate)
		}
		if config.RateLimit.Burst != 1000 {
			t.Errorf("RateLimit.Burst = %d, expected 1000", config.RateLimit.Burst)
		}
		if !config.HasRateLimit() {
			t.Errorf("expected HasRateLimit to be true")
		}
	})

	t.Run("environment variable overrides config file value", func(t *testing.T) {
		configData := `
port: ${PORT:-8000}
proxy_url: "${PROXY_URL:-http://default.proxy.com:8080}"
log_body: ${LOG_BODY:-false}
`
		configFile := "test_env_override.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		os.Setenv("PORT", "7777")
		os.Setenv("PROXY_URL", "http://env.proxy.com:9999")
		os.Setenv("LOG_BODY", "true")
		defer func() {
			os.Unsetenv("PORT")
			os.Unsetenv("PROXY_URL")
			os.Unsetenv("LOG_BODY")
		}()

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		// Environment variables should override config file values
		if config.Port != 7777 {
			t.Errorf("Port = %d, expected 7777 (from env, not 8000 from config)", config.Port)
		}
		if config.ProxyURL != "http://env.proxy.com:9999" {
			t.Errorf("ProxyURL = %s, expected http://env.proxy.com:9999 (from env)", config.ProxyURL)
		}
		if !config.LogBody {
			t.Errorf("LogBody = %v, expected true (from env, not false from config)", config.LogBody)
		}
	})

	t.Run("config file value used when environment variable not set", func(t *testing.T) {
		configData := `
port: 8888
proxy_url: "http://config.proxy.com:8080"
log_body: true
`
		configFile := "test_env_no_override.yml"
		if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
			t.Fatalf("failed to create test config file: %v", err)
		}
		defer os.Remove(configFile)

		// Ensure environment variables are not set
		os.Unsetenv("PORT")
		os.Unsetenv("PROXY_URL")
		os.Unsetenv("LOG_BODY")

		config, err := LoadConfig(configFile)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		// Config file values should be used when env vars are not set
		if config.Port != 8888 {
			t.Errorf("Port = %d, expected 8888 (from config file)", config.Port)
		}
		if config.ProxyURL != "http://config.proxy.com:8080" {
			t.Errorf("ProxyURL = %s, expected http://config.proxy.com:8080 (from config file)", config.ProxyURL)
		}
		if !config.LogBody {
			t.Errorf("LogBody = %v, expected true (from config file)", config.LogBody)
		}
	})
}
