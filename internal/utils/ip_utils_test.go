package utils

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestIsPrivateIP tests private IP address checking
func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "10.0.0.0/8 network segment",
			ip:       "10.0.0.1",
			expected: true,
		},
		{
			name:     "10.255.255.255 boundary value",
			ip:       "10.255.255.255",
			expected: true,
		},
		{
			name:     "172.16.0.0/12 network segment",
			ip:       "172.16.0.1",
			expected: true,
		},
		{
			name:     "172.31.255.255 boundary value",
			ip:       "172.31.255.255",
			expected: true,
		},
		{
			name:     "192.168.0.0/16 network segment",
			ip:       "192.168.1.1",
			expected: true,
		},
		{
			name:     "192.168.255.255 boundary value",
			ip:       "192.168.255.255",
			expected: true,
		},
		{
			name:     "127.0.0.0/8 network segment",
			ip:       "127.0.0.1",
			expected: true,
		},
		{
			name:     "127.255.255.255 boundary value",
			ip:       "127.255.255.255",
			expected: true,
		},
		{
			name:     "public IP address",
			ip:       "8.8.8.8",
			expected: false,
		},
		{
			name:     "another public IP address",
			ip:       "1.1.1.1",
			expected: false,
		},
		{
			name:     "invalid IP address",
			ip:       "invalid-ip",
			expected: false,
		},
		{
			name:     "empty string",
			ip:       "",
			expected: false,
		},
		{
			name:     "IPv6 private address",
			ip:       "::1",
			expected: false, // current implementation only checks IPv4
		},
		{
			name:     "boundary case - 9.255.255.255",
			ip:       "9.255.255.255",
			expected: false,
		},
		{
			name:     "boundary case - 11.0.0.0",
			ip:       "11.0.0.0",
			expected: false,
		},
		{
			name:     "boundary case - 172.15.255.255",
			ip:       "172.15.255.255",
			expected: false,
		},
		{
			name:     "boundary case - 172.32.0.0",
			ip:       "172.32.0.0",
			expected: false,
		},
		{
			name:     "boundary case - 192.167.255.255",
			ip:       "192.167.255.255",
			expected: false,
		},
		{
			name:     "boundary case - 192.169.0.0",
			ip:       "192.169.0.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPrivateIP(tt.ip)
			if result != tt.expected {
				t.Errorf("IsPrivateIP(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

// TestGetCachedIPs tests DNS cache retrieval functionality
func TestGetCachedIPs(t *testing.T) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	host := "example.com"
	testIPs := []net.IP{net.ParseIP("93.184.216.34")}

	// test empty cache
	ips, found := GetCachedIPs(host)
	if found || len(ips) != 0 {
		t.Errorf("expected no cached IPs")
	}

	// add cache
	CacheIPs(host, testIPs)

	// test with cache
	ips, found = GetCachedIPs(host)
	if !found || len(ips) != 1 || !ips[0].Equal(testIPs[0]) {
		t.Errorf("expected cached IPs")
	}

	// test multiple IPs case
	host2 := "multi.example.com"
	testIPs2 := []net.IP{
		net.ParseIP("93.184.216.34"),
		net.ParseIP("93.184.216.35"),
	}
	CacheIPs(host2, testIPs2)

	ips, found = GetCachedIPs(host2)
	if !found || len(ips) != 2 {
		t.Errorf("expected 2 cached IPs")
	}

	// test empty IP list
	host3 := "empty.example.com"
	CacheIPs(host3, []net.IP{})
	ips, found = GetCachedIPs(host3)
	if !found || len(ips) != 0 {
		t.Errorf("expected empty IP list")
	}
}

// TestCacheIPs tests DNS cache storage functionality
func TestCacheIPs(t *testing.T) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	host := "test.com"
	testIPs := []net.IP{net.ParseIP("8.8.8.8")}

	// test cache storage
	CacheIPs(host, testIPs)

	// verify cache content
	dnsCache.RLock()
	entry, exists := dnsCache.entries[host]
	dnsCache.RUnlock()

	if !exists {
		t.Errorf("expected cache entry to exist")
	}

	if len(entry.ips) != 1 || !entry.ips[0].Equal(testIPs[0]) {
		t.Errorf("expected cached IPs to be correct")
	}

	// test timestamp
	if time.Since(entry.timestamp) > time.Second {
		t.Errorf("expected timestamp to be recent")
	}

	// test overwriting existing cache
	newIPs := []net.IP{net.ParseIP("1.1.1.1")}
	CacheIPs(host, newIPs)

	dnsCache.RLock()
	entry, exists = dnsCache.entries[host]
	dnsCache.RUnlock()

	if !exists {
		t.Errorf("expected cache entry to still exist")
	}

	if len(entry.ips) != 1 || !entry.ips[0].Equal(newIPs[0]) {
		t.Errorf("expected cache to be properly overwritten")
	}
}

// TestLookupIPWithCache tests DNS lookup with cache
func TestLookupIPWithCache(t *testing.T) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	// test valid domain
	host := "google.com"
	ips, err := LookupIPWithCache(host)
	if err != nil {
		t.Skipf("skipping test, network error: %v", err)
	}
	if len(ips) == 0 {
		t.Errorf("expected %s to have valid IPs", host)
	}

	// test cache functionality
	ips2, err := LookupIPWithCache(host)
	if err != nil {
		t.Errorf("expected cached lookup to succeed: %v", err)
	}
	if len(ips2) != len(ips) {
		t.Errorf("expected cached IP count to be the same")
	}

	// verify IP content is the same
	for i, ip := range ips {
		if !ip.Equal(ips2[i]) {
			t.Errorf("expected cached IP content to be the same")
		}
	}

	// test invalid domain
	invalidHost := "invalid-domain-that-does-not-exist-12345.com"
	_, err = LookupIPWithCache(invalidHost)
	if err == nil {
		t.Errorf("expected invalid domain to return error")
	}
}

