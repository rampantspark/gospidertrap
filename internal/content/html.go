package content

import (
	"html"
	"strings"
)

// WriteLink writes an HTML anchor tag to the string builder.
//
// The link address is HTML-escaped to prevent injection attacks.
// The generated tag format is: <a href="...">...</a><br>
//
// Parameters:
//   - sb: the string builder to write to
//   - address: the link address (will be HTML-escaped)
func WriteLink(sb *strings.Builder, address string) {
	escapedAddress := html.EscapeString(address)
	sb.WriteString("<a href=\"")
	sb.WriteString(escapedAddress)
	sb.WriteString("\">")
	sb.WriteString(escapedAddress)
	sb.WriteString("</a><br>\n")
}

// WriteForm writes an HTML form element to the string builder.
//
// The form uses the provided endpoint as its action URL and submits via GET method.
// The endpoint is HTML-escaped to prevent injection attacks.
// The form includes a text input field and a submit button.
//
// Parameters:
//   - sb: the string builder to write to
//   - endpoint: the form action URL (will be HTML-escaped)
func WriteForm(sb *strings.Builder, endpoint string) {
	escapedEndpoint := html.EscapeString(endpoint)
	sb.WriteString(`<form action="`)
	sb.WriteString(escapedEndpoint)
	sb.WriteString(`" method="get">
		<input type="text" name="param">
		<button type="submit">Submit</button>
		</form>`)
}
