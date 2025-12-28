// Package main implements gospidertrap, a web server that generates HTML pages
// with random links to trap web crawlers and spiders.
//
// The server can generate pages with procedurally generated links or use
// links from a wordlist file. It also supports HTML template files where
// existing links are replaced with random ones.
//
// Usage:
//
//	gospidertrap -p 8000 -w wordlist.txt
//	gospidertrap -p 8080 -a template.html -w wordlist.txt -e /submit
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Configuration constants for page generation and server behavior.
const (
	// Minimum and maximum number of links to generate per page.
	linksPerPageMin = 5
	linksPerPageMax = 10

	// Minimum and maximum length of randomly generated link strings.
	lengthOfLinksMin = 3
	lengthOfLinksMax = 20

	// Default port for the HTTP server.
	defaultPort = "8000"

	// Delay in milliseconds to add to each request to simulate real-world response times.
	delayMilliseconds = 350

	// Character set used for generating random link strings.
	charSpace = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890_-/"

	// HTTP server timeout values in seconds.
	readTimeoutSeconds     = 15
	writeTimeoutSeconds    = 15
	idleTimeoutSeconds     = 60
	shutdownTimeoutSeconds = 5

	// Length of random admin endpoint path
	adminPathLength = 32
	// Length of admin authentication token
	adminTokenLength = 32

	// Input validation limits
	maxHTMLTemplateSize = 10 * 1024 * 1024 // 10 MB
	maxWordlistFileSize = 50 * 1024 * 1024 // 50 MB
	maxWordlistEntries  = 100000           // Maximum number of wordlist entries

	// Persistence settings
	defaultDataDir        = "data"
	statsSaveInterval     = 5 * time.Minute // Save stats every 5 minutes
	requestsLogFileName   = "requests.ndjson"
	statsFileName         = "stats.json"

	// Admin UI display settings
	maxRecentRequestsDisplay    = 50  // Maximum number of recent requests to display in admin UI
	maxUserAgentDisplayLength   = 50  // Maximum length of user agent to display
	maxUserAgentTruncateLength  = 47  // Length to truncate user agent to (with "...")
	topItemsDisplayCount        = 10  // Number of top items to display in charts and tables

	// Cookie settings for admin authentication
	adminCookieName   = "gospidertrap_admin_token"
	adminCookieMaxAge = 86400 // 24 hours in seconds
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
	mu                sync.RWMutex              // Mutex for thread-safe access
	StartTime         time.Time                 // Server start time
	TotalRequests     int                       // Total number of requests
	IPCounts          map[string]int            // Request count per IP address
	UserAgents        map[string]int            // Request count per user agent
	RecentRequests    []RequestInfo             // Recent request history (limited size)
	MaxRecentRequests int                       // Maximum number of recent requests to keep
}

// newStats creates and initializes a new Stats instance.
func newStats() *Stats {
	return &Stats{
		StartTime:         time.Now(),
		IPCounts:          make(map[string]int),
		UserAgents:        make(map[string]int),
		RecentRequests:    make([]RequestInfo, 0),
		MaxRecentRequests: 100, // Keep last 100 requests
	}
}

// Config holds the application configuration and state.
// It manages wordlists, HTML templates, server settings, and random number generation.
type Config struct {
	webpages     []string    // Wordlist entries to use for link generation
	chars        []rune      // Character set for random string generation
	port         string      // Port number for the HTTP server
	endpoint     string      // Form submission endpoint (optional)
	htmlTemplate string      // HTML template file content (optional)
	rand         *rand.Rand  // Random number generator instance
	stats        *Stats      // Statistics tracking instance
	adminPath    string      // Random admin UI endpoint path
	adminToken   string      // Authentication token for admin UI
	dataDir      string      // Directory for persisting data files
	logFile      *os.File    // File handle for NDJSON request log
	logFileMu    sync.Mutex  // Mutex for thread-safe log file writes
	logger       *slog.Logger // Structured logger instance
	saveCtx      context.Context    // Context for periodic stats saving
	saveCancel   context.CancelFunc // Cancel function for periodic stats saving
}

// newConfig creates and initializes a new Config instance with default values.
// It initializes the random number generator with a time-based seed and sets up structured logging.
func newConfig() *Config {
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	return &Config{
		chars:       []rune(charSpace),
		rand:        rand.New(rand.NewSource(time.Now().UnixNano())),
		stats:       newStats(),
		dataDir:    defaultDataDir,
		logger:     logger,
		saveCtx:    ctx,
		saveCancel: cancel,
	}
}

// generatePage generates an HTML page with random links.
//
// If an HTML template is configured, it replaces all href attributes in <a> tags
// with random links while preserving the rest of the template structure.
// Otherwise, it generates a new HTML page from scratch with random links
// and optionally a form if an endpoint is configured.
//
// Returns the generated HTML as a string.
func (cfg *Config) generatePage() string {
	if cfg.htmlTemplate != "" {
		return cfg.replaceLinksInHTML(cfg.htmlTemplate)
	}
	return cfg.generateNewPage()
}

// generateNewPage creates a new HTML page from scratch with random links.
//
// The number of links is randomly determined between linksPerPageMin and
// linksPerPageMax. Links are either selected from the wordlist (if available)
// or generated as random strings. If an endpoint is configured, a form is
// also included in the generated page.
//
// Returns the complete HTML page as a string.
func (cfg *Config) generateNewPage() string {
	var sb strings.Builder
	sb.WriteString("<html>\n<body>\n")

	// Generate random links
	numLinks := cfg.randomInt(linksPerPageMin, linksPerPageMax)
	for i := 0; i < numLinks; i++ {
		link := cfg.generateRandomLink()
		cfg.writeLink(&sb, link)
	}

	// Add form if endpoint is configured
	if cfg.endpoint != "" {
		cfg.writeForm(&sb)
	}

	sb.WriteString("</body>\n</html>")
	return sb.String()
}

