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
	"flag"
	"fmt"
	"html"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
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
)

// Config holds the application configuration and state.
// It manages wordlists, HTML templates, server settings, and random number generation.
type Config struct {
	webpages     []string    // Wordlist entries to use for link generation
	chars        []rune      // Character set for random string generation
	port         string      // Port number for the HTTP server
	endpoint     string      // Form submission endpoint (optional)
	htmlTemplate string      // HTML template file content (optional)
	rand         *rand.Rand  // Random number generator instance
}

// newConfig creates and initializes a new Config instance with default values.
// It initializes the random number generator with a time-based seed.
func newConfig() *Config {
	return &Config{
		chars: []rune(charSpace),
		rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
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

// handleRequest handles HTTP requests by generating and serving HTML pages.
//
// It adds a configurable delay to simulate real-world response times,
// sets the Content-Type header to text/html, and writes the generated
// HTML page to the response writer.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request (currently unused but required by the interface)
func (cfg *Config) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Add delay to simulate real-world response times
	time.Sleep(time.Duration(delayMilliseconds) * time.Millisecond)
	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, cfg.generatePage())
}

// printUsage prints the command-line usage information to stdout.
// It displays the program name, available flags, and their descriptions.
func printUsage() {
	fmt.Println("Usage:", os.Args[0], "[-p PORT] -a HTML_FILE -w WORDLIST_FILE [-e ENDPOINT]")
	fmt.Println()
	fmt.Println("-p   Port to run the server on (default: 8000)")
	fmt.Println("-a   HTML file input, replace <a href> links")
	fmt.Println("-e   Endpoint to point form GET requests to (optional)")
	fmt.Println("-w   Wordlist to use for links")
}

func main() {
	cfg := newConfig()
	var htmlFile string
	var wordlistFile string

	flag.StringVar(&cfg.port, "p", defaultPort, "Port to run the server on")
	flag.StringVar(&htmlFile, "a", "", "HTML file containing links to be replaced")
	flag.StringVar(&wordlistFile, "w", "", "Wordlist file to use for links")
	flag.StringVar(&cfg.endpoint, "e", "", "Endpoint to point form GET requests to")
	flag.Usage = printUsage
	flag.Parse()

	// Load HTML template if provided
	if htmlFile != "" {
		content, err := os.ReadFile(htmlFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: can't read HTML file: %v\n", err)
			os.Exit(1)
		}
		cfg.htmlTemplate = string(content)
	}

	// Load wordlist if provided
	if wordlistFile != "" {
		if err := cfg.loadWordlist(wordlistFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v. Using randomly generated links.\n", err)
		}
		if len(cfg.webpages) == 0 {
			fmt.Println("No links found in the wordlist file. Using randomly generated links.")
		}
	}

	// Validate and setup server
	if err := cfg.validatePort(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	server := cfg.createHTTPServer()
	http.HandleFunc("/", cfg.handleRequest)

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
// Returns an error if the file cannot be opened or read.
func (cfg *Config) loadWordlist(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("can't read wordlist file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
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
		fmt.Println("Starting server on port", cfg.port, "...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Error: starting HTTP server on port %s: %v\n", cfg.port, err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeoutSeconds*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: server forced to shutdown: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Server stopped gracefully")
}

