package proxy

import (
	"sync"
	"sync/atomic"
)

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	GetNext() string
	GetURLs() []string
}

// RoundRobinLoadBalancer RoundRobin负载均衡器
type RoundRobinLoadBalancer struct {
	urls    []string
	current uint64
	mu      sync.RWMutex
}

// NewRoundRobinLoadBalancer 创建新的RoundRobin负载均衡器
func NewRoundRobinLoadBalancer(urls []string) *RoundRobinLoadBalancer {
	return &RoundRobinLoadBalancer{
		urls:    urls,
		current: 0,
	}
}

// GetNext 获取下一个URL
func (rr *RoundRobinLoadBalancer) GetNext() string {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	if len(rr.urls) == 0 {
		return ""
	}

	// 使用轮询选择URL
	current := atomic.AddUint64(&rr.current, 1) - 1
	index := int(current % uint64(len(rr.urls)))

	return rr.urls[index]
}

// GetURLs 获取所有URL
func (rr *RoundRobinLoadBalancer) GetURLs() []string {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	result := make([]string, len(rr.urls))
	copy(result, rr.urls)
	return result
}

// LoadBalancerManager 负载均衡器管理器
type LoadBalancerManager struct {
	balancers map[string]LoadBalancer
	mu        sync.RWMutex
}

// NewLoadBalancerManager 创建负载均衡器管理器
func NewLoadBalancerManager() *LoadBalancerManager {
	return &LoadBalancerManager{
		balancers: make(map[string]LoadBalancer),
	}
}

// AddLoadBalancer 添加负载均衡器
func (lbm *LoadBalancerManager) AddLoadBalancer(key string, urls []string) {
	lbm.mu.Lock()
	defer lbm.mu.Unlock()

	balancer := NewRoundRobinLoadBalancer(urls)
	lbm.balancers[key] = balancer
}

// GetLoadBalancer 获取负载均衡器
func (lbm *LoadBalancerManager) GetLoadBalancer(key string) (LoadBalancer, bool) {
	lbm.mu.RLock()
	defer lbm.mu.RUnlock()

	balancer, exists := lbm.balancers[key]
	return balancer, exists
}

// GetNextURL 获取下一个URL
func (lbm *LoadBalancerManager) GetNextURL(key string) (string, bool) {
	balancer, exists := lbm.GetLoadBalancer(key)
	if !exists {
		return "", false
	}

	url := balancer.GetNext()
	return url, url != ""
}
