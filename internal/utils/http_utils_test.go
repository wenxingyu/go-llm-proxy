package utils

import (
	"fmt"
	"net/http"
	"testing"
)

// TestGetTargetURLWithCache test URL construction with cache
func TestGetTargetURLWithCache(t *testing.T) {
	// Clear cache
	urlCache.Flush()

	tests := []struct {
		name        string
		baseURL     string
		path        string
		expected    string
		expectError bool
	}{
		{
			name:        "Basic URL construction",
			baseURL:     "https://api.example.com",
			path:        "/v1/chat/completions",
			expected:    "https://api.example.com/v1/chat/completions",
			expectError: false,
		},
		{
			name:        "URL with port",
			baseURL:     "https://api.example.com:8080",
			path:        "/api/v1/models",
			expected:    "https://api.example.com:8080/api/v1/models",
			expectError: false,
		},
		{
			name:        "URL with query params",
			baseURL:     "https://api.example.com?version=1",
			path:        "/chat/completions",
			expected:    "https://api.example.com/chat/completions?version=1",
			expectError: false,
		},
		{
			name:        "Empty path",
			baseURL:     "https://api.example.com",
			path:        "",
			expected:    "https://api.example.com",
			expectError: false,
		},
		{
			name:        "Root path",
			baseURL:     "https://api.example.com",
			path:        "/",
			expected:    "https://api.example.com/",
			expectError: false,
		},
		{
			name:        "Complex path",
			baseURL:     "https://api.example.com",
			path:        "/api/v1/models/text-embedding-ada-002/embeddings",
			expected:    "https://api.example.com/api/v1/models/text-embedding-ada-002/embeddings",
			expectError: false,
		},
		{
			name:        "HTTP protocol",
			baseURL:     "http://localhost:3000",
			path:        "/api/test",
			expected:    "http://localhost:3000/api/test",
			expectError: false,
		},
		{
			name:        "Invalid baseURL",
			baseURL:     "://invalid-url",
			path:        "/test",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Empty baseURL",
			baseURL:     "",
			path:        "/test",
			expected:    "/test",
			expectError: false,
		},
		{
			name:        "baseURL with fragment",
			baseURL:     "https://api.example.com#fragment",
			path:        "/test",
			expected:    "https://api.example.com/test#fragment",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetTargetURLWithCache(tt.baseURL, tt.path)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result.String() != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, result.String())
				}
			}
		})
	}
}

// TestGetTargetURLWithCache_Cache test URL cache
func TestGetTargetURLWithCache_Cache(t *testing.T) {
	urlCache.Flush()
	baseURL := "https://api.example.com"
	path := "/test"
	result1, err := GetTargetURLWithCache(baseURL, path)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	result2, err := GetTargetURLWithCache(baseURL, path)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if result1.String() != result2.String() {
		t.Errorf("cache result mismatch: %s vs %s", result1.String(), result2.String())
	}
	if result1 != result2 {
		t.Errorf("cache did not return the same object")
	}
}

// TestGetTargetURLWithCache_DifferentPaths test cache with different paths
func TestGetTargetURLWithCache_DifferentPaths(t *testing.T) {
	urlCache.Flush()
	baseURL := "https://api.example.com"
	path1 := "/path1"
	path2 := "/path2"
	result1, err := GetTargetURLWithCache(baseURL, path1)
	if err != nil {
		t.Fatalf("first URL failed: %v", err)
	}
	result2, err := GetTargetURLWithCache(baseURL, path2)
	if err != nil {
		t.Fatalf("second URL failed: %v", err)
	}
	if result1.String() == result2.String() {
		t.Errorf("different paths produced the same URL: %s", result1.String())
	}
	expected1 := "https://api.example.com/path1"
	expected2 := "https://api.example.com/path2"
	if result1.String() != expected1 {
		t.Errorf("expected %s, got %s", expected1, result1.String())
	}
	if result2.String() != expected2 {
		t.Errorf("expected %s, got %s", expected2, result2.String())
	}
}

// TestGetTargetURLWithCache_SpecialCharacters test special characters in path
func TestGetTargetURLWithCache_SpecialCharacters(t *testing.T) {
	urlCache.Flush()
	tests := []struct {
		name        string
		baseURL     string
		path        string
		expectError bool
	}{
		{
			name:        "Path with Chinese characters",
			baseURL:     "https://api.example.com",
			path:        "/api/中文路径",
			expectError: false,
		},
		{
			name:        "Path with special characters",
			baseURL:     "https://api.example.com",
			path:        "/api/test%20path",
			expectError: false,
		},
		{
			name:        "Path with query params",
			baseURL:     "https://api.example.com",
			path:        "/api/test?param=value",
			expectError: false,
		},
		{
			name:        "Path with fragment",
			baseURL:     "https://api.example.com",
			path:        "/api/test#fragment",
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetTargetURLWithCache(tt.baseURL, tt.path)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result")
				}
			}
		})
	}
}