// TestGetClientIP tests client IP extraction
func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "X-Real-IP highest priority",
			headers:    map[string]string{"X-Real-IP": "192.168.1.100", "X-Forwarded-For": "203.0.113.1"},
			remoteAddr: "172.16.0.1:8080",
			expected:   "192.168.1.100",
		},
		{
			name:       "X-Forwarded-For second priority",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remoteAddr: "192.168.1.1:8080",
			expected:   "203.0.113.1",
		},
		{
			name:       "RemoteAddr last priority",
			headers:    map[string]string{},
			remoteAddr: "8.8.8.8:12345",
			expected:   "8.8.8.8:12345",
		},
		{
			name:       "empty RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "",
			expected:   "",
		},
		{
			name:       "X-Real-IP empty string",
			headers:    map[string]string{"X-Real-IP": "", "X-Forwarded-For": "203.0.113.1"},
			remoteAddr: "192.168.1.1:8080",
			expected:   "203.0.113.1",
		},
		{
			name:       "X-Forwarded-For empty string",
			headers:    map[string]string{"X-Forwarded-For": ""},
			remoteAddr: "8.8.8.8:12345",
			expected:   "8.8.8.8:12345",
		},
		{
			name:       "IPv6 RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "[::1]:8080",
			expected:   "[::1]:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header:     make(http.Header),
				RemoteAddr: tt.remoteAddr,
			}
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			result := GetClientIP(req)
			if result != tt.expected {
				t.Errorf("GetClientIP() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestShouldUseProxy tests proxy usage determination
func TestShouldUseProxy(t *testing.T) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{
			name:     "public domain should use proxy",
			host:     "google.com",
			expected: true,
		},
		{
			name:     "localhost should not use proxy",
			host:     "localhost",
			expected: false,
		},
		{
			name:     "127.0.0.1 should not use proxy",
			host:     "127.0.0.1",
			expected: false,
		},
		{
			name:     "10.0.0.1 should not use proxy",
			host:     "10.0.0.1",
			expected: false,
		},
		{
			name:     "192.168.1.1 should not use proxy",
			host:     "192.168.1.1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldUseProxy(tt.host)
			if result != tt.expected {
				t.Errorf("ShouldUseProxy(%s) = %v, expected %v", tt.host, result, tt.expected)
			}
		})
	}

	// test DNS lookup failure case
	// simulate DNS lookup failure
	invalidHost := "invalid-domain-that-does-not-exist-12345.com"
	result := ShouldUseProxy(invalidHost)
	if !result {
		t.Errorf("should default to using proxy when DNS lookup fails")
	}
}