// generateRandomLink generates a random link address.
//
// If a wordlist is available, it selects a random entry from the wordlist.
// Otherwise, it generates a random string using the configured character set
// with a length between lengthOfLinksMin and lengthOfLinksMax.
//
// Returns the generated link address as a string.
func (cfg *Config) generateRandomLink() string {
	if len(cfg.webpages) > 0 {
		idx := cfg.rand.Intn(len(cfg.webpages))
		return cfg.webpages[idx]
	}
	length := cfg.randomInt(lengthOfLinksMin, lengthOfLinksMax)
	return cfg.randString(length)
}

// randomInt returns a random integer in the inclusive range [min, max].
//
// Parameters:
//   - min: the minimum value (inclusive)
//   - max: the maximum value (inclusive)
//
// Returns a random integer between min and max, inclusive.
func (cfg *Config) randomInt(min, max int) int {
	return cfg.rand.Intn(max-min+1) + min
}

// writeLink writes an HTML anchor tag to the string builder.
//
// The link address is HTML-escaped to prevent injection attacks.
// The generated tag format is: <a href="...">...</a><br>
//
// Parameters:
//   - sb: the string builder to write to
//   - address: the link address (will be HTML-escaped)
func (cfg *Config) writeLink(sb *strings.Builder, address string) {
	escapedAddress := html.EscapeString(address)
	sb.WriteString("<a href=\"")
	sb.WriteString(escapedAddress)
	sb.WriteString("\">")
	sb.WriteString(escapedAddress)
	sb.WriteString("</a><br>\n")
}

// writeForm writes an HTML form element to the string builder.
//
// The form uses the configured endpoint as its action URL and submits via GET method.
// The endpoint is HTML-escaped to prevent injection attacks.
// The form includes a text input field and a submit button.
//
// Parameters:
//   - sb: the string builder to write to
func (cfg *Config) writeForm(sb *strings.Builder) {
	escapedEndpoint := html.EscapeString(cfg.endpoint)
	sb.WriteString(`<form action="`)
	sb.WriteString(escapedEndpoint)
	sb.WriteString(`" method="get">
		<input type="text" name="param">
		<button type="submit">Submit</button>
		</form>`)
}

// replaceLinksInHTML replaces all href attributes in <a> tags with random links.
//
// It uses regular expressions to find all anchor tags with href attributes
// (supporting both single and double quotes) and replaces the href value
// with a randomly generated link while preserving all other attributes
// and the tag structure. The replacement links are HTML-escaped.
//
// Parameters:
//   - template: the HTML template string to process
//
// Returns the modified HTML string with replaced links.
func (cfg *Config) replaceLinksInHTML(template string) string {
	// Regex to match <a href="..."> or <a href='...'>
	hrefRegex := regexp.MustCompile(`<a\s+[^>]*href=["']([^"']*)["'][^>]*>`)
	hrefReplacer := regexp.MustCompile(`href=["'][^"']*["']`)

	return hrefRegex.ReplaceAllStringFunc(template, func(match string) string {
		randomLink := cfg.generateRandomLink()
		escapedLink := html.EscapeString(randomLink)
		return hrefReplacer.ReplaceAllString(match, `href="`+escapedLink+`"`)
	})
}

// randString generates a random string of the specified length.
//
// The string is composed of characters randomly selected from the configured
// character set (charSpace). Each character has an equal probability of
// being selected.
//
// Parameters:
//   - length: the desired length of the generated string
//
// Returns a random string of the specified length.
func (cfg *Config) randString(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = cfg.chars[cfg.rand.Intn(len(cfg.chars))]
	}
	return string(b)
}

// generateAdminPath generates a random admin endpoint path.
//
// Returns a random path string starting with "/" for the admin UI.
func (cfg *Config) generateAdminPath() string {
	return "/" + cfg.randString(adminPathLength)
}

// generateAdminToken generates a random authentication token for the admin UI.
//
// Returns a random token string.
func (cfg *Config) generateAdminToken() string {
	return cfg.randString(adminTokenLength)
}

// validateAdminToken checks if the provided token matches the admin token.
//
// Parameters:
//   - token: the token to validate
//
// Returns true if the token is valid, false otherwise.
func (cfg *Config) validateAdminToken(token string) bool {
	return token == cfg.adminToken
}

// getAdminTokenFromRequest extracts the admin token from either cookie or query parameter.
//
// For backward compatibility, it checks query parameters first, then cookies.
//
// Parameters:
//   - r: the HTTP request
//
// Returns the token if found, empty string otherwise.
func (cfg *Config) getAdminTokenFromRequest(r *http.Request) string {
	// Check cookie first (preferred method)
	if cookie, err := r.Cookie(adminCookieName); err == nil {
		return cookie.Value
	}
	// Fallback to query parameter for backward compatibility
	return r.URL.Query().Get("token")
}

// setAdminCookie sets the admin authentication cookie.
//
// Parameters:
//   - w: the HTTP response writer
func (cfg *Config) setAdminCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    cfg.adminToken,
		Path:     "/",
		MaxAge:   adminCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   false, // Set to true if using HTTPS
	})
}

// isAuthenticated checks if the request is authenticated with a valid admin token.
//
// Parameters:
//   - r: the HTTP request
//
// Returns true if authenticated, false otherwise.
func (cfg *Config) isAuthenticated(r *http.Request) bool {
	token := cfg.getAdminTokenFromRequest(r)
	return cfg.validateAdminToken(token)
}

