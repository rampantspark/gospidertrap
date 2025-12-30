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
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rampantspark/gospidertrap/internal/admin"
	"github.com/rampantspark/gospidertrap/internal/content"
	"github.com/rampantspark/gospidertrap/internal/handler"
	"github.com/rampantspark/gospidertrap/internal/logging"
	"github.com/rampantspark/gospidertrap/internal/middleware"
	"github.com/rampantspark/gospidertrap/internal/random"
	"github.com/rampantspark/gospidertrap/internal/ratelimit"
	"github.com/rampantspark/gospidertrap/internal/server"
	"github.com/rampantspark/gospidertrap/internal/stats"
	"github.com/rampantspark/gospidertrap/internal/ui"
)

// Configuration constants for server behavior.
const (
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

	// HTTP server size limits
	maxHeaderBytes      = 1 << 20 // 1 MB max header size
	maxRequestBodyBytes = 1 << 20 // 1 MB max request body size


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

)

// Config holds the application configuration and state.
// It manages wordlists, HTML templates, server settings, and random number generation.
type Config struct {
	port         string              // Port number for the HTTP server
	contentGen   *content.Generator  // HTML page generator
	statsManager *stats.Manager      // Unified stats manager (database or file-based)
	statsBackend *stats.Stats        // In-memory stats (for file-based mode only)
	db           *stats.Database     // Database instance (for database mode only)
	adminHandler *admin.Handler      // Admin UI handler
	rateLimitReq int                 // Rate limit: requests per second
	rateLimitBurst int               // Rate limit: burst size
	dataDir      string              // Directory for persisting data files
	dbPath       string              // Path to SQLite database file
	logFile      *os.File            // File handle for NDJSON request log
	logFileMu    sync.Mutex          // Mutex for thread-safe log file writes
	logger       *slog.Logger        // Structured logger instance
	saveCtx      context.Context     // Context for periodic stats saving
	saveCancel   context.CancelFunc  // Cancel function for periodic stats saving
	useHTTPS     bool                // Whether HTTPS is being used (affects cookie Secure flag)
	trustProxy   bool                // Whether to trust X-Forwarded-For and X-Real-IP headers
	useFiles     bool                // Whether to use file-based persistence (vs SQLite)
}

// newConfig creates and initializes a new Config instance with default values.
// It initializes the random number generator with a time-based seed and sets up structured logging.
func newConfig() *Config {
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(logging.NewHumanReadableHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove time attribute
			if a.Key == "time" {
				return slog.Attr{}
			}
			// Replace level attribute with custom values
			if a.Key == "level" {
				level, ok := a.Value.Any().(slog.Level)
				if !ok {
					return a
				}
				var levelStr string
				switch level {
				case slog.LevelDebug:
					levelStr = "[%]"
				case slog.LevelInfo:
					levelStr = "[*]"
				case slog.LevelWarn:
					levelStr = "[?]"
				case slog.LevelError:
					levelStr = "[!]"
				default:
					levelStr = level.String()
				}
				return slog.Attr{Key: "level", Value: slog.StringValue(levelStr)}
			}
			return a
		},
	}))
	return &Config{
		statsBackend: stats.NewStats(),
		dataDir:      defaultDataDir,
		logger:       logger,
		saveCtx:      ctx,
		saveCancel:   cancel,
	}
}


// ensureDataDir creates the data directory if it doesn't exist.
//
// Returns an error if the directory cannot be created.
func (cfg *Config) ensureDataDir() error {
	if cfg.dataDir == "" {
		return nil // Persistence disabled
	}
	return os.MkdirAll(cfg.dataDir, 0750)
}

