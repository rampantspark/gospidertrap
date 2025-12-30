// Package stats provides statistics tracking for HTTP requests.
package stats

import (
	"sync"
	"time"
)

// RequestInfo holds information about a single request.
type RequestInfo struct {
	IP        string    // Client IP address
	UserAgent string    // Client User-Agent header
	Path      string    // Requested path
	Timestamp time.Time // Request timestamp
}

// Stats holds connection statistics and request information.
type Stats struct {
	Mu                sync.RWMutex              // Mutex for thread-safe access
	StartTime         time.Time                 // Server start time
	TotalRequests     int                       // Total number of requests
	IPCounts          map[string]int            // Request count per IP address
	UserAgents        map[string]int            // Request count per user agent
	RecentRequests    []RequestInfo             // Recent request history (limited size)
	MaxRecentRequests int                       // Maximum number of recent requests to keep
}

// NewStats creates and initializes a new Stats instance.
func NewStats() *Stats {
	return &Stats{
		StartTime:         time.Now(),
		IPCounts:          make(map[string]int),
		UserAgents:        make(map[string]int),
		RecentRequests:    make([]RequestInfo, 0),
		MaxRecentRequests: 100, // Keep last 100 requests
	}
}

// PersistedStats holds the serializable form of Stats for JSON persistence.
type PersistedStats struct {
	StartTime      time.Time      `json:"startTime"`
	TotalRequests  int            `json:"totalRequests"`
	IPCounts       map[string]int `json:"ipCounts"`
	UserAgents     map[string]int `json:"userAgents"`
	RecentRequests []RequestInfo  `json:"recentRequests"`
}

// ChartData holds data for rendering charts in the admin UI.
type ChartData struct {
	TopIPs struct {
		Labels []string `json:"labels"`
		Data   []int    `json:"data"`
	} `json:"topIPs"`
	TopUserAgents struct {
		Labels []string `json:"labels"`
		Data   []int    `json:"data"`
	} `json:"topUserAgents"`
}