// ChartData holds data for rendering charts.
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

// getChartData retrieves chart data from statistics.
//
// Returns chart data including top IPs and top user agents.
func (cfg *Config) getChartData() ChartData {
	cfg.stats.mu.RLock()
	defer cfg.stats.mu.RUnlock()

	var data ChartData

	// Top IPs (top 10)
	type ipCount struct {
		ip    string
		count int
	}
	ipList := make([]ipCount, 0, len(cfg.stats.IPCounts))
	for ip, count := range cfg.stats.IPCounts {
		ipList = append(ipList, ipCount{ip: ip, count: count})
	}
	sort.Slice(ipList, func(i, j int) bool {
		return ipList[i].count > ipList[j].count
	})
	maxIPs := topItemsDisplayCount
	if len(ipList) < maxIPs {
		maxIPs = len(ipList)
	}
	for i := 0; i < maxIPs; i++ {
		data.TopIPs.Labels = append(data.TopIPs.Labels, ipList[i].ip)
		data.TopIPs.Data = append(data.TopIPs.Data, ipList[i].count)
	}

	// Top User Agents
	type uaCount struct {
		ua    string
		count int
	}
	uaList := make([]uaCount, 0, len(cfg.stats.UserAgents))
	for ua, count := range cfg.stats.UserAgents {
		uaList = append(uaList, uaCount{ua: ua, count: count})
	}
	sort.Slice(uaList, func(i, j int) bool {
		return uaList[i].count > uaList[j].count
	})
	maxUAs := topItemsDisplayCount
	if len(uaList) < maxUAs {
		maxUAs = len(uaList)
	}
	for i := 0; i < maxUAs; i++ {
		// Truncate long user agents for display
		ua := uaList[i].ua
		if len(ua) > maxUserAgentDisplayLength {
			ua = ua[:maxUserAgentTruncateLength] + "..."
		}
		data.TopUserAgents.Labels = append(data.TopUserAgents.Labels, ua)
		data.TopUserAgents.Data = append(data.TopUserAgents.Data, uaList[i].count)
	}

	return data
}

// handleChartData handles requests for chart data in JSON format.
//
// It validates the authentication token and returns chart data as JSON.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request
func (cfg *Config) handleChartData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Validate authentication
	if !cfg.isAuthenticated(r) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid or missing authentication token"})
		return
	}

	// Check if context is cancelled
	if ctx.Err() != nil {
		return
	}

	data := cfg.getChartData()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleAdminLogin handles login requests for the admin UI.
//
// It accepts a token via query parameter, validates it, and sets an HTTP cookie
// if valid. For backward compatibility, it also accepts the token in the URL.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request
func (cfg *Config) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if context is cancelled
	if ctx.Err() != nil {
		return
	}

	token := r.URL.Query().Get("token")
	if !cfg.validateAdminToken(token) {
		w.WriteHeader(http.StatusForbidden)
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!DOCTYPE html>\n<html>\n<head><title>Access Denied</title></head>\n<body>\n<h1>403 Forbidden</h1>\n<p>Invalid or missing authentication token.</p>\n</body>\n</html>")
		return
	}

	// Set authentication cookie
	cfg.setAdminCookie(w)

	// Redirect to admin UI (without token in URL)
	http.Redirect(w, r, cfg.adminPath, http.StatusSeeOther)
}

// writeAdminHTMLHeader writes the HTML header, styles, and opening body tag for the admin UI.
//
// Parameters:
//   - sb: the string builder to write to
func (cfg *Config) writeAdminHTMLHeader(sb *strings.Builder) {
	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	sb.WriteString("<title>gospidertrap - dashboard</title>\n")
	sb.WriteString("<style>\n")
	sb.WriteString("body { font-family: monospace; margin: 20px; background: #f5f5f5; }\n")
	sb.WriteString("h1 { color: #333; }\n")
	sb.WriteString(".stat-box { background: white; padding: 15px; margin: 10px 0; border-radius: 5px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }\n")
	sb.WriteString("table { width: 100%; border-collapse: collapse; margin-top: 10px; }\n")
	sb.WriteString("th, td { padding: 8px; text-align: left; border-bottom: 1px solid #ddd; }\n")
	sb.WriteString("th { background-color: #4CAF50; color: white; }\n")
	sb.WriteString("tr:hover { background-color: #f5f5f5; }\n")
	sb.WriteString(".ip { font-family: monospace; }\n")
	sb.WriteString(".chart-container { position: relative; height: 300px; margin: 20px 0; }\n")
	sb.WriteString(".charts-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin: 20px 0; }\n")
	sb.WriteString("@media (max-width: 768px) { .charts-grid { grid-template-columns: 1fr; } }\n")
	sb.WriteString(".chart-table-row { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin: 20px 0; }\n")
	sb.WriteString("@media (max-width: 768px) { .chart-table-row { grid-template-columns: 1fr; } }\n")
	sb.WriteString("</style>\n")
	sb.WriteString("<script src=\"https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js\"></script>\n")
	sb.WriteString("</head>\n<body>\n")
	sb.WriteString("<h1>gospidertrap</h1>\n")
}

// writeAdminStatsBox writes the overall server statistics box.
//
// Parameters:
//   - sb: the string builder to write to
//   - uptime: the server uptime duration
//   - totalRequests: total number of requests
//   - uniqueIPs: number of unique IP addresses
//   - uniqueUAs: number of unique user agents
func (cfg *Config) writeAdminStatsBox(sb *strings.Builder, uptime time.Duration, totalRequests, uniqueIPs, uniqueUAs int) {
	sb.WriteString("<div class=\"stat-box\">\n")
	sb.WriteString("<h2>Server Statistics</h2>\n")
	sb.WriteString("<p><strong>Uptime:</strong> " + html.EscapeString(uptime.String()) + "</p>\n")
	sb.WriteString("<p><strong>Total Requests:</strong> " + strconv.Itoa(totalRequests) + "</p>\n")
	sb.WriteString("<p><strong>Unique IPs:</strong> " + strconv.Itoa(uniqueIPs) + "</p>\n")
	sb.WriteString("<p><strong>Unique User Agents:</strong> " + strconv.Itoa(uniqueUAs) + "</p>\n")
	sb.WriteString("</div>\n")
}