// openLogFile opens or creates the NDJSON log file for appending requests.
//
// Returns an error if the file cannot be opened.
func (cfg *Config) openLogFile() error {
	if cfg.dataDir == "" {
		return nil // Persistence disabled
	}

	logPath := filepath.Join(cfg.dataDir, requestsLogFileName)

	// Validate path is within data directory
	if err := validateDataDirPath(cfg.dataDir, logPath); err != nil {
		return fmt.Errorf("invalid log file path: %w", err)
	}

	// #nosec G304 -- path validated by validateDataDirPath to prevent traversal
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
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
func (cfg *Config) appendRequestToLog(reqInfo stats.RequestInfo) {
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

	cfg.statsBackend.Mu.RLock()
	persisted := stats.PersistedStats{
		StartTime:      cfg.statsBackend.StartTime,
		TotalRequests:   cfg.statsBackend.TotalRequests,
		IPCounts:       make(map[string]int),
		UserAgents:     make(map[string]int),
		RecentRequests: make([]stats.RequestInfo, len(cfg.statsBackend.RecentRequests)),
	}
	// Copy maps to avoid holding lock during I/O
	for k, v := range cfg.statsBackend.IPCounts {
		persisted.IPCounts[k] = v
	}
	for k, v := range cfg.statsBackend.UserAgents {
		persisted.UserAgents[k] = v
	}
	// Copy recent requests slice
	copy(persisted.RecentRequests, cfg.statsBackend.RecentRequests)
	cfg.statsBackend.Mu.RUnlock()

	statsPath := filepath.Join(cfg.dataDir, statsFileName)
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	if err := os.WriteFile(statsPath, data, 0600); err != nil {
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

	// Validate path is within data directory
	if err := validateDataDirPath(cfg.dataDir, statsPath); err != nil {
		return fmt.Errorf("invalid stats file path: %w", err)
	}

	// #nosec G304 -- path validated by validateDataDirPath to prevent traversal
	data, err := os.ReadFile(statsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return fmt.Errorf("failed to read stats file: %w", err)
	}

	var persisted stats.PersistedStats
	if err := json.Unmarshal(data, &persisted); err != nil {
		return fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	cfg.statsBackend.Mu.Lock()
	defer cfg.statsBackend.Mu.Unlock()

	// Restore stats, but keep the current StartTime for this session
	cfg.statsBackend.TotalRequests = persisted.TotalRequests
	cfg.statsBackend.IPCounts = persisted.IPCounts
	cfg.statsBackend.UserAgents = persisted.UserAgents

	// Restore recent requests, but limit to MaxRecentRequests
	if len(persisted.RecentRequests) > cfg.statsBackend.MaxRecentRequests {
		// Keep only the most recent requests
		startIdx := len(persisted.RecentRequests) - cfg.statsBackend.MaxRecentRequests
		cfg.statsBackend.RecentRequests = persisted.RecentRequests[startIdx:]
	} else {
		cfg.statsBackend.RecentRequests = persisted.RecentRequests
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
// If trustProxy is true, it checks X-Forwarded-For and X-Real-IP headers for
// proxied requests and validates that they contain valid IP addresses.
// Invalid IPs or untrusted headers are ignored.
// Falls back to RemoteAddr if proxy headers are not present or not trusted.
//
// Parameters:
//   - r: the HTTP request
//   - trustProxy: whether to trust proxy headers
//
// Returns the client IP address as a string.
func getClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		// Check X-Forwarded-For header (first IP in comma-separated list)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				ip := strings.TrimSpace(ips[0])
				// Validate IP format
				if parsedIP := net.ParseIP(ip); parsedIP != nil {
					return ip
				}
			}
		}
		// Check X-Real-IP header
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			ip := strings.TrimSpace(xri)
			// Validate IP format
			if parsedIP := net.ParseIP(ip); parsedIP != nil {
				return ip
			}
		}
	}
	// Fall back to RemoteAddr, removing port if present
	ip := r.RemoteAddr
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}
	return ip
}

// printUsage prints the command-line usage information to stdout.
// It displays the program name, available flags, and their descriptions.
func printUsage() {
	fmt.Println("Usage:", os.Args[0], "[-p PORT] -a HTML_FILE -w WORDLIST_FILE [-e ENDPOINT] [-d DATA_DIR] [-db-path DB_FILE] [-use-files] [-rate-limit N] [-rate-burst N] [-https] [-trust-proxy]")
	fmt.Println()
	fmt.Println("-p            Port to run the server on (default: 8000)")
	fmt.Println("-a            HTML file input, replace <a href> links")
	fmt.Println("-e            Endpoint to point form GET requests to (optional)")
	fmt.Println("-w            Wordlist to use for links")
	fmt.Println("-d            Data directory for persistence (default: data, empty to disable)")
	fmt.Println("-db-path      Path to SQLite database file (default: data/stats.db)")
	fmt.Println("-use-files    Use legacy file-based persistence instead of SQLite")
	fmt.Println("-rate-limit   Rate limit: requests per second per IP (default: 10)")
	fmt.Println("-rate-burst   Rate limit: burst size per IP (default: 20)")
	fmt.Println("-https        Enable HTTPS mode (sets Secure flag on cookies)")
	fmt.Println("-trust-proxy  Trust X-Forwarded-For and X-Real-IP headers (use when behind reverse proxy)")
}