// TestGetRequestID test request ID extraction
func TestGetRequestID(t *testing.T) {
	tests := []struct {
		name        string
		requestID   string
		expectEmpty bool
	}{
		{
			name:        "Valid request ID",
			requestID:   "req-12345-abcde",
			expectEmpty: false,
		},
		{
			name:        "Empty request ID",
			requestID:   "",
			expectEmpty: true,
		},
		{
			name:        "UUID request ID",
			requestID:   "550e8400-e29b-41d4-a716-446655440000",
			expectEmpty: false,
		},
		{
			name:        "Numeric request ID",
			requestID:   "123456789",
			expectEmpty: false,
		},
		{
			name:        "Request ID with special characters",
			requestID:   "req_123-456@test",
			expectEmpty: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: make(http.Header),
			}
			if tt.requestID != "" {
				req.Header.Set("X-Request-ID", tt.requestID)
			}
			result := GetRequestID(req)
			if tt.expectEmpty {
				if result != "" {
					t.Errorf("expected empty string, got %s", result)
				}
			} else {
				if result != tt.requestID {
					t.Errorf("expected %s, got %s", tt.requestID, result)
				}
			}
		})
	}
}

// TestGetRequestID_NilRequest test nil request
func TestGetRequestID_NilRequest(t *testing.T) {
	result := GetRequestID(nil)
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

// TestGetRequestID_EmptyHeader test empty header
func TestGetRequestID_EmptyHeader(t *testing.T) {
	req := &http.Request{
		Header: make(http.Header),
	}
	result := GetRequestID(req)
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

// TestGetRequestID_CaseInsensitive test case-insensitive header
func TestGetRequestID_CaseInsensitive(t *testing.T) {
	req := &http.Request{
		Header: make(http.Header),
	}
	req.Header.Set("x-request-id", "test-id")
	result := GetRequestID(req)
	if result != "test-id" {
		t.Errorf("expected test-id, got %s", result)
	}
}

// TestGetRequestID_MultipleHeaders test multiple headers
func TestGetRequestID_MultipleHeaders(t *testing.T) {
	req := &http.Request{
		Header: make(http.Header),
	}
	req.Header.Set("X-Request-ID", "primary-id")
	req.Header.Set("x-request-id", "secondary-id")
	result := GetRequestID(req)
	if result != "secondary-id" {
		t.Errorf("expected secondary-id, got %s", result)
	}
}

// TestURLCacheConcurrency test URL cache concurrency
func TestURLCacheConcurrency(t *testing.T) {
	urlCache.Flush()
	const numGoroutines = 10
	const numOperations = 100
	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOperations; j++ {
				baseURL := "https://api.example.com"
				path := fmt.Sprintf("/test%d", id*numOperations+j)
				_, err := GetTargetURLWithCache(baseURL, path)
				if err != nil {
					t.Errorf("concurrent URL build failed: %v", err)
				}
			}
			done <- true
		}(i)
	}
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
	itemCount := urlCache.ItemCount()
	expectedCount := numGoroutines * numOperations
	if itemCount != expectedCount {
		t.Errorf("expected %d cache items, got %d", expectedCount, itemCount)
	}
}

// TestURLCachePerformance test URL cache performance
func TestURLCachePerformance(t *testing.T) {
	urlCache.Flush()
	baseURL := "https://api.example.com"
	path := "/performance/test"
	_, err := GetTargetURLWithCache(baseURL, path)
	if err != nil {
		t.Fatalf("cache warmup failed: %v", err)
	}
	for i := 0; i < 1000; i++ {
		_, err := GetTargetURLWithCache(baseURL, path)
		if err != nil {
			t.Fatalf("cache hit test failed: %v", err)
		}
	}
}

// Benchmark tests
func BenchmarkGetTargetURLWithCache(b *testing.B) {
	urlCache.Flush()
	baseURL := "https://api.example.com"
	path := "/benchmark/test"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetTargetURLWithCache(baseURL, path)
		if err != nil {
			b.Fatalf("benchmark failed: %v", err)
		}
	}
}

