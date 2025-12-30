// Package random provides thread-safe cryptographically secure random number generation.
package random

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
)

// Source provides thread-safe cryptographically secure random number generation with a specific character set.
type Source struct {
	mu    sync.Mutex
	chars []rune
}

// NewSource creates a new random source with the given character set.
//
// Note: The seed parameter is ignored as crypto/rand is used for cryptographic security.
// It's kept for backward compatibility.
//
// Parameters:
//   - charSet: the character set to use for string generation
//   - seed: ignored (kept for backward compatibility)
//
// Returns a new Source instance.
func NewSource(charSet string, seed int64) *Source {
	return &Source{
		chars: []rune(charSet),
	}
}

// Intn returns a cryptographically secure random integer in [0, n).
//
// Parameters:
//   - n: the upper bound (exclusive)
//
// Returns a random integer in [0, n).
func (s *Source) Intn(n int) int {
	if n <= 0 {
		panic("invalid argument to Intn")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Use crypto/rand to generate random bytes
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand.Read should never fail
	}

	// Convert bytes to uint64
	val := binary.BigEndian.Uint64(b[:])

	// Modulo ensures result is in range [0, n)
	// Since we validated n > 0 and result < n, conversion to int is safe
	// #nosec G115 -- modulo result is always less than n, which fits in int
	result := int(val % uint64(n))
	return result
}

// RandomInt returns a random integer in the inclusive range [min, max].
//
// Uses thread-safe random number generation to prevent race conditions.
//
// Parameters:
//   - min: the minimum value (inclusive)
//   - max: the maximum value (inclusive)
//
// Returns a random integer between min and max, inclusive.
func (s *Source) RandomInt(min, max int) int {
	return s.Intn(max-min+1) + min
}

// RandString generates a cryptographically secure random string of the specified length.
//
// The string is composed of characters randomly selected from the configured
// character set. Each character has an equal probability of being selected.
//
// Uses crypto/rand for cryptographically secure random generation.
// Thread-safe for concurrent use.
//
// Parameters:
//   - length: the desired length of the generated string
//
// Returns a random string of the specified length.
func (s *Source) RandString(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = s.chars[s.Intn(len(s.chars))]
	}
	return string(b)
}
