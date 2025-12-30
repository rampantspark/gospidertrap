package content

import (
	"strings"
	"testing"

	"github.com/rampantspark/gospidertrap/internal/random"
)

func TestNewGenerator(t *testing.T) {
	webpages := []string{"page1", "page2"}
	template := "<html><body>test</body></html>"
	endpoint := "/submit"
	randomSrc := random.NewSource("abc", 42)

	gen := NewGenerator(webpages, template, endpoint, randomSrc)

	if gen == nil {
		t.Fatal("NewGenerator returned nil")
	}
	if len(gen.webpages) != len(webpages) {
		t.Errorf("webpages length = %d, want %d", len(gen.webpages), len(webpages))
	}
	if gen.htmlTemplate != template {
		t.Errorf("htmlTemplate = %q, want %q", gen.htmlTemplate, template)
	}
	if gen.endpoint != endpoint {
		t.Errorf("endpoint = %q, want %q", gen.endpoint, endpoint)
	}
}

func TestGeneratePage_WithTemplate(t *testing.T) {
	template := `<html><body><a href="/old">link</a></body></html>`
	randomSrc := random.NewSource("abc", 42)
	gen := NewGenerator(nil, template, "", randomSrc)

	page := gen.GeneratePage()

	if !strings.Contains(page, "<html>") {
		t.Error("Generated page missing <html> tag")
	}
	if !strings.Contains(page, "<body>") {
		t.Error("Generated page missing <body> tag")
	}
	if strings.Contains(page, "/old") {
		t.Error("Generated page still contains old link")
	}
}

func TestGeneratePage_WithoutTemplate(t *testing.T) {
	webpages := []string{"page1", "page2", "page3"}
	randomSrc := random.NewSource("abc", 42)
	gen := NewGenerator(webpages, "", "", randomSrc)

	page := gen.GeneratePage()

	if !strings.Contains(page, "<html>") {
		t.Error("Generated page missing <html> tag")
	}
	if !strings.Contains(page, "<body>") {
		t.Error("Generated page missing <body> tag")
	}
	// Should contain at least one link
	if !strings.Contains(page, "<a href=") {
		t.Error("Generated page missing links")
	}
}

func TestGenerateNewPage(t *testing.T) {
	webpages := []string{"page1", "page2", "page3"}
	randomSrc := random.NewSource("abc", 42)
	gen := NewGenerator(webpages, "", "", randomSrc)

	page := gen.GenerateNewPage()

	if !strings.Contains(page, "<html>") {
		t.Error("Generated page missing <html> tag")
	}
	if !strings.Contains(page, "<body>") {
		t.Error("Generated page missing <body> tag")
	}
	// Count number of links (should have multiple)
	linkCount := strings.Count(page, "<a href=")
	if linkCount < 5 {
		t.Errorf("Generated page has %d links, want at least 5", linkCount)
	}
}

func TestGenerateRandomLink_WithWebpages(t *testing.T) {
	webpages := []string{"page1", "page2", "page3"}
	randomSrc := random.NewSource("abc", 42)
	gen := NewGenerator(webpages, "", "", randomSrc)

	for i := 0; i < 10; i++ {
		link := gen.GenerateRandomLink()
		// Should be one of the webpages (no "/" prefix in stored webpages)
		found := false
		for _, page := range webpages {
			if link == page {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GenerateRandomLink() = %q, not in webpages list", link)
		}
	}
}

func TestGenerateRandomLink_WithoutWebpages(t *testing.T) {
	randomSrc := random.NewSource("abcdefghijklmnopqrstuvwxyz", 42)
	gen := NewGenerator(nil, "", "", randomSrc)

	for i := 0; i < 10; i++ {
		link := gen.GenerateRandomLink()
		// Random links don't have "/" prefix - they're just random strings
		if len(link) < 1 {
			t.Error("GenerateRandomLink() returned empty link")
		}
		// All characters should be from the charset
		for _, char := range link {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz", char) {
				t.Errorf("GenerateRandomLink() = %q, contains invalid character %c", link, char)
				break
			}
		}
	}
}