// TestDNSCacheCleanup tests DNS cache cleanup
func TestDNSCacheCleanup(t *testing.T) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	host1 := "test1.com"
	host2 := "test2.com"
	host3 := "test3.com"
	testIPs := []net.IP{net.ParseIP("8.8.8.8")}

	// add normal cache
	CacheIPs(host1, testIPs)

	// add cache that will expire soon
	dnsCache.Lock()
	dnsCache.entries[host2] = struct {
		ips       []net.IP
		timestamp time.Time
	}{
		ips:       testIPs,
		timestamp: time.Now().Add(-4 * time.Minute), // 4 minutes ago, should be kept
	}
	dnsCache.Unlock()

	// add expired cache
	dnsCache.Lock()
	dnsCache.entries[host3] = struct {
		ips       []net.IP
		timestamp time.Time
	}{
		ips:       testIPs,
		timestamp: time.Now().Add(-10 * time.Minute), // 10 minutes ago, should be deleted
	}
	dnsCache.Unlock()

	// perform cleanup
	cleanupDNSCache()

	// verify results
	_, found1 := GetCachedIPs(host1)
	if !found1 {
		t.Errorf("expected host1 to still be cached")
	}

	_, found2 := GetCachedIPs(host2)
	if !found2 {
		t.Errorf("expected host2 to still be cached (4 minutes ago)")
	}

	_, found3 := GetCachedIPs(host3)
	if found3 {
		t.Errorf("expected host3 to be deleted (10 minutes ago)")
	}
}

// TestStartDNSCacheCleanup tests DNS cache cleanup startup
func TestStartDNSCacheCleanup(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("StartDNSCacheCleanup() panicked: %v", r)
		}
	}()

	// test multiple startups
	StartDNSCacheCleanup()
	time.Sleep(100 * time.Millisecond)

	StartDNSCacheCleanup()
	time.Sleep(100 * time.Millisecond)
}

// TestConcurrentCacheAccess tests concurrent cache access
func TestConcurrentCacheAccess(t *testing.T) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	const numGoroutines = 10
	const numOperations = 100

	// concurrent writes
	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOperations; j++ {
				host := fmt.Sprintf("test%d.com", id*numOperations+j)
				ips := []net.IP{net.ParseIP("8.8.8.8")}
				CacheIPs(host, ips)
			}
			done <- true
		}(i)
	}

	// wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// verify cache entry count
	dnsCache.RLock()
	entryCount := len(dnsCache.entries)
	dnsCache.RUnlock()

	expectedCount := numGoroutines * numOperations
	if entryCount != expectedCount {
		t.Errorf("expected %d cache entries, got %d", expectedCount, entryCount)
	}
}

// TestCacheExpiration tests cache expiration mechanism
func TestCacheExpiration(t *testing.T) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	host := "expiration-test.com"
	testIPs := []net.IP{net.ParseIP("8.8.8.8")}

	// add cache
	CacheIPs(host, testIPs)

	// check immediately, should exist
	ips, found := GetCachedIPs(host)
	if !found || len(ips) != 1 {
		t.Errorf("expected cache to be effective immediately")
	}

	// simulate time passing 6 minutes (exceeds 5 minute expiration time)
	dnsCache.Lock()
	entry := dnsCache.entries[host]
	entry.timestamp = time.Now().Add(-6 * time.Minute)
	dnsCache.entries[host] = entry
	dnsCache.Unlock()

	// check again, should be expired
	_, found = GetCachedIPs(host)
	if found {
		t.Errorf("expected cache to be expired")
	}
}