func main() {
	// Print banner
	ui.PrintBanner()

	cfg := newConfig()
	var htmlFile string
	var wordlistFile string
	var endpoint string

	flag.StringVar(&cfg.port, "p", defaultPort, "Port to run the server on")
	flag.StringVar(&htmlFile, "a", "", "HTML file containing links to be replaced")
	flag.StringVar(&wordlistFile, "w", "", "Wordlist file to use for links")
	flag.StringVar(&endpoint, "e", "", "Endpoint to point form GET requests to")
	flag.StringVar(&cfg.dataDir, "d", defaultDataDir, "Data directory for persistence (empty to disable)")
	flag.StringVar(&cfg.dbPath, "db-path", "", "Path to SQLite database file (default: data/stats.db, uses SQLite by default)")
	flag.BoolVar(&cfg.useFiles, "use-files", false, "Use legacy file-based persistence instead of SQLite")
	flag.IntVar(&cfg.rateLimitReq, "rate-limit", 10, "Rate limit: requests per second per IP")
	flag.IntVar(&cfg.rateLimitBurst, "rate-burst", 20, "Rate limit: burst size per IP")
	flag.BoolVar(&cfg.useHTTPS, "https", false, "Enable HTTPS mode (sets Secure flag on cookies)")
	flag.BoolVar(&cfg.trustProxy, "trust-proxy", false, "Trust X-Forwarded-For and X-Real-IP headers")
	flag.Usage = printUsage
	flag.Parse()

	// Validate rate limiting parameters
	if cfg.rateLimitReq <= 0 {
		ui.PrintError("Rate limit must be positive", fmt.Errorf("rate-limit=%d", cfg.rateLimitReq))
		os.Exit(1)
	}
	if cfg.rateLimitBurst <= 0 {
		ui.PrintError("Rate limit burst must be positive", fmt.Errorf("rate-burst=%d", cfg.rateLimitBurst))
		os.Exit(1)
	}
	if cfg.rateLimitBurst < cfg.rateLimitReq {
		ui.PrintError("Rate limit burst should be >= rate limit for optimal performance",
			fmt.Errorf("burst=%d, rate=%d", cfg.rateLimitBurst, cfg.rateLimitReq))
		os.Exit(1)
	}

	// Set default database path if not specified and not using files
	if cfg.dbPath == "" && !cfg.useFiles && cfg.dataDir != "" {
		cfg.dbPath = filepath.Join(cfg.dataDir, "stats.db")
	}

	// Load HTML template if provided
	var htmlTemplate string
	var htmlTemplateSize int
	if htmlFile != "" {
		// Validate file path to prevent directory traversal
		if err := validateFilePath(htmlFile); err != nil {
			ui.PrintError("Invalid HTML file path", err)
			os.Exit(1)
		}

		// #nosec G304 -- path validated by validateFilePath to prevent traversal
		content, err := os.ReadFile(htmlFile)
		if err != nil {
			ui.PrintError("Failed to read HTML file", err)
			os.Exit(1)
		}
		if len(content) > maxHTMLTemplateSize {
			ui.PrintError(fmt.Sprintf("HTML file too large: %s (max %s)", ui.FormatSize(len(content)), ui.FormatSize(maxHTMLTemplateSize)), nil)
			os.Exit(1)
		}
		htmlTemplate = string(content)
		htmlTemplateSize = len(content)
	}

	// Load wordlist if provided
	var wordlist []string
	if wordlistFile != "" {
		var err error
		wordlist, err = loadWordlist(wordlistFile)
		if err != nil {
			ui.PrintError("Failed to load wordlist", err)
			os.Exit(1)
		}
	}

	// Initialize content generator
	randomSrc := random.NewSource(charSpace, time.Now().UnixNano())
	cfg.contentGen = content.NewGenerator(wordlist, htmlTemplate, endpoint, randomSrc)

	// Setup persistence
	if cfg.dataDir != "" {
		if err := cfg.ensureDataDir(); err != nil {
			ui.PrintError("Failed to create data directory", err)
			os.Exit(1)
		}
	}

	if cfg.useFiles {
		// Legacy file-based persistence
		if cfg.dataDir != "" {
			// Load existing stats if available
			if err := cfg.loadStats(); err != nil {
				// Not a critical error, just log it
				cfg.logger.Debug("Failed to load existing stats", "error", err)
			}

			// Open log file for appending requests
			if err := cfg.openLogFile(); err != nil {
				ui.PrintError("Failed to open log file", err)
				os.Exit(1)
			}
			defer cfg.closeLogFile()

			// Start periodic stats saving
			cfg.startPeriodicSave()
			defer cfg.stopPeriodicSave()
		}
	} else if cfg.dbPath != "" {
		// SQLite database persistence (default)
		db, err := stats.NewDatabase(cfg.dbPath, cfg.logger)
		if err != nil {
			ui.PrintError("Failed to initialize database", err)
			os.Exit(1)
		}
		cfg.db = db
		defer cfg.db.Close()

		// Run migration from files if they exist
		if cfg.dataDir != "" {
			if err := stats.MigrateFromFiles(db, cfg.dataDir, cfg.logger); err != nil {
				// Not a critical error, just log it
				cfg.logger.Debug("Migration from files failed", "error", err)
			}
		}
	}

	// Create stats manager with appropriate backend
	cfg.statsManager = stats.NewManager(cfg.db, cfg.statsBackend, cfg.trustProxy, cfg.logger)

	// Create rate limiter
	rateLimiter := ratelimit.NewLimiter(cfg.rateLimitReq, cfg.rateLimitBurst)
	defer rateLimiter.Stop()

	// Create and validate server configuration
	serverConfig := &server.Config{
		Port:           cfg.port,
		ReadTimeout:    readTimeoutSeconds * time.Second,
		WriteTimeout:   writeTimeoutSeconds * time.Second,
		IdleTimeout:    idleTimeoutSeconds * time.Second,
		MaxHeaderBytes: maxHeaderBytes,
	}
	if err := serverConfig.Validate(); err != nil {
		ui.PrintError("Invalid server configuration", err)
		os.Exit(1)
	}

	// Create admin handler
	auth, err := admin.NewAuthenticator(cfg.useHTTPS)
	if err != nil {
		ui.PrintError("Failed to create admin authenticator", err)
		os.Exit(1)
	}
	cfg.adminHandler = admin.NewHandler(auth, cfg.statsManager, cfg.logger)

	// Create request handler
	requestHandler := handler.New(
		cfg.contentGen,
		cfg.statsManager,
		cfg.logger,
		time.Duration(delayMilliseconds)*time.Millisecond,
	)

	// Create wrapper for NDJSON logging (file mode only)
	handleRequest := func(w http.ResponseWriter, r *http.Request) {
		// Append to NDJSON log file if using file-based persistence
		if cfg.useFiles && cfg.logFile != nil {
			ip := cfg.statsManager.GetClientIP(r)
			userAgent := r.Header.Get("User-Agent")
			if userAgent == "" {
				userAgent = "Unknown"
			}
			reqInfo := stats.RequestInfo{
				IP:        ip,
				UserAgent: userAgent,
				Path:      r.URL.Path,
				Timestamp: time.Now(),
			}
			cfg.appendRequestToLog(reqInfo)
		}
		// Handle the request
		requestHandler.Handle(w, r)
	}

	// Create ServeMux and register handlers
	mux := http.NewServeMux()
	adminPath := cfg.adminHandler.GetPath()
	mux.HandleFunc(adminPath+"/login", cfg.adminHandler.HandleLogin)
	mux.HandleFunc(adminPath+"/data", cfg.adminHandler.HandleChartData)
	mux.HandleFunc(adminPath, cfg.adminHandler.HandleUI)
	mux.HandleFunc("/", handleRequest)

	// Apply middleware stack (order matters: outermost first)
	// 1. Panic recovery - catch all panics
	// 2. Request body size limit - prevent memory exhaustion
	// 3. Rate limiting - prevent abuse (applies to ALL routes including admin)
	httpHandler := middleware.RecoverPanic(cfg.logger)(
		middleware.LimitRequestBody(maxRequestBodyBytes)(
			middleware.RateLimit(rateLimiter, cfg.statsManager.GetClientIP)(mux),
		),
	)

	// Create and configure server
	srv := server.New(serverConfig, cfg.logger)
	srv.RegisterHandler(httpHandler)

	// Build admin URLs for startup info
	// Note: Admin token is security-sensitive and should not be printed to logs in production
	adminLoginURL := cfg.adminHandler.GetLoginURL("localhost:" + cfg.port)
	adminURL := cfg.adminHandler.GetAdminURL("localhost:" + cfg.port)

	startupInfo := ui.StartupInfo{
		Port:          cfg.port,
		AdminLoginURL: adminLoginURL,
		AdminURL:      adminURL,
		PersistMode:   ui.BuildPersistModeSummary(cfg.useFiles, cfg.dbPath, cfg.dataDir),
		RateLimit:     ui.BuildRateLimitSummary(cfg.rateLimitReq, cfg.rateLimitBurst),
		Wordlist:      ui.BuildWordlistSummary(wordlistFile, len(wordlist)),
		Template:      ui.BuildTemplateSummary(htmlFile, htmlTemplateSize),
	}
	ui.PrintStartupInfo(startupInfo)

	// Define cleanup function for graceful shutdown
	cleanup := func() {
		ui.PrintShutdown()

		// Stop periodic saving
		cfg.stopPeriodicSave()

		// Save final stats snapshot if using file-based persistence
		if cfg.useFiles && cfg.dataDir != "" {
			if err := cfg.saveStats(); err != nil {
				cfg.logger.Debug("Failed to save final stats", "error", err)
			}
		}

		ui.PrintShutdownComplete()
	}

	// Start server with graceful shutdown
	srv.GracefulShutdown(shutdownTimeoutSeconds*time.Second, cleanup)
}

