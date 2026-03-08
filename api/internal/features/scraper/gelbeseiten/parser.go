// Package gelbeseiten implements scraping of company contact data from
// www.gelbeseiten.de.
package gelbeseiten

import (
	"net/url"
	"regexp"
	"strings"
)

var emailRe = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,24}`)

// ParsePhone normalises a tel: href value into a compact digit string (with
// optional leading +).  Returns empty string if the result has fewer than 7
// digits.
//
// Example: "tel:+4946515903" → "+4946515903"
func ParsePhone(telHref string) string {
	raw := strings.TrimPrefix(strings.TrimSpace(telHref), "tel:")
	var b strings.Builder
	for i, r := range raw {
		switch {
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	result := b.String()
	digits := strings.TrimLeft(result, "+")
	if len(digits) < 7 {
		return ""
	}
	return result
}

// ParseWebsite returns a cleaned website URL or empty string.
// Rejects javascript: URIs, anchors, and blank values.
func ParseWebsite(href string) string {
	href = strings.TrimSpace(href)
	if href == "" ||
		strings.HasPrefix(href, "#") ||
		strings.HasPrefix(strings.ToLower(href), "javascript") {
		return ""
	}
	return href
}

// ParseEmail extracts and validates an email from a GelbeSeiten data-link
// attribute of the form "mailto:addr@example.de?subject=...".
//
// Example: "mailto:bergedorfer-tor@stolle-ot.de?subject=Anfrage" → "bergedorfer-tor@stolle-ot.de"
func ParseEmail(dataLink string) string {
	dl := strings.TrimSpace(dataLink)
	if !strings.HasPrefix(strings.ToLower(dl), "mailto:") {
		return ""
	}
	email := dl[len("mailto:"):]
	if i := strings.IndexByte(email, '?'); i >= 0 {
		email = email[:i]
	}
	if decoded, err := url.QueryUnescape(email); err == nil {
		email = decoded
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if !emailRe.MatchString(email) {
		return ""
	}
	return email
}
