package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Server manages the HTTP server lifecycle.
//
// It wraps an http.Server with configuration and provides methods for
// starting, stopping, and graceful shutdown.
type Server struct {
	config *Config
	server *http.Server
	logger *slog.Logger
}

// New creates a new server instance.
//
// Parameters:
//   - config: server configuration (timeouts, port, size limits)
//   - logger: structured logger instance
//
// Returns a new Server instance.
func New(config *Config, logger *slog.Logger) *Server {
	return &Server{
		config: config,
		server: &http.Server{
			Addr:           ":" + config.Port,
			ReadTimeout:    config.ReadTimeout,
			WriteTimeout:   config.WriteTimeout,
			IdleTimeout:    config.IdleTimeout,
			MaxHeaderBytes: config.MaxHeaderBytes,
		},
		logger: logger,
	}
}

// RegisterHandler sets the HTTP handler for the server.
//
// This should be called before starting the server.
//
// Parameters:
//   - handler: the HTTP handler to register
func (s *Server) RegisterHandler(handler http.Handler) {
	s.server.Handler = handler
}

// Start starts the HTTP server.
//
// This is a blocking call that starts the server and waits for it to exit.
// Returns an error if the server fails to start or encounters a fatal error.
// Returns nil if the server is shut down gracefully.
func (s *Server) Start() error {
	s.logger.Debug("Starting HTTP server", "port", s.config.Port)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the server with a timeout.
//
// It waits for active connections to finish before shutting down, up to
// the specified timeout duration.
//
// Parameters:
//   - ctx: context with timeout for shutdown
//
// Returns an error if the shutdown fails or times out.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Debug("Shutting down server")
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Server forced to shutdown", "error", err)
		return err
	}
	s.logger.Debug("Server stopped gracefully")
	return nil
}

// GracefulShutdown starts the server and handles graceful shutdown on signals.
//
// This method:
//  1. Starts the server in a background goroutine
//  2. Waits for SIGINT or SIGTERM signals
//  3. Initiates graceful shutdown with the specified timeout
//  4. Calls the cleanup function before shutting down (if provided)
//
// This is a blocking call that runs until the server is shut down.
//
// Parameters:
//   - timeout: maximum time to wait for graceful shutdown
//   - cleanup: optional function to call before shutdown (can be nil)
func (s *Server) GracefulShutdown(timeout time.Duration, cleanup func()) {
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in background
	go func() {
		if err := s.Start(); err != nil {
			s.logger.Error("HTTP server error", "port", s.config.Port, "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	s.logger.Debug("Shutdown signal received")

	// Call cleanup function if provided
	if cleanup != nil {
		cleanup()
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		os.Exit(1)
	}
}
