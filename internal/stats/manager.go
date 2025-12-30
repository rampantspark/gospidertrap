package stats

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Manager provides a unified interface for statistics tracking.
//
// It supports both database and file-based backends and handles
// IP resolution, request recording, and statistics retrieval.
type Manager struct {
	db         *Database
	stats      *Stats
	ipResolver *IPResolver
	logger     *slog.Logger
}

// NewManager creates a new stats manager.
//
// Parameters:
//   - db: optional database instance (nil for file-based mode)
//   - stats: optional in-memory stats (nil for database mode)
//   - trustProxy: whether to trust proxy headers for IP resolution
//   - logger: structured logger instance
//
// Returns a new Manager instance.
func NewManager(db *Database, stats *Stats, trustProxy bool, logger *slog.Logger) *Manager {
	return &Manager{
		db:         db,
		stats:      stats,
		ipResolver: NewIPResolver(trustProxy),
		logger:     logger,
	}
}

// RecordRequest records a request in the statistics.
//
// Uses database if configured, otherwise falls back to in-memory stats.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - r: the HTTP request
//
// Returns an error if database recording fails (file mode never returns error).
func (m *Manager) RecordRequest(ctx context.Context, r *http.Request) error {
	ip := m.ipResolver.GetClientIP(r)
	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		userAgent = "Unknown"
	}
	path := r.URL.Path
	now := time.Now()

	reqInfo := RequestInfo{
		IP:        ip,
		UserAgent: userAgent,
		Path:      path,
		Timestamp: now,
	}

	// Use database if configured
	if m.db != nil {
		if err := m.db.RecordRequest(ctx, reqInfo); err != nil {
			m.logger.Warn("Failed to record request in database", "error", err)
			return err
		}
		return nil
	}

	// Fall back to in-memory stats (file mode)
	// Note: File persistence is handled separately by the caller
	m.stats.Mu.Lock()
	defer m.stats.Mu.Unlock()

	m.stats.TotalRequests++

	// Only track new IPs if we haven't reached the limit
	if _, exists := m.stats.IPCounts[ip]; exists {
		m.stats.IPCounts[ip]++
	} else if len(m.stats.IPCounts) < maxTrackedIPs {
		m.stats.IPCounts[ip] = 1
	}

	// Only track new user agents if we haven't reached the limit
	if _, exists := m.stats.UserAgents[userAgent]; exists {
		m.stats.UserAgents[userAgent]++
	} else if len(m.stats.UserAgents) < maxTrackedUserAgents {
		m.stats.UserAgents[userAgent] = 1
	}

	// Add to recent requests
	m.stats.RecentRequests = append(m.stats.RecentRequests, reqInfo)

	// Keep only the most recent requests
	if len(m.stats.RecentRequests) > m.stats.MaxRecentRequests {
		m.stats.RecentRequests = m.stats.RecentRequests[1:]
	}

	return nil
}

// GetChartData retrieves chart data for the admin UI.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - topItemsCount: number of top items to retrieve
//   - maxUserAgentLength: maximum length of user agent strings
//
// Returns chart data including top IPs and top user agents.
func (m *Manager) GetChartData(ctx context.Context, topItemsCount, maxUserAgentLength int) ChartData {
	var data ChartData

	// Use database if configured
	if m.db != nil {
		topIPs, err := m.db.GetTopIPs(ctx, topItemsCount)
		if err != nil {
			m.logger.Warn("Failed to get top IPs from database", "error", err)
			return data
		}

		topUAs, err := m.db.GetTopUserAgents(ctx, topItemsCount)
		if err != nil {
			m.logger.Warn("Failed to get top user agents from database", "error", err)
			return data
		}

		// Convert to chart data (preserves order from database)
		for _, entry := range topIPs {
			data.TopIPs.Labels = append(data.TopIPs.Labels, entry.Label)
			data.TopIPs.Data = append(data.TopIPs.Data, entry.Count)
		}

		for _, entry := range topUAs {
			// Truncate long user agents for display
			displayUA := entry.Label
			if len(entry.Label) > maxUserAgentLength {
				displayUA = entry.Label[:maxUserAgentLength-3] + "..."
			}
			data.TopUserAgents.Labels = append(data.TopUserAgents.Labels, displayUA)
			data.TopUserAgents.Data = append(data.TopUserAgents.Data, entry.Count)
		}

		return data
	}

	// Fall back to in-memory stats (file mode)
	return m.getChartDataFromMemory(topItemsCount, maxUserAgentLength)
}