func TestReplaceLinksInHTML(t *testing.T) {
	randomSrc := random.NewSource("abc", 42)
	gen := NewGenerator(nil, "", "", randomSrc)

	tests := []struct {
		name     string
		input    string
		wantLink bool
	}{
		{
			name:     "simple link",
			input:    `<a href="/old">text</a>`,
			wantLink: true,
		},
		{
			name:     "multiple links",
			input:    `<a href="/old1">text1</a><a href="/old2">text2</a>`,
			wantLink: true,
		},
		{
			name:     "no links",
			input:    `<html><body>no links here</body></html>`,
			wantLink: false,
		},
		{
			name:     "link with attributes",
			input:    `<a class="btn" href="/old" id="link">text</a>`,
			wantLink: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gen.ReplaceLinksInHTML(tt.input)

			if tt.wantLink {
				// Should not contain old links
				if strings.Contains(result, "/old") {
					t.Error("Result still contains old link")
				}
				// Should contain new links
				if !strings.Contains(result, `href="`) {
					t.Error("Result missing href attribute")
				}
			} else {
				// Should be unchanged
				if result != tt.input {
					t.Errorf("Result changed when no links present")
				}
			}
		})
	}
}

func TestWriteLink(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"simple", "/page"},
		{"with query params", "/page?id=1"},
		{"special chars", "/page?test=<>&"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			WriteLink(&sb, tt.address)
			result := sb.String()

			if !strings.Contains(result, `href="`) {
				t.Errorf("WriteLink missing href attribute: %q", result)
			}
			if !strings.Contains(result, "<a ") {
				t.Error("WriteLink should contain <a tag")
			}
			if !strings.Contains(result, "</a>") {
				t.Error("WriteLink should contain </a> closing tag")
			}
			// Should escape special characters
			if tt.address == "/page?test=<>&" && strings.Contains(result, "<>&") {
				t.Error("WriteLink should escape special characters")
			}
		})
	}
}

func TestWriteForm(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"simple endpoint", "/submit"},
		{"endpoint with query", "/submit?id=1"},
		{"endpoint with special chars", "/submit?test=<>&"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			WriteForm(&sb, tt.endpoint)
			result := sb.String()

			if !strings.Contains(result, `action="`) {
				t.Error("Form missing action attribute")
			}
			if !strings.Contains(result, `method="get"`) {
				t.Error("Form missing GET method")
			}
			if !strings.Contains(result, "<form") {
				t.Error("Form missing form tag")
			}
			if !strings.Contains(result, "</form>") {
				t.Error("Form missing closing form tag")
			}
			if !strings.Contains(result, `<input`) {
				t.Error("Form missing input field")
			}
			if !strings.Contains(result, `<button`) {
				t.Error("Form missing submit button")
			}
			// Should escape special characters
			if tt.endpoint == "/submit?test=<>&" && strings.Contains(result, "<>&") {
				t.Error("WriteForm should escape special characters")
			}
		})
	}
}

func TestMultipleGenerations(t *testing.T) {
	// Test that crypto/rand produces valid but different pages each time
	// (Not deterministic like math/rand with seed)
	webpages := []string{"page1", "page2"}
	template := `<a href="/old">link</a>`

	src := random.NewSource("abc", 0) // Seed ignored with crypto/rand
	gen := NewGenerator(webpages, template, "", src)

	// Generate multiple pages
	pages := make(map[string]bool)
	for i := 0; i < 10; i++ {
		page := gen.GeneratePage()

		// Basic validation
		if page == "" {
			t.Error("GeneratePage returned empty string")
		}
		if !strings.Contains(page, "href=") {
			t.Error("GeneratePage missing href attribute")
		}

		pages[page] = true
	}

	// With crypto/rand, we should get some variety (not all identical)
	// Though it's possible (unlikely) all could be the same with small webpage list
	if len(pages) < 2 {
		t.Logf("Warning: Got %d unique pages out of 10 generations (expected more variety with crypto/rand)", len(pages))
	}
}

func BenchmarkGeneratePage(b *testing.B) {
	webpages := []string{"page1", "page2", "page3"}
	randomSrc := random.NewSource("abc", 42)
	gen := NewGenerator(webpages, "", "", randomSrc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gen.GeneratePage()
	}
}

func BenchmarkGenerateNewPage(b *testing.B) {
	webpages := []string{"page1", "page2", "page3"}
	randomSrc := random.NewSource("abc", 42)
	gen := NewGenerator(webpages, "", "", randomSrc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gen.GenerateNewPage()
	}
}

func BenchmarkReplaceLinksInHTML(b *testing.B) {
	template := `<html><body><a href="/old1">link1</a><a href="/old2">link2</a><a href="/old3">link3</a></body></html>`
	randomSrc := random.NewSource("abc", 42)
	gen := NewGenerator(nil, "", "", randomSrc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gen.ReplaceLinksInHTML(template)
	}
}
