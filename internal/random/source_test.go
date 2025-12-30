package random

import (
	"sync"
	"testing"
)

func TestNewSource(t *testing.T) {
	charSet := "abc123"
	seed := int64(12345) // Seed is ignored in crypto/rand implementation

	src := NewSource(charSet, seed)

	if src == nil {
		t.Fatal("NewSource returned nil")
	}
	if len(src.chars) != len(charSet) {
		t.Errorf("chars length = %d, want %d", len(src.chars), len(charSet))
	}
}

func TestIntn(t *testing.T) {
	src := NewSource("abc", 42)

	tests := []struct {
		name string
		n    int
		want func(int) bool // validation function
	}{
		{
			name: "small number",
			n:    10,
			want: func(result int) bool { return result >= 0 && result < 10 },
		},
		{
			name: "large number",
			n:    1000,
			want: func(result int) bool { return result >= 0 && result < 1000 },
		},
		{
			name: "one",
			n:    1,
			want: func(result int) bool { return result == 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := src.Intn(tt.n)
			if !tt.want(got) {
				t.Errorf("Intn(%d) = %d, validation failed", tt.n, got)
			}
		})
	}
}

func TestRandomInt(t *testing.T) {
	src := NewSource("abc", 42)

	tests := []struct {
		name string
		min  int
		max  int
	}{
		{"positive range", 1, 10},
		{"zero min", 0, 5},
		{"same values", 5, 5},
		{"large range", 1, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := src.RandomInt(tt.min, tt.max)
			if got < tt.min || got > tt.max {
				t.Errorf("RandomInt(%d, %d) = %d, want value in range [%d, %d]", tt.min, tt.max, got, tt.min, tt.max)
			}
		})
	}
}

func TestRandString(t *testing.T) {
	charSet := "abc123"
	src := NewSource(charSet, 42)

	tests := []struct {
		name   string
		length int
	}{
		{"empty string", 0},
		{"single char", 1},
		{"small string", 5},
		{"medium string", 20},
		{"large string", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := src.RandString(tt.length)
			if len(got) != tt.length {
				t.Errorf("RandString(%d) length = %d, want %d", tt.length, len(got), tt.length)
			}

			// Verify all characters are from the charset
			for i, char := range got {
				found := false
				for _, validChar := range src.chars {
					if char == validChar {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("RandString(%d) char at position %d = %c, not in charset", tt.length, i, char)
				}
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	src := NewSource("abcdefghijklmnopqrstuvwxyz", 42)
	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Test concurrent Intn calls
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = src.Intn(100)
			}
		}()
	}
	wg.Wait()

	// Test concurrent RandomInt calls
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = src.RandomInt(1, 10)
			}
		}()
	}
	wg.Wait()

	// Test concurrent RandString calls
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = src.RandString(20)
			}
		}()
	}
	wg.Wait()
}

func TestRandomnessDistribution(t *testing.T) {
	// Test that crypto/rand produces values across the range
	// (Not deterministic like math/rand with seed)
	charSet := "abc"
	src := NewSource(charSet, 0) // Seed ignored with crypto/rand

	// Generate multiple values and check distribution
	counts := make(map[int]int)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		val := src.Intn(10)
		if val < 0 || val >= 10 {
			t.Errorf("Intn(10) = %d, want value in range [0, 10)", val)
		}
		counts[val]++
	}

	// Verify we got some distribution (not all the same value)
	if len(counts) < 5 {
		t.Errorf("Distribution too narrow: got %d unique values in %d iterations", len(counts), iterations)
	}
}

func BenchmarkIntn(b *testing.B) {
	src := NewSource("abc", 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = src.Intn(100)
	}
}

func BenchmarkRandomInt(b *testing.B) {
	src := NewSource("abc", 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = src.RandomInt(1, 100)
	}
}

func BenchmarkRandString(b *testing.B) {
	src := NewSource("abcdefghijklmnopqrstuvwxyz0123456789", 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = src.RandString(20)
	}
}

func BenchmarkConcurrentIntn(b *testing.B) {
	src := NewSource("abc", 42)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = src.Intn(100)
		}
	})
}