// 基准测试
func BenchmarkIsPrivateIP(b *testing.B) {
	testIPs := []string{
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"127.0.0.1",
		"8.8.8.8",
		"1.1.1.1",
		"203.0.113.1",
		"198.51.100.1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsPrivateIP(testIPs[i%len(testIPs)])
	}
}

func BenchmarkIsPrivateIP_PrivateOnly(b *testing.B) {
	testIPs := []string{
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"127.0.0.1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsPrivateIP(testIPs[i%len(testIPs)])
	}
}

func BenchmarkIsPrivateIP_PublicOnly(b *testing.B) {
	testIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"203.0.113.1",
		"198.51.100.1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsPrivateIP(testIPs[i%len(testIPs)])
	}
}

func BenchmarkLookupIPWithCache(b *testing.B) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	host := "google.com"

	// perform one lookup first to populate cache
	_, err := LookupIPWithCache(host)
	if err != nil {
		b.Skipf("skipping benchmark, network error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LookupIPWithCache(host)
	}
}

func BenchmarkLookupIPWithCache_NoCache(b *testing.B) {
	// clear cache
	dnsCache.Lock()
	dnsCache.entries = make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	})
	dnsCache.Unlock()

	hosts := []string{"google.com", "github.com", "stackoverflow.com"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		host := hosts[i%len(hosts)]
		LookupIPWithCache(host)
	}
}

func BenchmarkGetClientIP(b *testing.B) {
	req := &http.Request{
		Header:     make(http.Header),
		RemoteAddr: "192.168.1.1:8080",
	}
	req.Header.Set("X-Real-IP", "203.0.113.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetClientIP(req)
	}
}

func BenchmarkGetClientIP_RemoteAddrOnly(b *testing.B) {
	req := &http.Request{
		Header:     make(http.Header),
		RemoteAddr: "192.168.1.1:8080",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetClientIP(req)
	}
}

func BenchmarkGetClientIP_XForwardedFor(b *testing.B) {
	req := &http.Request{
		Header:     make(http.Header),
		RemoteAddr: "192.168.1.1:8080",
	}
	req.Header.Set("X-Forwarded-For", "203.0.113.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetClientIP(req)
	}
}

func BenchmarkShouldUseProxy(b *testing.B) {
	hosts := []string{"google.com", "github.com", "localhost", "127.0.0.1"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ShouldUseProxy(hosts[i%len(hosts)])
	}
}

func BenchmarkCacheIPs(b *testing.B) {
	hosts := make([]string, b.N)
	ips := []net.IP{net.ParseIP("8.8.8.8")}

	for i := 0; i < b.N; i++ {
		hosts[i] = fmt.Sprintf("test%d.com", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CacheIPs(hosts[i], ips)
	}
}

func BenchmarkGetCachedIPs(b *testing.B) {
	// prepare test data
	host := "benchmark-test.com"
	ips := []net.IP{net.ParseIP("8.8.8.8")}
	CacheIPs(host, ips)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetCachedIPs(host)
	}
}

func BenchmarkDNSCacheCleanup(b *testing.B) {
	// prepare test data
	testIPs := []net.IP{net.ParseIP("8.8.8.8")}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// clear cache
		dnsCache.Lock()
		dnsCache.entries = make(map[string]struct {
			ips       []net.IP
			timestamp time.Time
		})
		dnsCache.Unlock()

		// add some test data
		for j := 0; j < 100; j++ {
			host := fmt.Sprintf("test%d.com", j)
			if j%2 == 0 {
				// normal cache
				CacheIPs(host, testIPs)
			} else {
				// expired cache
				dnsCache.Lock()
				dnsCache.entries[host] = struct {
					ips       []net.IP
					timestamp time.Time
				}{
					ips:       testIPs,
					timestamp: time.Now().Add(-10 * time.Minute),
				}
				dnsCache.Unlock()
			}
		}

		// perform cleanup
		cleanupDNSCache()
	}
}