func BenchmarkGetTargetURLWithCache_Cached(b *testing.B) {
	urlCache.Flush()
	baseURL := "https://api.example.com"
	path := "/benchmark/cached"
	_, err := GetTargetURLWithCache(baseURL, path)
	if err != nil {
		b.Fatalf("cache warmup failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetTargetURLWithCache(baseURL, path)
		if err != nil {
			b.Fatalf("cache benchmark failed: %v", err)
		}
	}
}

func BenchmarkGetTargetURLWithCache_DifferentPaths(b *testing.B) {
	urlCache.Flush()
	baseURL := "https://api.example.com"
	paths := []string{"/path1", "/path2", "/path3", "/path4", "/path5"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := paths[i%len(paths)]
		_, err := GetTargetURLWithCache(baseURL, path)
		if err != nil {
			b.Fatalf("different paths benchmark failed: %v", err)
		}
	}
}

func BenchmarkGetTargetURLWithCache_ComplexURLs(b *testing.B) {
	urlCache.Flush()
	baseURLs := []string{"https://api.example.com:8080", "https://api.example.com:8443", "https://api.example.com:3000"}
	paths := []string{"/api/v1/chat/completions", "/api/v1/models/text-embedding-ada-002/embeddings", "/api/v1/files"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		baseURL := baseURLs[i%len(baseURLs)]
		path := paths[i%len(paths)]
		_, err := GetTargetURLWithCache(baseURL, path)
		if err != nil {
			b.Fatalf("complex URLs benchmark failed: %v", err)
		}
	}
}

func BenchmarkGetRequestID(b *testing.B) {
	req := &http.Request{
		Header: make(http.Header),
	}
	req.Header.Set("X-Request-ID", "benchmark-test-id")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetRequestID(req)
	}
}

func BenchmarkGetRequestID_Empty(b *testing.B) {
	req := &http.Request{
		Header: make(http.Header),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetRequestID(req)
	}
}

func BenchmarkGetRequestID_Nil(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetRequestID(nil)
	}
}

func BenchmarkGetRequestID_LongID(b *testing.B) {
	req := &http.Request{
		Header: make(http.Header),
	}
	req.Header.Set("X-Request-ID", "very-long-request-id-that-might-be-a-uuid-or-some-other-long-identifier")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetRequestID(req)
	}
}

// Memory allocation benchmarks
func BenchmarkGetTargetURLWithCache_Memory(b *testing.B) {
	urlCache.Flush()
	baseURL := "https://api.example.com"
	path := "/memory/test"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetTargetURLWithCache(baseURL, path)
		if err != nil {
			b.Fatalf("memory benchmark failed: %v", err)
		}
	}
}

func BenchmarkGetRequestID_Memory(b *testing.B) {
	req := &http.Request{
		Header: make(http.Header),
	}
	req.Header.Set("X-Request-ID", "memory-test-id")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetRequestID(req)
	}
}

// TestGetOrGenerateRequestID tests the GetOrGenerateRequestID function
func TestGetOrGenerateRequestID(t *testing.T) {
	tests := []struct {
		name           string
		requestID      string
		expectGenerate bool
	}{
		{
			name:           "Existing request ID",
			requestID:      "existing-request-id-123",
			expectGenerate: false,
		},
		{
			name:           "Empty request ID",
			requestID:      "",
			expectGenerate: true,
		},
		{
			name:           "Whitespace request ID",
			requestID:      "   ",
			expectGenerate: true, // HTTP headers preserve whitespace, so this will be treated as existing
		},
		{
			name:           "Long request ID",
			requestID:      "very-long-request-id-that-exceeds-normal-length-limits-for-testing-purposes",
			expectGenerate: false,
		},
		{
			name:           "Special characters request ID",
			requestID:      "request-id-with-special-chars!@#$%^&*()",
			expectGenerate: false,
		},
		{
			name:           "UUID format request ID",
			requestID:      "550e8400-e29b-41d4-a716-446655440000",
			expectGenerate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/test", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tt.requestID != "" {
				req.Header.Set("X-Request-ID", tt.requestID)
			}

			result := GetOrGenerateRequestID(req)

			if tt.expectGenerate {
				// Should generate a new UUID
				if result == "" {
					t.Errorf("Expected generated UUID, got empty string")
				}
				if result == tt.requestID {
					t.Errorf("Expected new UUID, got original request ID: %s", result)
				}
				// Check if it's a valid UUID format
				if len(result) != 36 {
					t.Errorf("Expected UUID length 36, got %d: %s", len(result), result)
				}
				// Verify UUID format (8-4-4-4-12)
				if !isValidUUID(result) {
					t.Errorf("Generated ID is not a valid UUID: %s", result)
				}
				// Check if header was set
				if req.Header.Get("X-Request-ID") != result {
					t.Errorf("Header not set correctly, expected %s, got %s", result, req.Header.Get("X-Request-ID"))
				}
			} else {
				// Should return existing request ID
				if result != tt.requestID {
					t.Errorf("Expected %s, got %s", tt.requestID, result)
				}
				// Header should remain unchanged
				if req.Header.Get("X-Request-ID") != tt.requestID {
					t.Errorf("Header should remain unchanged, expected %s, got %s", tt.requestID, req.Header.Get("X-Request-ID"))
				}
			}
		})
	}
}

