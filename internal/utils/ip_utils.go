package utils

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Private IP blocks
var privateBlocks = []net.IPNet{
	{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
	{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},
	{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},
	{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)}, // localhost
}

// DNS cache with expiration
var dnsCache = struct {
	sync.RWMutex
	entries map[string]struct {
		ips       []net.IP
		timestamp time.Time
	}
}{
	entries: make(map[string]struct {
		ips       []net.IP
		timestamp time.Time
	}),
}

// StartDNSCacheCleanup starts a background goroutine to clean up expired DNS cache entries
func StartDNSCacheCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			cleanupDNSCache()
		}
	}()
}

// cleanupDNSCache removes expired entries from the DNS cache
func cleanupDNSCache() {
	dnsCache.Lock()
	defer dnsCache.Unlock()

	now := time.Now()
	for host, entry := range dnsCache.entries {
		if now.Sub(entry.timestamp) > 5*time.Minute {
			delete(dnsCache.entries, host)
		}
	}
}

// IsPrivateIP checks if the given IP address is a private IP
func IsPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	for _, block := range privateBlocks {
		if block.Contains(parsedIP) {
			return true
		}
	}
	return false
}

// GetCachedIPs retrieves cached IP addresses for a host
func GetCachedIPs(host string) ([]net.IP, bool) {
	dnsCache.RLock()
	defer dnsCache.RUnlock()

	if entry, exists := dnsCache.entries[host]; exists {
		// Check if the cache is still valid (5 minutes)
		if time.Since(entry.timestamp) < 5*time.Minute {
			return entry.ips, true
		}
	}
	return nil, false
}

// CacheIPs stores IP addresses in the cache
func CacheIPs(host string, ips []net.IP) {
	dnsCache.Lock()
	defer dnsCache.Unlock()

	dnsCache.entries[host] = struct {
		ips       []net.IP
		timestamp time.Time
	}{
		ips:       ips,
		timestamp: time.Now(),
	}
}

// LookupIPWithCache performs DNS lookup with caching
func LookupIPWithCache(host string) ([]net.IP, error) {
	if ips, found := GetCachedIPs(host); found {
		return ips, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	CacheIPs(host, ips)
	return ips, nil
}

// GetClientIP extracts the client's real IP address from the request
func GetClientIP(r *http.Request) string {
	clientIP := r.Header.Get("X-Real-IP")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
	}
	return clientIP
}

// ShouldUseProxy determines whether to use proxy based on the target URL
func ShouldUseProxy(host string) bool {
	ips, err := LookupIPWithCache(host)
	if err != nil {
		// If DNS lookup fails, default to using proxy
		return true
	}

	// If any IP is private, don't use proxy
	for _, ip := range ips {
		if IsPrivateIP(ip.String()) {
			return false
		}
	}

	return true
}