// writeAdminTopIPsSection writes the top IP addresses chart and table section.
//
// Parameters:
//   - sb: the string builder to write to
//   - chartData: the chart data containing top IPs
func (cfg *Config) writeAdminTopIPsSection(sb *strings.Builder, chartData ChartData) {
	sb.WriteString("<div class=\"chart-table-row\">\n")
	// Top IP Addresses Chart
	sb.WriteString("<div class=\"stat-box\">\n")
	sb.WriteString("<h2>Top IP Addresses</h2>\n")
	sb.WriteString("<div class=\"chart-container\"><canvas id=\"ipChart\"></canvas></div>\n")
	sb.WriteString("</div>\n")

	// Top IP Addresses Table
	if len(chartData.TopIPs.Labels) > 0 {
		sb.WriteString("<div class=\"stat-box\">\n")
		sb.WriteString("<h2>Top IP Addresses</h2>\n")
		sb.WriteString("<table>\n")
		sb.WriteString("<tr><th>IP Address</th><th>Request Count</th></tr>\n")

		for i := 0; i < len(chartData.TopIPs.Labels); i++ {
			sb.WriteString("<tr><td class=\"ip\">")
			sb.WriteString(html.EscapeString(chartData.TopIPs.Labels[i]))
			sb.WriteString("</td><td>")
			sb.WriteString(strconv.Itoa(chartData.TopIPs.Data[i]))
			sb.WriteString("</td></tr>\n")
		}
		sb.WriteString("</table>\n")
		sb.WriteString("</div>\n")
	}
	sb.WriteString("</div>\n")
}

// writeAdminTopUAsSection writes the top user agents chart and table section.
//
// Parameters:
//   - sb: the string builder to write to
//   - chartData: the chart data containing top user agents
func (cfg *Config) writeAdminTopUAsSection(sb *strings.Builder, chartData ChartData) {
	sb.WriteString("<div class=\"chart-table-row\">\n")
	// Top User Agents Chart
	sb.WriteString("<div class=\"stat-box\">\n")
	sb.WriteString("<h2>Top User Agents</h2>\n")
	sb.WriteString("<div class=\"chart-container\"><canvas id=\"uaChart\"></canvas></div>\n")
	sb.WriteString("</div>\n")

	// Top User Agents Table
	if len(chartData.TopUserAgents.Labels) > 0 {
		sb.WriteString("<div class=\"stat-box\">\n")
		sb.WriteString("<h2>Top User Agents</h2>\n")
		sb.WriteString("<table>\n")
		sb.WriteString("<tr><th>User Agent</th><th>Request Count</th></tr>\n")

		for i := 0; i < len(chartData.TopUserAgents.Labels); i++ {
			sb.WriteString("<tr><td>")
			sb.WriteString(html.EscapeString(chartData.TopUserAgents.Labels[i]))
			sb.WriteString("</td><td>")
			sb.WriteString(strconv.Itoa(chartData.TopUserAgents.Data[i]))
			sb.WriteString("</td></tr>\n")
		}
		sb.WriteString("</table>\n")
		sb.WriteString("</div>\n")
	}
	sb.WriteString("</div>\n")
}

// writeAdminRecentRequestsSection writes the recent requests table section.
//
// Parameters:
//   - sb: the string builder to write to
//   - recentRequests: the slice of recent requests
func (cfg *Config) writeAdminRecentRequestsSection(sb *strings.Builder, recentRequests []RequestInfo) {
	sb.WriteString("<div class=\"stat-box\">\n")
	sb.WriteString("<h2>Recent Requests</h2>\n")
	if len(recentRequests) > 0 {
		sb.WriteString("<table>\n")
		sb.WriteString("<tr><th>Timestamp</th><th>IP Address</th><th>Path</th><th>User Agent</th></tr>\n")

		// Show requests in reverse order (most recent first)
		startIdx := len(recentRequests) - 1
		if startIdx >= maxRecentRequestsDisplay {
			startIdx = maxRecentRequestsDisplay - 1
		}
		for i := startIdx; i >= 0; i-- {
			req := recentRequests[i]
			sb.WriteString("<tr><td>")
			sb.WriteString(html.EscapeString(req.Timestamp.Format("2006-01-02 15:04:05")))
			sb.WriteString("</td><td class=\"ip\">")
			sb.WriteString(html.EscapeString(req.IP))
			sb.WriteString("</td><td>")
			sb.WriteString(html.EscapeString(req.Path))
			sb.WriteString("</td><td>")
			sb.WriteString(html.EscapeString(req.UserAgent))
			sb.WriteString("</td></tr>\n")
		}
		sb.WriteString("</table>\n")
	} else {
		sb.WriteString("<p>No recent requests yet.</p>\n")
	}
	sb.WriteString("</div>\n")
}

