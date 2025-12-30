package ui

import (
	"fmt"
	"time"
)

// Banner is the ASCII art banner for gospidertrap
const Banner = `
  __ _  ___  ___ _ __ (_) __| | ___ _ __| |_ _ __ __ _ _ __
 / _' |/ _ \/ __| '_ \| |/ _' |/ _ \ '__| __| '__/ _' | '_ \
| (_| | (_) \__ \ |_) | | (_| |  __/ |  | |_| | | (_| | |_) |
 \__, |\___/|___/ .__/|_|\__,_|\___|_|   \__|_|  \__,_| .__/
 |___/          |_|                                    |_|
`

// StartupInfo holds configuration information to display at startup
type StartupInfo struct {
	Port          string
	AdminLoginURL string
	AdminURL      string
	PersistMode   string
	RateLimit     string
	Wordlist      string
	Template      string
}

// PrintBanner prints the ASCII banner
func PrintBanner() {
	fmt.Print(Banner)
}

// PrintStartupInfo prints a clean summary of the server configuration
func PrintStartupInfo(info StartupInfo) {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Server started at %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Server Info
	fmt.Println("  SERVER")
	fmt.Printf("     Port:            %s\n", info.Port)
	fmt.Printf("     Rate Limiting:   %s\n", info.RateLimit)
	fmt.Println()

	// Content Configuration
	fmt.Println("  CONTENT")
	if info.Wordlist != "" {
		fmt.Printf("     Wordlist:        %s\n", info.Wordlist)
	} else {
		fmt.Printf("     Wordlist:        Random links\n")
	}
	if info.Template != "" {
		fmt.Printf("     Template:        %s\n", info.Template)
	} else {
		fmt.Printf("     Template:        Generated pages\n")
	}
	fmt.Println()

	// Persistence
	fmt.Println("   PERSISTENCE")
	fmt.Printf("     Mode:            %s\n", info.PersistMode)
	fmt.Println()

	// Admin Access
	fmt.Println("  ADMIN ACCESS")
	fmt.Printf("     Login URL:       %s\n", info.AdminLoginURL)
	fmt.Printf("     Dashboard:       %s\n", info.AdminURL)
	fmt.Println()
	fmt.Println("  ⚠️  SECURITY WARNING:")
	fmt.Println("     The login URL above contains a one-time authentication token.")
	fmt.Println("     Keep it secure and rotate logs containing this token.")
	fmt.Println()

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  Press Ctrl+C to stop the server")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

// PrintShutdown prints a shutdown message
func PrintShutdown() {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  Server shutting down gracefully...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// PrintShutdownComplete prints a final shutdown message
func PrintShutdownComplete() {
	fmt.Println()
	fmt.Println("  ✓ Server stopped successfully")
	fmt.Println()
}

// PrintError prints a formatted error message
func PrintError(message string, err error) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  ❌ ERROR: %s\n", message)
	if err != nil {
		fmt.Printf("     %v\n", err)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

// FormatSize formats a byte size into a human-readable string
func FormatSize(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// BuildWordlistSummary creates a summary string for wordlist info
func BuildWordlistSummary(filename string, entries int) string {
	if filename == "" {
		return ""
	}
	return fmt.Sprintf("%s (%d entries)", filename, entries)
}

// BuildTemplateSummary creates a summary string for template info
func BuildTemplateSummary(filename string, size int) string {
	if filename == "" {
		return ""
	}
	return fmt.Sprintf("%s (%s)", filename, FormatSize(size))
}

// BuildRateLimitSummary creates a summary string for rate limiting
func BuildRateLimitSummary(requestsPerSec, burst int) string {
	return fmt.Sprintf("%d req/sec (burst: %d)", requestsPerSec, burst)
}

// BuildPersistModeSummary creates a summary string for persistence mode
func BuildPersistModeSummary(useFiles bool, dbPath, dataDir string) string {
	if useFiles {
		return fmt.Sprintf("File-based (legacy) - %s", dataDir)
	}
	if dbPath != "" {
		return fmt.Sprintf("SQLite database - %s", dbPath)
	}
	return "Disabled"
}