// getChartDataFromMemory retrieves chart data from in-memory stats.
func (m *Manager) getChartDataFromMemory(topItemsCount, maxUserAgentLength int) ChartData {
	m.stats.Mu.RLock()
	defer m.stats.Mu.RUnlock()

	var data ChartData

	// Get top IPs
	type countPair struct {
		label string
		count int
	}

	ipList := make([]countPair, 0, len(m.stats.IPCounts))
	for ip, count := range m.stats.IPCounts {
		ipList = append(ipList, countPair{label: ip, count: count})
	}
	// Sort by count descending
	for i := 0; i < len(ipList); i++ {
		for j := i + 1; j < len(ipList); j++ {
			if ipList[j].count > ipList[i].count {
				ipList[i], ipList[j] = ipList[j], ipList[i]
			}
		}
	}

	// Take top N
	maxIPs := topItemsCount
	if len(ipList) < maxIPs {
		maxIPs = len(ipList)
	}
	for i := 0; i < maxIPs; i++ {
		data.TopIPs.Labels = append(data.TopIPs.Labels, ipList[i].label)
		data.TopIPs.Data = append(data.TopIPs.Data, ipList[i].count)
	}

	// Get top user agents
	uaList := make([]countPair, 0, len(m.stats.UserAgents))
	for ua, count := range m.stats.UserAgents {
		uaList = append(uaList, countPair{label: ua, count: count})
	}
	// Sort by count descending
	for i := 0; i < len(uaList); i++ {
		for j := i + 1; j < len(uaList); j++ {
			if uaList[j].count > uaList[i].count {
				uaList[i], uaList[j] = uaList[j], uaList[i]
			}
		}
	}

	// Take top N
	maxUAs := topItemsCount
	if len(uaList) < maxUAs {
		maxUAs = len(uaList)
	}
	for i := 0; i < maxUAs; i++ {
		ua := uaList[i].label
		// Truncate long user agents for display
		if len(ua) > maxUserAgentLength {
			ua = ua[:maxUserAgentLength-3] + "..."
		}
		data.TopUserAgents.Labels = append(data.TopUserAgents.Labels, ua)
		data.TopUserAgents.Data = append(data.TopUserAgents.Data, uaList[i].count)
	}

	return data
}

// GetStats retrieves current statistics.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//
// Returns:
//   - uptime: server uptime duration
//   - totalRequests: total number of requests
//   - uniqueIPs: number of unique IP addresses
//   - uniqueUAs: number of unique user agents
func (m *Manager) GetStats(ctx context.Context) (uptime time.Duration, totalRequests, uniqueIPs, uniqueUAs int) {
	if m.db != nil {
		dbStats, err := m.db.GetStats(ctx)
		if err != nil {
			m.logger.Warn("Failed to get stats from database", "error", err)
			return 0, 0, 0, 0
		}
		return time.Since(dbStats.StartTime), dbStats.TotalRequests, len(dbStats.IPCounts), len(dbStats.UserAgents)
	}

	// Fall back to in-memory stats
	m.stats.Mu.RLock()
	defer m.stats.Mu.RUnlock()

	return time.Since(m.stats.StartTime), m.stats.TotalRequests, len(m.stats.IPCounts), len(m.stats.UserAgents)
}

// GetRecentRequests retrieves recent request log entries.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - limit: maximum number of requests to retrieve
//
// Returns a slice of RequestInfo ordered by timestamp (most recent first).
func (m *Manager) GetRecentRequests(ctx context.Context, limit int) []RequestInfo {
	if m.db != nil {
		// Database mode: already returns DESC order (most recent first)
		requests, err := m.db.GetRecentRequests(ctx, limit)
		if err != nil {
			m.logger.Warn("Failed to get recent requests from database", "error", err)
			return nil
		}
		return requests
	}

	// File mode: in-memory stats stored chronologically (oldest first)
	// Need to reverse to match the "most recent first" contract
	m.stats.Mu.RLock()
	defer m.stats.Mu.RUnlock()

	total := len(m.stats.RecentRequests)
	if total == 0 {
		return nil
	}

	// Limit to requested amount
	count := total
	if limit > 0 && limit < total {
		count = limit
	}

	// Create result in reverse order (most recent first)
	result := make([]RequestInfo, count)
	for i := 0; i < count; i++ {
		result[i] = m.stats.RecentRequests[total-1-i]
	}
	return result
}

// GetClientIP extracts the client IP from a request.
//
// This is a convenience method that delegates to the IPResolver.
//
// Parameters:
//   - r: the HTTP request
//
// Returns the client IP address.
func (m *Manager) GetClientIP(r *http.Request) string {
	return m.ipResolver.GetClientIP(r)
}

// Memory limit constants (only used in file mode)
const (
	maxTrackedIPs        = 10000 // Maximum number of unique IP addresses to track
	maxTrackedUserAgents = 1000  // Maximum number of unique user agents to track
)