// writeAdminChartScript writes the JavaScript code for loading and rendering charts.
//
// Parameters:
//   - sb: the string builder to write to
func (cfg *Config) writeAdminChartScript(sb *strings.Builder) {
	sb.WriteString("<script>\n")
	sb.WriteString("const baseDataUrl = '")
	sb.WriteString(html.EscapeString(cfg.adminPath))
	sb.WriteString("/data';\n")
	sb.WriteString("let ipChart = null;\n")
	sb.WriteString("let uaChart = null;\n")
	sb.WriteString("\n")
	sb.WriteString("async function loadCharts() {\n")
	sb.WriteString("  try {\n")
	sb.WriteString("    const response = await fetch(baseDataUrl);\n")
	sb.WriteString("    if (!response.ok) throw new Error('Failed to load chart data');\n")
	sb.WriteString("    const data = await response.json();\n")
	sb.WriteString("\n")
	sb.WriteString("    // Top IPs Donut Chart\n")
	sb.WriteString("    if (!ipChart) {\n")
	sb.WriteString("      ipChart = new Chart(document.getElementById('ipChart'), {\n")
	sb.WriteString("      type: 'doughnut',\n")
	sb.WriteString("      data: {\n")
	sb.WriteString("        labels: data.topIPs.labels,\n")
	sb.WriteString("        datasets: [{\n")
	sb.WriteString("          data: data.topIPs.data,\n")
	sb.WriteString("          backgroundColor: [\n")
	sb.WriteString("            'rgba(255, 99, 132, 0.8)',\n")
	sb.WriteString("            'rgba(54, 162, 235, 0.8)',\n")
	sb.WriteString("            'rgba(255, 206, 86, 0.8)',\n")
	sb.WriteString("            'rgba(75, 192, 192, 0.8)',\n")
	sb.WriteString("            'rgba(153, 102, 255, 0.8)',\n")
	sb.WriteString("            'rgba(255, 159, 64, 0.8)',\n")
	sb.WriteString("            'rgba(199, 199, 199, 0.8)',\n")
	sb.WriteString("            'rgba(83, 102, 255, 0.8)',\n")
	sb.WriteString("          ]\n")
	sb.WriteString("        }]\n")
	sb.WriteString("      },\n")
	sb.WriteString("      options: {\n")
	sb.WriteString("        responsive: true,\n")
	sb.WriteString("        maintainAspectRatio: false,\n")
	sb.WriteString("        plugins: {\n")
	sb.WriteString("          legend: { position: 'bottom' }\n")
	sb.WriteString("        }\n")
	sb.WriteString("      }\n")
	sb.WriteString("    });\n")
	sb.WriteString("    }\n")
	sb.WriteString("\n")
	sb.WriteString("    // Top User Agents Donut Chart\n")
	sb.WriteString("    if (!uaChart) {\n")
	sb.WriteString("      uaChart = new Chart(document.getElementById('uaChart'), {\n")
	sb.WriteString("      type: 'doughnut',\n")
	sb.WriteString("      data: {\n")
	sb.WriteString("        labels: data.topUserAgents.labels,\n")
	sb.WriteString("        datasets: [{\n")
	sb.WriteString("          data: data.topUserAgents.data,\n")
	sb.WriteString("          backgroundColor: [\n")
	sb.WriteString("            'rgba(255, 99, 132, 0.8)',\n")
	sb.WriteString("            'rgba(54, 162, 235, 0.8)',\n")
	sb.WriteString("            'rgba(255, 206, 86, 0.8)',\n")
	sb.WriteString("            'rgba(75, 192, 192, 0.8)',\n")
	sb.WriteString("            'rgba(153, 102, 255, 0.8)',\n")
	sb.WriteString("            'rgba(255, 159, 64, 0.8)',\n")
	sb.WriteString("            'rgba(199, 199, 199, 0.8)',\n")
	sb.WriteString("            'rgba(83, 102, 255, 0.8)'\n")
	sb.WriteString("          ]\n")
	sb.WriteString("        }]\n")
	sb.WriteString("      },\n")
	sb.WriteString("      options: {\n")
	sb.WriteString("        responsive: true,\n")
	sb.WriteString("        maintainAspectRatio: false,\n")
	sb.WriteString("        plugins: {\n")
	sb.WriteString("          legend: { position: 'bottom' }\n")
	sb.WriteString("        }\n")
	sb.WriteString("      }\n")
	sb.WriteString("    });\n")
	sb.WriteString("    }\n")
	sb.WriteString("  } catch (error) {\n")
	sb.WriteString("    console.error('Error loading charts:', error);\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("loadCharts();\n")
	sb.WriteString("</script>\n")
}

// handleAdminUI handles requests to the admin UI endpoint.
//
// It validates authentication via cookie or query parameter (for backward compatibility).
// If authentication fails, it returns a 403 Forbidden response.
// Otherwise, it displays connection statistics including total requests, IP counts,
// user agent counts, and recent request history in a formatted HTML page.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request
func (cfg *Config) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if context is cancelled
	if ctx.Err() != nil {
		return
	}

	// Validate authentication (supports both cookie and query param for backward compatibility)
	if !cfg.isAuthenticated(r) {
		// If token is in query param but not in cookie, set cookie for future requests
		if token := r.URL.Query().Get("token"); token != "" && cfg.validateAdminToken(token) {
			cfg.setAdminCookie(w)
		} else {
			w.WriteHeader(http.StatusForbidden)
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, "<!DOCTYPE html>\n<html>\n<head><title>Access Denied</title></head>\n<body>\n<h1>403 Forbidden</h1>\n<p>Invalid or missing authentication token.</p>\n<p>Use: <a href=\""+html.EscapeString(cfg.adminPath)+"/login?token=YOUR_TOKEN\">Login</a></p>\n</body>\n</html>")
			return
		}
	}

	// Check context again before processing
	if ctx.Err() != nil {
		return
	}

	// Get chart data (reuses existing efficient sorting logic)
	chartData := cfg.getChartData()

	cfg.stats.mu.RLock()
	uptime := time.Since(cfg.stats.StartTime)
	totalRequests := cfg.stats.TotalRequests
	uniqueIPs := len(cfg.stats.IPCounts)
	uniqueUAs := len(cfg.stats.UserAgents)
	recentRequests := make([]RequestInfo, len(cfg.stats.RecentRequests))
	copy(recentRequests, cfg.stats.RecentRequests)
	cfg.stats.mu.RUnlock()

	// Build HTML using helper functions
	var sb strings.Builder
	cfg.writeAdminHTMLHeader(&sb)
	cfg.writeAdminStatsBox(&sb, uptime, totalRequests, uniqueIPs, uniqueUAs)
	cfg.writeAdminTopIPsSection(&sb, chartData)
	cfg.writeAdminTopUAsSection(&sb, chartData)
	cfg.writeAdminRecentRequestsSection(&sb, recentRequests)
	cfg.writeAdminChartScript(&sb)
	sb.WriteString("</body>\n</html>")

	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, sb.String())
}

