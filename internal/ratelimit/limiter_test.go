package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	limiter := NewLimiter(10, 20)

	if limiter == nil {
		t.Fatal("NewLimiter returned nil")
	}
	if limiter.limiters == nil {
		t.Error("limiters map is nil")
	}
	if limiter.cleanup == nil {
		t.Error("cleanup ticker is nil")
	}
	if limiter.stopChan == nil {
		t.Error("stopChan is nil")
	}

	// Cleanup
	limiter.Stop()
}

func TestAllow_SingleIP(t *testing.T) {
	limiter := NewLimiter(10, 20)
	defer limiter.Stop()

	ip := "192.168.1.1"

	// Should allow burst number of requests immediately
	for i := 0; i < 20; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("Request %d was denied, should be allowed (within burst)", i)
		}
	}

	// Next request should be denied (exceeded burst)
	if limiter.Allow(ip) {
		t.Error("Request after burst should be denied")
	}
}

func TestAllow_MultipleIPs(t *testing.T) {
	limiter := NewLimiter(10, 5)
	defer limiter.Stop()

	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	// Each IP should have its own limit
	for _, ip := range ips {
		for i := 0; i < 5; i++ {
			if !limiter.Allow(ip) {
				t.Errorf("IP %s request %d denied, should be allowed", ip, i)
			}
		}
		// Exceed burst for this IP
		if limiter.Allow(ip) {
			t.Errorf("IP %s exceeded burst, should be denied", ip)
		}
	}
}

func TestAllow_RateRefill(t *testing.T) {
	// High rate so tokens refill quickly
	limiter := NewLimiter(100, 1)
	defer limiter.Stop()

	ip := "192.168.1.1"

	// Use up the burst
	if !limiter.Allow(ip) {
		t.Fatal("First request denied")
	}

	// Should be denied immediately
	if limiter.Allow(ip) {
		t.Error("Second request should be denied (burst used)")
	}

	// Wait for token to refill (at 100 req/sec, should refill in 10ms)
	time.Sleep(20 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow(ip) {
		t.Error("Request after refill should be allowed")
	}
}

func TestConcurrentAccess(t *testing.T) {
	limiter := NewLimiter(100, 50)
	defer limiter.Stop()

	ip := "192.168.1.1"
	const goroutines = 100
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	allowed := make([]int, goroutines)
	denied := make([]int, goroutines)

	for i := 0; i < goroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				if limiter.Allow(ip) {
					allowed[idx]++
				} else {
					denied[idx]++
				}
			}
		}()
	}

	wg.Wait()

	totalAllowed := 0
	totalDenied := 0
	for i := 0; i < goroutines; i++ {
		totalAllowed += allowed[i]
		totalDenied += denied[i]
	}

	t.Logf("Total allowed: %d, denied: %d", totalAllowed, totalDenied)

	// Should have denied at least some requests (we sent 1000 requests, burst is 50)
	if totalDenied == 0 {
		t.Error("Expected some requests to be denied")
	}

	// Total should equal all requests
	totalRequests := goroutines * requestsPerGoroutine
	if totalAllowed+totalDenied != totalRequests {
		t.Errorf("Total allowed + denied = %d, want %d", totalAllowed+totalDenied, totalRequests)
	}
}

func TestCleanupOldEntries(t *testing.T) {
	limiter := NewLimiter(10, 10)
	defer limiter.Stop()

	// Create limiters for multiple IPs
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}
	for _, ip := range ips {
		limiter.Allow(ip)
	}

	// Check all IPs are tracked
	if limiter.Stats() != 3 {
		t.Errorf("Stats() = %d, want 3", limiter.Stats())
	}

	// Wait for tokens to refill completely (at 10 req/sec, 10 tokens needs 1 second)
	time.Sleep(1200 * time.Millisecond)

	// Trigger cleanup manually
	limiter.cleanupOldEntries()

	// IPs with full tokens should be removed
	stats := limiter.Stats()
	if stats != 0 {
		t.Errorf("Stats() after cleanup = %d, want 0 (all should be cleaned up)", stats)
	}
}

func TestStats(t *testing.T) {
	limiter := NewLimiter(10, 5)
	defer limiter.Stop()

	// Initially empty
	if limiter.Stats() != 0 {
		t.Errorf("Stats() = %d, want 0", limiter.Stats())
	}

	// Add some IPs
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3", "192.168.1.4"}
	for _, ip := range ips {
		limiter.Allow(ip)
	}

	if limiter.Stats() != len(ips) {
		t.Errorf("Stats() = %d, want %d", limiter.Stats(), len(ips))
	}
}

func TestStop(t *testing.T) {
	limiter := NewLimiter(10, 5)

	// Should not panic
	limiter.Stop()

	// Stopping again should not panic
	limiter.Stop()
}

func TestCleanupRoutine(t *testing.T) {
	// Create limiter with very short cleanup interval for testing
	limiter := NewLimiter(100, 10)

	// Create some limiters
	for i := 0; i < 5; i++ {
		limiter.Allow("192.168.1.1")
	}

	if limiter.Stats() == 0 {
		t.Error("Expected at least one limiter to be tracked")
	}

	// Stop should halt the cleanup goroutine without panic
	limiter.Stop()

	// Give it a moment to process
	time.Sleep(10 * time.Millisecond)

	// Should be able to stop again
	limiter.Stop()
}

func BenchmarkAllow_SingleIP(b *testing.B) {
	limiter := NewLimiter(1000, 100)
	defer limiter.Stop()
	ip := "192.168.1.1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ip)
	}
}

func BenchmarkAllow_MultipleIPs(b *testing.B) {
	limiter := NewLimiter(1000, 100)
	defer limiter.Stop()

	ips := []string{
		"192.168.1.1",
		"192.168.1.2",
		"192.168.1.3",
		"192.168.1.4",
		"192.168.1.5",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := ips[i%len(ips)]
		limiter.Allow(ip)
	}
}

func BenchmarkConcurrentAllow(b *testing.B) {
	limiter := NewLimiter(10000, 1000)
	defer limiter.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ip := "192.168.1.1"
		for pb.Next() {
			limiter.Allow(ip)
		}
	})
}
