package content

import (
	"html"
	"regexp"
	"strings"

	"github.com/rampantspark/gospidertrap/internal/random"
)

// Pre-compiled regular expressions for link replacement (cached for performance)
var (
	// hrefRegex matches <a> tags with href attributes
	hrefRegex = regexp.MustCompile(`<a\s+[^>]*href=["']([^"']*)["'][^>]*>`)
	// hrefReplacer matches just the href attribute value
	hrefReplacer = regexp.MustCompile(`href=["'][^"']*["']`)
)

// Generator handles HTML page generation with random links.
type Generator struct {
	webpages     []string       // Wordlist entries to use for link generation
	htmlTemplate string         // HTML template file content (optional)
	endpoint     string         // Form submission endpoint (optional)
	random       *random.Source // Random number generator
}

// NewGenerator creates a new content generator.
//
// Parameters:
//   - webpages: wordlist entries for link generation (can be nil/empty for random strings)
//   - htmlTemplate: optional HTML template with links to replace
//   - endpoint: optional form submission endpoint
//   - random: random number generator source
//
// Returns a new Generator instance.
func NewGenerator(webpages []string, htmlTemplate, endpoint string, random *random.Source) *Generator {
	return &Generator{
		webpages:     webpages,
		htmlTemplate: htmlTemplate,
		endpoint:     endpoint,
		random:       random,
	}
}

// GeneratePage generates an HTML page with random links.
//
// If an HTML template is configured, it replaces all href attributes in <a> tags
// with random links while preserving the rest of the template structure.
// Otherwise, it generates a new HTML page from scratch with random links
// and optionally a form if an endpoint is configured.
//
// Returns the generated HTML as a string.
func (g *Generator) GeneratePage() string {
	if g.htmlTemplate != "" {
		return g.ReplaceLinksInHTML(g.htmlTemplate)
	}
	return g.GenerateNewPage()
}

// GenerateNewPage creates a new HTML page from scratch with random links.
//
// The number of links is randomly determined between LinksPerPageMin and
// LinksPerPageMax. Links are either selected from the wordlist (if available)
// or generated as random strings. If an endpoint is configured, a form is
// also included in the generated page.
//
// Returns the complete HTML page as a string.
func (g *Generator) GenerateNewPage() string {
	var sb strings.Builder
	sb.WriteString("<html>\n<body>\n")

	// Generate random links
	numLinks := g.random.RandomInt(LinksPerPageMin, LinksPerPageMax)
	for i := 0; i < numLinks; i++ {
		link := g.GenerateRandomLink()
		WriteLink(&sb, link)
	}

	// Add form if endpoint is configured
	if g.endpoint != "" {
		WriteForm(&sb, g.endpoint)
	}

	sb.WriteString("</body>\n</html>")
	return sb.String()
}

// GenerateRandomLink generates a random link address.
//
// If a wordlist is available, it selects a random entry from the wordlist.
// Otherwise, it generates a random string using the configured character set
// with a length between LengthOfLinksMin and LengthOfLinksMax.
//
// Uses thread-safe random number generation to prevent race conditions.
//
// Returns the generated link address as a string.
func (g *Generator) GenerateRandomLink() string {
	if len(g.webpages) > 0 {
		idx := g.random.Intn(len(g.webpages))
		return g.webpages[idx]
	}
	length := g.random.RandomInt(LengthOfLinksMin, LengthOfLinksMax)
	return g.random.RandString(length)
}

// ReplaceLinksInHTML replaces all href attributes in <a> tags with random links.
//
// It uses pre-compiled regular expressions to find all anchor tags with href attributes
// (supporting both single and double quotes) and replaces the href value
// with a randomly generated link while preserving all other attributes
// and the tag structure. The replacement links are HTML-escaped.
//
// Parameters:
//   - template: the HTML template string to process
//
// Returns the modified HTML string with replaced links.
func (g *Generator) ReplaceLinksInHTML(template string) string {
	return hrefRegex.ReplaceAllStringFunc(template, func(match string) string {
		randomLink := g.GenerateRandomLink()
		escapedLink := html.EscapeString(randomLink)
		return hrefReplacer.ReplaceAllString(match, `href="`+escapedLink+`"`)
	})
}