// PersistedStats holds the serializable form of Stats for JSON persistence.
type PersistedStats struct {
	StartTime      time.Time      `json:"startTime"`
	TotalRequests  int            `json:"totalRequests"`
	IPCounts       map[string]int `json:"ipCounts"`
	UserAgents     map[string]int `json:"userAgents"`
	RecentRequests []RequestInfo  `json:"recentRequests"`
}

// ensureDataDir creates the data directory if it doesn't exist.
//
// Returns an error if the directory cannot be created.
func (cfg *Config) ensureDataDir() error {
	if cfg.dataDir == "" {
		return nil // Persistence disabled
	}
	return os.MkdirAll(cfg.dataDir, 0755)
}

// openLogFile opens or creates the NDJSON log file for appending requests.
//
// Returns an error if the file cannot be opened.
func (cfg *Config) openLogFile() error {
	if cfg.dataDir == "" {
		return nil // Persistence disabled
	}

	logPath := filepath.Join(cfg.dataDir, requestsLogFileName)
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	cfg.logFile = file
	return nil
}

// closeLogFile closes the NDJSON log file.
func (cfg *Config) closeLogFile() {
	if cfg.logFile != nil {
		cfg.logFile.Close()
		cfg.logFile = nil
	}
}

// appendRequestToLog appends a request to the NDJSON log file.
//
// Parameters:
//   - reqInfo: the request information to log
func (cfg *Config) appendRequestToLog(reqInfo RequestInfo) {
	if cfg.dataDir == "" || cfg.logFile == nil {
		return // Persistence disabled
	}

	cfg.logFileMu.Lock()
	defer cfg.logFileMu.Unlock()

	encoder := json.NewEncoder(cfg.logFile)
	if err := encoder.Encode(reqInfo); err != nil {
		// Log error but don't fail the request
		cfg.logger.Warn("Failed to write to log file", "error", err)
	} else {
		// Flush to ensure data is written to disk
		cfg.logFile.Sync()
	}
}

// saveStats saves the aggregated statistics to a JSON file.
//
// Returns an error if the file cannot be written.
func (cfg *Config) saveStats() error {
	if cfg.dataDir == "" {
		return nil // Persistence disabled
	}

	cfg.stats.mu.RLock()
	persisted := PersistedStats{
		StartTime:      cfg.stats.StartTime,
		TotalRequests:   cfg.stats.TotalRequests,
		IPCounts:       make(map[string]int),
		UserAgents:     make(map[string]int),
		RecentRequests: make([]RequestInfo, len(cfg.stats.RecentRequests)),
	}
	// Copy maps to avoid holding lock during I/O
	for k, v := range cfg.stats.IPCounts {
		persisted.IPCounts[k] = v
	}
	for k, v := range cfg.stats.UserAgents {
		persisted.UserAgents[k] = v
	}
	// Copy recent requests slice
	copy(persisted.RecentRequests, cfg.stats.RecentRequests)
	cfg.stats.mu.RUnlock()

	statsPath := filepath.Join(cfg.dataDir, statsFileName)
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	if err := os.WriteFile(statsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write stats file: %w", err)
	}

	return nil
}

// loadStats loads aggregated statistics from a JSON file if it exists.
//
// If the file doesn't exist, it returns nil (not an error).
// Returns an error if the file exists but cannot be read or parsed.
func (cfg *Config) loadStats() error {
	if cfg.dataDir == "" {
		return nil // Persistence disabled
	}

	statsPath := filepath.Join(cfg.dataDir, statsFileName)
	data, err := os.ReadFile(statsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return fmt.Errorf("failed to read stats file: %w", err)
	}

	var persisted PersistedStats
	if err := json.Unmarshal(data, &persisted); err != nil {
		return fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	cfg.stats.mu.Lock()
	defer cfg.stats.mu.Unlock()

	// Restore stats, but keep the current StartTime for this session
	cfg.stats.TotalRequests = persisted.TotalRequests
	cfg.stats.IPCounts = persisted.IPCounts
	cfg.stats.UserAgents = persisted.UserAgents
	
	// Restore recent requests, but limit to MaxRecentRequests
	if len(persisted.RecentRequests) > cfg.stats.MaxRecentRequests {
		// Keep only the most recent requests
		startIdx := len(persisted.RecentRequests) - cfg.stats.MaxRecentRequests
		cfg.stats.RecentRequests = persisted.RecentRequests[startIdx:]
	} else {
		cfg.stats.RecentRequests = persisted.RecentRequests
	}

	return nil
}

// startPeriodicSave starts a goroutine that periodically saves stats to disk.
func (cfg *Config) startPeriodicSave() {
	if cfg.dataDir == "" {
		return // Persistence disabled
	}

	go func() {
		ticker := time.NewTicker(statsSaveInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := cfg.saveStats(); err != nil {
					cfg.logger.Warn("Failed to save stats periodically", "error", err)
				}
			case <-cfg.saveCtx.Done():
				return
			}
		}
	}()
}

