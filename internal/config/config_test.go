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
