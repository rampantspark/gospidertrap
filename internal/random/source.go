// Package random provides thread-safe random number generation.
package random

import (
	"math/rand"
	"sync"
)

// Source provides thread-safe random number generation with a specific character set.
type Source struct {
	rand  *rand.Rand
	mu    sync.Mutex
	chars []rune
}

// NewSource creates a new random source with the given character set and seed.
//
// Parameters:
//   - charSet: the character set to use for string generation
//   - seed: the seed for the random number generator
//
// Returns a new Source instance.
func NewSource(charSet string, seed int64) *Source {
	return &Source{
		rand:  rand.New(rand.NewSource(seed)),
		chars: []rune(charSet),
	}
}

// Intn returns a random integer in [0, n) using thread-safe access to the
// random number generator.
//
// Parameters:
//   - n: the upper bound (exclusive)
//
// Returns a random integer in [0, n).
func (s *Source) Intn(n int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rand.Intn(n)
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

// RandString generates a random string of the specified length.
//
// The string is composed of characters randomly selected from the configured
// character set. Each character has an equal probability of being selected.
//
// NOTE: This uses math/rand and is NOT cryptographically secure.
// For security-sensitive random generation, use crypto/rand directly.
//
// Uses thread-safe random number generation to prevent race conditions.
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