// stopPeriodicSave stops the periodic stats saving goroutine.
func (cfg *Config) stopPeriodicSave() {
	if cfg.saveCancel != nil {
		cfg.saveCancel()
	}
}

// getClientIP extracts the client IP address from the request.
//
// It checks X-Forwarded-For and X-Real-IP headers for proxied requests,
// falling back to RemoteAddr if those headers are not present.
//
// Parameters:
//   - r: the HTTP request
//
// Returns the client IP address as a string.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (first IP in comma-separated list)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// Fall back to RemoteAddr, removing port if present
	ip := r.RemoteAddr
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}
	return ip
}

// recordRequest records request information in the stats.
//
// Parameters:
//   - r: the HTTP request
func (cfg *Config) recordRequest(r *http.Request) {
	ip := getClientIP(r)
	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		userAgent = "Unknown"
	}
	path := r.URL.Path
	now := time.Now()

	// Create request info
	reqInfo := RequestInfo{
		IP:        ip,
		UserAgent: userAgent,
		Path:      path,
		Timestamp: now,
	}

	// Append to NDJSON log file (async, non-blocking)
	cfg.appendRequestToLog(reqInfo)

	cfg.stats.mu.Lock()
	defer cfg.stats.mu.Unlock()

	cfg.stats.TotalRequests++
	cfg.stats.IPCounts[ip]++
	cfg.stats.UserAgents[userAgent]++

	// Add to recent requests
	cfg.stats.RecentRequests = append(cfg.stats.RecentRequests, reqInfo)

	// Keep only the most recent requests
	if len(cfg.stats.RecentRequests) > cfg.stats.MaxRecentRequests {
		cfg.stats.RecentRequests = cfg.stats.RecentRequests[1:]
	}
}

// handleRequest handles HTTP requests by generating and serving HTML pages.
//
// It adds a configurable delay to simulate real-world response times,
// sets the Content-Type header to text/html, and writes the generated
// HTML page to the response writer. It also records request statistics.
// Requests to the admin path are excluded from statistics.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request
func (cfg *Config) handleRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Only record requests that are not to the admin path
	if r.URL.Path != cfg.adminPath {
		cfg.recordRequest(r)
	}

	// Add delay to simulate real-world response times, respecting context cancellation
	select {
	case <-ctx.Done():
		// Request was cancelled or timed out
		cfg.logger.Warn("Request cancelled or timed out", "path", r.URL.Path, "error", ctx.Err())
		return
	case <-time.After(time.Duration(delayMilliseconds) * time.Millisecond):
		// Delay completed
	}

	// Check context again before writing response
	if ctx.Err() != nil {
		return
	}

	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, cfg.generatePage())
}

// printUsage prints the command-line usage information to stdout.
// It displays the program name, available flags, and their descriptions.
func printUsage() {
	fmt.Println("Usage:", os.Args[0], "[-p PORT] -a HTML_FILE -w WORDLIST_FILE [-e ENDPOINT] [-d DATA_DIR]")
	fmt.Println()
	fmt.Println("-p   Port to run the server on (default: 8000)")
	fmt.Println("-a   HTML file input, replace <a href> links")
	fmt.Println("-e   Endpoint to point form GET requests to (optional)")
	fmt.Println("-w   Wordlist to use for links")
	fmt.Println("-d   Data directory for persistence (default: data, empty to disable)")
}

