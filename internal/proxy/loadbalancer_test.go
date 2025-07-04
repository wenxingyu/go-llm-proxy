package proxy

import (
	"testing"
)

func TestRoundRobinLoadBalancer(t *testing.T) {
	urls := []string{
		"https://api1.example.com",
		"https://api2.example.com",
		"https://api3.example.com",
	}

	lb := NewRoundRobinLoadBalancer(urls)

	// 测试轮询顺序
	expected := []string{
		"https://api1.example.com",
		"https://api2.example.com",
		"https://api3.example.com",
		"https://api1.example.com", // 回到第一个
	}

	for i, expectedURL := range expected {
		actual := lb.GetNext()
		if actual != expectedURL {
			t.Errorf("Expected %s, got %s at iteration %d", expectedURL, actual, i)
		}
	}
}

func TestLoadBalancerManager(t *testing.T) {
	lbm := NewLoadBalancerManager()

	// 添加负载均衡器
	urls := []string{
		"https://api1.example.com",
		"https://api2.example.com",
	}
	lbm.AddLoadBalancer("test-model", urls)

	// 测试获取URL
	url, exists := lbm.GetNextURL("test-model")
	if !exists {
		t.Error("Expected to find load balancer for test-model")
	}
	if url != "https://api1.example.com" {
		t.Errorf("Expected https://api1.example.com, got %s", url)
	}

	// 测试不存在的模型
	_, exists = lbm.GetNextURL("non-existent-model")
	if exists {
		t.Error("Expected not to find load balancer for non-existent-model")
	}
}

func TestLoadBalancerWithEmptyURLs(t *testing.T) {
	lb := NewRoundRobinLoadBalancer([]string{})

	url := lb.GetNext()
	if url != "" {
		t.Errorf("Expected empty string, got %s", url)
	}
}
