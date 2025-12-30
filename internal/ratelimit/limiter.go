package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	// maxTrackedIPs limits the number of unique IPs tracked to prevent memory exhaustion
	// during DDoS attacks with many unique source IPs
	maxTrackedIPs = 10000
)

// Limiter provides per-IP rate limiting.
type Limiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	cleanup  *time.Ticker
	stopChan chan struct{}
}

// NewLimiter creates a new rate limiter.
//
// Parameters:
//   - requestsPerSecond: maximum requests per second per IP
//   - burst: maximum burst size per IP
//
// Returns a new Limiter instance.
func NewLimiter(requestsPerSecond int, burst int) *Limiter {
	l := &Limiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(requestsPerSecond),
		burst:    burst,
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine to remove old entries
	l.cleanup = time.NewTicker(time.Minute)
	go l.cleanupRoutine()

	return l
}

// Allow checks if a request from the given IP should be allowed.
//
// Parameters:
//   - ip: the client IP address
//
// Returns true if the request is allowed, false if rate limited.
func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	limiter, exists := l.limiters[ip]
	if !exists {
		// Prevent unbounded memory growth during DDoS attacks
		if len(l.limiters) >= maxTrackedIPs {
			l.mu.Unlock()
			// Reject new IPs when at capacity to prevent memory exhaustion
			return false
		}
		limiter = rate.NewLimiter(l.rate, l.burst)
		l.limiters[ip] = limiter
	}
	l.mu.Unlock()

	return limiter.Allow()
}

// cleanupRoutine periodically removes limiters for IPs that haven't been seen recently.
//
// This prevents unbounded memory growth from tracking too many unique IPs.
func (l *Limiter) cleanupRoutine() {
	for {
		select {
		case <-l.cleanup.C:
			l.cleanupOldEntries()
		case <-l.stopChan:
			return
		}
	}
}

// cleanupOldEntries removes rate limiters that haven't been used recently.
//
// We remove limiters that have accumulated their full burst, indicating
// they haven't been used in a while.
func (l *Limiter) cleanupOldEntries() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for ip, limiter := range l.limiters {
		// If the limiter has full burst available, it hasn't been used recently
		if limiter.Tokens() >= float64(l.burst) {
			delete(l.limiters, ip)
		}
	}
}

// Stop stops the cleanup goroutine.
//
// Should be called when shutting down the server.
// Safe to call multiple times.
func (l *Limiter) Stop() {
	l.cleanup.Stop()
	select {
	case <-l.stopChan:
		// Already closed
	default:
		close(l.stopChan)
	}
}

// Stats returns current rate limiter statistics.
//
// Returns the number of tracked IPs.
func (l *Limiter) Stats() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.limiters)
}