func main() {
	cfg := newConfig()
	var htmlFile string
	var wordlistFile string

	flag.StringVar(&cfg.port, "p", defaultPort, "Port to run the server on")
	flag.StringVar(&htmlFile, "a", "", "HTML file containing links to be replaced")
	flag.StringVar(&wordlistFile, "w", "", "Wordlist file to use for links")
	flag.StringVar(&cfg.endpoint, "e", "", "Endpoint to point form GET requests to")
	flag.StringVar(&cfg.dataDir, "d", defaultDataDir, "Data directory for persistence (empty to disable)")
	flag.Usage = printUsage
	flag.Parse()

	// Load HTML template if provided
	if htmlFile != "" {
		content, err := os.ReadFile(htmlFile)
		if err != nil {
			cfg.logger.Error("Failed to read HTML file", "file", htmlFile, "error", err)
			os.Exit(1)
		}
		if len(content) > maxHTMLTemplateSize {
			cfg.logger.Error("HTML file too large", "file", htmlFile, "size", len(content), "max", maxHTMLTemplateSize)
			os.Exit(1)
		}
		cfg.htmlTemplate = string(content)
		cfg.logger.Info("Loaded HTML template", "file", htmlFile, "size", len(content))
	}

	// Load wordlist if provided
	if wordlistFile != "" {
		if err := cfg.loadWordlist(wordlistFile); err != nil {
			cfg.logger.Error("Failed to load wordlist", "file", wordlistFile, "error", err)
			os.Exit(1)
		}
		if len(cfg.webpages) == 0 {
			cfg.logger.Warn("No links found in wordlist file, using randomly generated links", "file", wordlistFile)
		} else {
			cfg.logger.Info("Loaded wordlist", "file", wordlistFile, "entries", len(cfg.webpages))
		}
	}

	// Setup persistence if data directory is specified
	if cfg.dataDir != "" {
		if err := cfg.ensureDataDir(); err != nil {
			cfg.logger.Error("Failed to create data directory", "dir", cfg.dataDir, "error", err)
			os.Exit(1)
		}

		// Load existing stats if available
		if err := cfg.loadStats(); err != nil {
			cfg.logger.Warn("Failed to load stats", "error", err)
		} else {
			statsPath := filepath.Join(cfg.dataDir, statsFileName)
			cfg.logger.Info("Loaded existing stats", "file", statsPath)
		}

		// Open log file for appending requests
		if err := cfg.openLogFile(); err != nil {
			cfg.logger.Error("Failed to open log file", "error", err)
			os.Exit(1)
		}
		defer cfg.closeLogFile()

		// Start periodic stats saving
		cfg.startPeriodicSave()
		defer cfg.stopPeriodicSave()
		cfg.logger.Info("Persistence enabled", "dataDir", cfg.dataDir)
	}

	// Validate and setup server
	if err := cfg.validatePort(); err != nil {
		cfg.logger.Error("Invalid port configuration", "port", cfg.port, "error", err)
		os.Exit(1)
	}

	// Generate random admin endpoint and token
	// Ensure admin path doesn't conflict with root or common paths
	cfg.adminPath = cfg.generateAdminPath()
	for cfg.adminPath == "/" || strings.HasPrefix(cfg.adminPath, "/data") {
		cfg.adminPath = cfg.generateAdminPath()
	}
	cfg.adminToken = cfg.generateAdminToken()

	server := cfg.createHTTPServer()
	// Register admin handlers first so they take precedence over the root handler
	http.HandleFunc(cfg.adminPath+"/login", cfg.handleAdminLogin)
	http.HandleFunc(cfg.adminPath+"/data", cfg.handleChartData)
	http.HandleFunc(cfg.adminPath, cfg.handleAdminUI)
	http.HandleFunc("/", cfg.handleRequest)

	// Log admin UI URLs
	adminURL := fmt.Sprintf("http://localhost:%s%s?token=%s", cfg.port, cfg.adminPath, cfg.adminToken)
	adminLoginURL := fmt.Sprintf("http://localhost:%s%s/login?token=%s", cfg.port, cfg.adminPath, cfg.adminToken)
	cfg.logger.Info("Admin UI available", "url", adminURL)
	cfg.logger.Info("Admin login endpoint", "url", adminLoginURL)

	// Start server and handle graceful shutdown
	cfg.runServer(server)
}

// loadWordlist loads wordlist entries from a file, one entry per line.
//
// Empty lines and lines containing only whitespace are ignored.
// Each non-empty line is trimmed of leading and trailing whitespace
// before being added to the wordlist.
//
// Parameters:
//   - filename: the path to the wordlist file
//
// Returns an error if the file cannot be opened or read, or if it exceeds size limits.
func (cfg *Config) loadWordlist(filename string) error {
	// Check file size before reading
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("can't read wordlist file: %w", err)
	}
	if fileInfo.Size() > maxWordlistFileSize {
		return fmt.Errorf("wordlist file too large (%d bytes, max %d bytes)", fileInfo.Size(), maxWordlistFileSize)
	}

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("can't read wordlist file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if len(cfg.webpages) >= maxWordlistEntries {
				return fmt.Errorf("wordlist has too many entries (max %d)", maxWordlistEntries)
			}
			cfg.webpages = append(cfg.webpages, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading wordlist file: %w", err)
	}

	return nil
}

// validatePort validates that the configured port number is valid.
//
// A valid port must be:
//   - Numeric (can be converted to an integer)
//   - Within the valid TCP/UDP port range (1-65535)
//
// Returns an error describing the validation failure, or nil if valid.
func (cfg *Config) validatePort() error {
	portNum, err := strconv.Atoi(cfg.port)
	if err != nil {
		return fmt.Errorf("invalid port number: %s (must be numeric)", cfg.port)
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("invalid port number: %s (must be between 1 and 65535)", cfg.port)
	}
	return nil
}

// createHTTPServer creates and configures an HTTP server with appropriate timeouts.
//
// The server is configured with:
//   - ReadTimeout: maximum duration for reading the entire request
//   - WriteTimeout: maximum duration for writing the response
//   - IdleTimeout: maximum duration to wait for the next request when keep-alives are enabled
//
// The server address is set to listen on the configured port.
//
// Returns a configured http.Server instance ready to be started.
func (cfg *Config) createHTTPServer() *http.Server {
	return &http.Server{
		Addr:         ":" + cfg.port,
		ReadTimeout:  readTimeoutSeconds * time.Second,
		WriteTimeout: writeTimeoutSeconds * time.Second,
		IdleTimeout:  idleTimeoutSeconds * time.Second,
	}
}

// runServer starts the HTTP server and handles graceful shutdown on interrupt signals.
//
// The server is started in a background goroutine. The function blocks waiting
// for SIGINT or SIGTERM signals. When received, it initiates a graceful shutdown
// with a configurable timeout. If the shutdown timeout is exceeded, the function
// exits with an error.
//
// Parameters:
//   - server: the HTTP server instance to start and manage
func (cfg *Config) runServer(server *http.Server) {
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in background
	go func() {
		cfg.logger.Info("Starting HTTP server", "port", cfg.port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			cfg.logger.Error("HTTP server error", "port", cfg.port, "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	cfg.logger.Info("Shutdown signal received, shutting down server")

	// Stop periodic saving
	cfg.stopPeriodicSave()

	// Save final stats snapshot
	if cfg.dataDir != "" {
		if err := cfg.saveStats(); err != nil {
			cfg.logger.Warn("Failed to save final stats", "error", err)
		} else {
			cfg.logger.Info("Saved stats to disk")
		}
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeoutSeconds*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		cfg.logger.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	cfg.logger.Info("Server stopped gracefully")
}

