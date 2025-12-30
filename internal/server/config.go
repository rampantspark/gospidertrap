package server

import (
	"fmt"
	"strconv"
	"time"
)

// Config holds HTTP server configuration parameters.
//
// It defines timeouts, size limits, and network settings for the HTTP server.
type Config struct {
	Port           string        // Port number to listen on
	ReadTimeout    time.Duration // Maximum duration for reading the entire request
	WriteTimeout   time.Duration // Maximum duration for writing the response
	IdleTimeout    time.Duration // Maximum duration to wait for next request with keep-alives
	MaxHeaderBytes int           // Maximum size of request headers
}

// Validate validates that the server configuration is valid.
//
// Currently only validates the port number. A valid port must be:
//   - Numeric (can be converted to an integer)
//   - Within the valid TCP/UDP port range (1-65535)
//
// Returns an error describing the validation failure, or nil if valid.
func (c *Config) Validate() error {
	portNum, err := strconv.Atoi(c.Port)
	if err != nil {
		return fmt.Errorf("invalid port number: %s (must be numeric)", c.Port)
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("invalid port number: %s (must be between 1 and 65535)", c.Port)
	}
	return nil
}
