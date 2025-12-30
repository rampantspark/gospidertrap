package handler

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/rampantspark/gospidertrap/internal/content"
	"github.com/rampantspark/gospidertrap/internal/stats"
)

// RequestHandler handles HTTP requests by generating and serving HTML pages.
//
// It coordinates between content generation, statistics tracking, and response
// timing to create a realistic spider trap.
type RequestHandler struct {
	content *content.Generator
	stats   *stats.Manager
	logger  *slog.Logger
	delay   time.Duration
}

// New creates a new request handler.
//
// Parameters:
//   - content: the content generator for creating HTML pages
//   - stats: the stats manager for recording requests
//   - logger: structured logger instance
//   - delay: delay to add to each request to simulate real-world response times
//
// Returns a new RequestHandler instance.
func New(content *content.Generator, stats *stats.Manager, logger *slog.Logger, delay time.Duration) *RequestHandler {
	return &RequestHandler{
		content: content,
		stats:   stats,
		logger:  logger,
		delay:   delay,
	}
}

// Handle handles an HTTP request by recording stats, adding delay, and serving content.
//
// The method:
//  1. Records request statistics using the stats manager
//  2. Adds a configurable delay to simulate real-world response times
//  3. Generates and serves an HTML page with random links
//
// The delay respects context cancellation, so if the client disconnects or
// the request times out, the response is not written.
//
// Parameters:
//   - w: the HTTP response writer
//   - r: the HTTP request
func (h *RequestHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Record request statistics
	if err := h.stats.RecordRequest(r); err != nil {
		h.logger.Warn("Failed to record request", "error", err)
	}

	// Add delay to simulate real-world response times, respecting context cancellation
	select {
	case <-ctx.Done():
		// Request was cancelled or timed out
		h.logger.Warn("Request cancelled or timed out", "path", r.URL.Path, "error", ctx.Err())
		return
	case <-time.After(h.delay):
		// Delay completed
	}

	// Check context again before writing response
	if ctx.Err() != nil {
		return
	}

	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, h.content.GeneratePage())
}