// TestGetOrGenerateRequestID_NilRequest tests behavior with nil request
func TestGetOrGenerateRequestID_NilRequest(t *testing.T) {
	result := GetOrGenerateRequestID(nil)
	if result == "" {
		t.Errorf("Expected generated UUID for nil request, got empty string")
	}
	if !isValidUUID(result) {
		t.Errorf("Generated ID for nil request is not a valid UUID: %s", result)
	}
}

// TestGetOrGenerateRequestID_CaseInsensitive tests case insensitivity of header
func TestGetOrGenerateRequestID_CaseInsensitive(t *testing.T) {
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Test different case variations
	testCases := []string{
		"x-request-id",
		"X-Request-Id",
		"X-REQUEST-ID",
		"x-Request-Id",
	}

	for _, headerName := range testCases {
		t.Run("Case_"+headerName, func(t *testing.T) {
			req.Header.Set(headerName, "test-request-id")
			result := GetOrGenerateRequestID(req)
			if result != "test-request-id" {
				t.Errorf("Expected 'test-request-id', got %s for header %s", result, headerName)
			}
			// Clear header for next test
			req.Header.Del(headerName)
		})
	}
}

// TestGetOrGenerateRequestID_MultipleCalls tests multiple calls on same request
func TestGetOrGenerateRequestID_MultipleCalls(t *testing.T) {
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// First call should generate UUID
	result1 := GetOrGenerateRequestID(req)
	if result1 == "" {
		t.Fatalf("First call should generate UUID")
	}

	// Second call should return same UUID
	result2 := GetOrGenerateRequestID(req)
	if result2 != result1 {
		t.Errorf("Second call should return same ID, expected %s, got %s", result1, result2)
	}

	// Third call should also return same UUID
	result3 := GetOrGenerateRequestID(req)
	if result3 != result1 {
		t.Errorf("Third call should return same ID, expected %s, got %s", result1, result3)
	}
}

// TestGetOrGenerateRequestID_ExistingID tests with existing request ID
func TestGetOrGenerateRequestID_ExistingID(t *testing.T) {
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	existingID := "existing-request-id-12345"
	req.Header.Set("X-Request-ID", existingID)

	result := GetOrGenerateRequestID(req)
	if result != existingID {
		t.Errorf("Expected existing ID %s, got %s", existingID, result)
	}

	// Verify header was not modified
	if req.Header.Get("X-Request-ID") != existingID {
		t.Errorf("Header should not be modified, expected %s, got %s", existingID, req.Header.Get("X-Request-ID"))
	}
}

// TestGetOrGenerateRequestID_ConcurrentAccess tests concurrent access to the function
func TestGetOrGenerateRequestID_ConcurrentAccess(t *testing.T) {
	const numGoroutines = 100
	results := make(chan string, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			req, err := http.NewRequest("GET", "/test", nil)
			if err != nil {
				t.Errorf("Failed to create request: %v", err)
				return
			}
			result := GetOrGenerateRequestID(req)
			results <- result
		}()
	}

	// Collect all results
	allResults := make([]string, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		allResults[i] = <-results
	}

	// All results should be valid UUIDs
	for i, result := range allResults {
		if result == "" {
			t.Errorf("Result %d is empty", i)
		}
		if !isValidUUID(result) {
			t.Errorf("Result %d is not a valid UUID: %s", i, result)
		}
	}
}

// isValidUUID checks if a string is a valid UUID format
func isValidUUID(uuid string) bool {
	if len(uuid) != 36 {
		return false
	}
	// Check UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	for i, char := range uuid {
		switch i {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return false
			}
		}
	}
	return true
}
