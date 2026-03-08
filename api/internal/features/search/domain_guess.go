package search

import (
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/ahmet4dev/gol-lib/logging"
	"go.uber.org/zap"
	"golang.org/x/text/unicode/norm"

	"webscraper/internal/features/model"
)

// legalSuffixRe strips common German legal form suffixes from company names.
var legalSuffixRe = regexp.MustCompile(
	`(?i)\b(GmbH|AG|KG|OHG|GbR|e\.V\.|eV|UG|SE|KGaA|mbH|Gesellschaft|Immobilien|Verwaltung|Betrieb)\b`,
)

// nonAlnumRe replaces non-alphanumeric characters with hyphens for domain slugging.
var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

// DomainGuesser generates likely .de domain candidates from a company name
// and verifies each with a lightweight HTTP HEAD request.
// It is used as a fallback when search engines are unavailable or blocked.
type DomainGuesser struct {
	client *http.Client
}

// NewDomainGuesser creates a DomainGuesser with sensible timeout defaults.
func NewDomainGuesser() *DomainGuesser {
	return &DomainGuesser{
		client: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 6 * time.Second,
			},
			// Do not follow redirects — a 301/302 still confirms the domain exists.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Search generates domain candidates from the company name and returns
// those that respond to HTTP (any 2xx or 3xx code).
func (d *DomainGuesser) Search(company model.Company) ([]string, error) {
	candidates := d.generateCandidates(company.ReName)

	logging.Logger.Debug("domain guess candidates",
		zap.String("company", company.ReName),
		zap.Strings("candidates", candidates),
	)

	var found []string
	for _, c := range candidates {
		if len(found) >= maxCandidates {
			break
		}
		if d.isReachable(c) {
			found = append(found, c)
			logging.Logger.Info("domain guess hit",
				zap.String("company", company.ReName),
				zap.String("url", c),
			)
		}
	}
	return found, nil
}

// generateCandidates builds a prioritised list of domain guesses.
//
// Priority order (highest first):
//  1. First word alone — catches brand names like "efomed.de", "alves.de"
//  2. First two words hyphenated — "alves-baumschulen.de"
//  3. First two words compact — "alvesbaumschulen.de"
//  4. Full slug (without legal suffixes) — "efomed-elektronik.de"
func (d *DomainGuesser) generateCandidates(name string) []string {
	clean := legalSuffixRe.ReplaceAllString(name, " ")
	clean = multiSpaceRe.ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)
	slug := toSlug(clean)
	if slug == "" {
		return nil
	}

	var candidates []string
	seen := map[string]bool{}

	add := func(s string) {
		if s != "" && !seen[s] && len(s) > len("https://xx.de") {
			seen[s] = true
			candidates = append(candidates, s)
		}
	}

	parts := strings.Split(slug, "-")

	// 1. First word: "efomed.de" — only for sufficiently distinctive tokens.
	//    Short generic words like "haus", "frank", "gut" are skipped to avoid
	//    hitting unrelated websites that happen to own that short domain.
	if len(parts[0]) >= 6 {
		add("https://" + parts[0] + ".de")
	}

	// 2. First two words hyphenated: "alves-baumschulen.de"
	if len(parts) >= 2 {
		add("https://" + parts[0] + "-" + parts[1] + ".de")
		// 3. First two words compact: "alvesbaumschulen.de"
		add("https://" + parts[0] + parts[1] + ".de")
	}

	// 4. Full slug (up to 4 words to keep domains reasonable)
	if len(parts) > 2 {
		truncated := strings.Join(parts[:min4(len(parts), 4)], "-")
		add("https://" + truncated + ".de")
	} else {
		add("https://" + slug + ".de")
	}

	return candidates
}

func min4(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// toSlug converts a company name to a URL-safe lowercase ASCII slug.
// German umlauts are transliterated: ä→ae, ö→oe, ü→ue, ß→ss.
func toSlug(s string) string {
	s = strings.ToLower(s)
	// German umlaut transliteration
	s = strings.NewReplacer(
		"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
		"Ä", "ae", "Ö", "oe", "Ü", "ue",
	).Replace(s)
	// Normalize any remaining non-ASCII to their ASCII base
	s = removeNonASCII(s)
	s = nonAlnumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// removeNonASCII strips or normalises non-ASCII characters.
func removeNonASCII(s string) string {
	// NFD decomposition drops diacritical marks from base letters
	normalized := norm.NFD.String(s)
	var b strings.Builder
	for _, r := range normalized {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == ' ' {
			if r <= 127 {
				b.WriteRune(r)
			}
		} else if r == '-' || r == ' ' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isReachable confirms a domain has an active web server.
// It tries HTTPS first, then falls back to HTTP.
// Any response (including 4xx) means a web server is running at that domain —
// the impressum or contact page may still be reachable at a specific path.
func (d *DomainGuesser) isReachable(rawURL string) bool {
	if d.tryURL(rawURL) {
		return true
	}
	// HTTPS failed — try HTTP
	if strings.HasPrefix(rawURL, "https://") {
		httpURL := "http://" + rawURL[len("https://"):]
		return d.tryURL(httpURL)
	}
	return false
}

func (d *DomainGuesser) tryURL(rawURL string) bool {
	resp, err := d.client.Get(rawURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	// Any response (2xx, 3xx, 4xx) means a server is there; reject only on dial errors.
	return true
}