// validateFilePath checks if a file path is safe to access.
// It prevents directory traversal attacks by rejecting paths containing ".."
// and ensures the path is absolute or relative to current directory.
func validateFilePath(path string) error {
	// Check for path traversal sequences
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains directory traversal sequence: %s", path)
	}
	// Additional security: convert to absolute path and check it's readable
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve absolute path: %w", err)
	}
	// Verify file exists and is accessible
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}
	return nil
}

// validateDataDirPath checks if a path is within the data directory.
// This prevents directory traversal for programmatically constructed paths.
func validateDataDirPath(dataDir, filePath string) error {
	// Convert both to absolute paths
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("cannot resolve data directory: %w", err)
	}
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("cannot resolve file path: %w", err)
	}

	// Check if file path is within data directory
	relPath, err := filepath.Rel(absDataDir, absFilePath)
	if err != nil {
		return fmt.Errorf("cannot determine relative path: %w", err)
	}

	// If relative path starts with "..", it's outside the data directory
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("file path is outside data directory: %s", filePath)
	}

	return nil
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
// Returns the wordlist slice and an error if the file cannot be opened or read, or if it exceeds size limits.
func loadWordlist(filename string) ([]string, error) {
	// Validate file path to prevent directory traversal
	if err := validateFilePath(filename); err != nil {
		return nil, fmt.Errorf("invalid wordlist file path: %w", err)
	}

	// Check file size before reading
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("can't read wordlist file: %w", err)
	}
	if fileInfo.Size() > maxWordlistFileSize {
		return nil, fmt.Errorf("wordlist file too large (%d bytes, max %d bytes)", fileInfo.Size(), maxWordlistFileSize)
	}

	// #nosec G304 -- path validated by validateFilePath to prevent traversal
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("can't read wordlist file: %w", err)
	}
	defer file.Close()

	var wordlist []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if len(wordlist) >= maxWordlistEntries {
				return nil, fmt.Errorf("wordlist has too many entries (max %d)", maxWordlistEntries)
			}
			wordlist = append(wordlist, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading wordlist file: %w", err)
	}

	return wordlist, nil
}

