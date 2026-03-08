// Package search provides DuckDuckGo HTML scraping for company website discovery.
package search

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"go.uber.org/zap"

	"webscraper/internal/features/model"
)

const (
	// searchEndpoint is the plain-HTML DuckDuckGo interface.
	// duckduckgo.com/html/ is preferred over html.duckduckgo.com because it uses
	// the main domain and is less aggressively blocked for automated requests.
	searchEndpoint = "https://duckduckgo.com/html/"
	maxCandidates  = 3
	// ddgMinDelay is the minimum time we sleep after acquiring ddgSem before
	// making the HTTP request. This acts as a lightweight rate limiter and
	// replaces the Colly LimitRule (which added an unpredictable 3-5 s overhead
	// that consumed most of the request timeout budget).
	ddgMinDelay = 2 * time.Second
)

// ddgSem serialises all DuckDuckGo requests process-wide (capacity 1 = one at a time).
// This is intentional: DDG aggressively rate-limits concurrent scrapers.
var ddgSem = make(chan struct{}, 1)

// multiSpaceRe collapses consecutive whitespace to a single space.
var multiSpaceRe = regexp.MustCompile(`\s+`)

// germanPLZRe matches 5-digit German postal codes that appear as standalone tokens.
var germanPLZRe = regexp.MustCompile(`\b\d{5}\b`)

// uddgValueRe extracts percent-encoded URLs from DuckDuckGo redirect links.
// DDG HTML encodes result URLs as the "uddg" query-string parameter:
//
//	href="/l/?uddg=https%3A%2F%2Fexample.de%2F&amp;rut=…"
//
// The character class [^&"'\s<>] stops at the first HTML attribute delimiter
// or entity boundary so we never capture trailing noise.
var uddgValueRe = regexp.MustCompile(`[?&]uddg=([^&"'\s<>]+)`)

// ddgTransport is a dedicated HTTP transport for DuckDuckGo searches.
// ResponseHeaderTimeout is set to 30 s so that a silent connection-drop by
// DDG (bot detection) fails fast rather than blocking a worker for 60 s.
var ddgTransport = &http.Transport{
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	IdleConnTimeout:       60 * time.Second,
	MaxIdleConns:          5,
	MaxIdleConnsPerHost:   1,
	DisableKeepAlives:     false,
}

// Config holds tuneable parameters for DuckDuckGoSearcher.
type Config struct {
	Timeout      time.Duration
	RetryCount   int
	RequestDelay time.Duration
	RandomDelay  time.Duration
}

// DirectoryDomains is the set of business-directory and social-network domains
// that must be excluded when picking a company's own website from DDG results.
var DirectoryDomains = map[string]bool{
	"gelbeseiten.de":            true,
	"dasoertliche.de":           true,
	"yelp.de":                   true,
	"yelp.com":                  true,
	"11880.com":                 true,
	"meinestadt.de":             true,
	"cylex.de":                  true,
	"firma.de":                  true,
	"facebook.com":              true,
	"instagram.com":             true,
	"twitter.com":               true,
	"x.com":                     true,
	"linkedin.com":              true,
	"xing.com":                  true,
	"google.com":                true,
	"google.de":                 true,
	"wikipedia.org":             true,
	"wikidata.org":              true,
	"duckduckgo.com":            true,
	"bing.com":                  true,
	"northdata.de":              true,
	"handelsregister.de":        true,
	"unternehmensregister.de":   true,
	"bundesanzeiger.de":         true,
	"northdata.com":             true,
	"zhihu.com":                 true,
	"booking.com":               true,
	"trustpilot.com":            true,
	"branchenbuch.de":           true,
	"wlw.de":                    true,
	"kompass.com":               true,
	"klicktel.de":               true,
	"telefonbuch.de":            true,
	"stadtbranchenbuch.com":     true,
	"herold.at":                 true,
	"europages.de":              true,
	"tripadvisor.de":            true,
	"tripadvisor.com":           true,
	"foursquare.com":            true,
	"golocal.de":                true,
	"hotfrog.de":                true,
	"qype.com":                  true,
	"marktplatz-mittelstand.de": true,
	"local.ch":                  true,
	"yellowpages.com":           true,
}

// DuckDuckGoSearcher scrapes DuckDuckGo HTML search results to find a company's
// own website. It is safe to call from multiple goroutines; the package-level
// ddgSem ensures only one HTTP request reaches DDG at a time.
type DuckDuckGoSearcher struct {
	cfg Config
}

// New creates a DuckDuckGoSearcher with the given config.
func New(cfg Config) *DuckDuckGoSearcher {
	return &DuckDuckGoSearcher{cfg: cfg}
}

// CleanQuery removes quotes, punctuation noise, German postal codes, and
// address-style separators, then normalises whitespace. It is intentionally
// conservative so that legitimate hyphens inside company names ("Müller-Schmidt")
// are preserved while bare address fragments like "Haus 75, 76, 79, 23769 Kiel"
// are stripped out.
//
// Examples:
//
//	`Camping An der Waterkant "" Westerdeichstrich` → `Camping An der Waterkant Westerdeichstrich`
//	`88 ETW Staberdorf - Haus 75, 76, 79, 23769 Fehmarn` → `88 ETW Staberdorf`
//	`ADIMO Immobilien-` → `ADIMO Immobilien`
func CleanQuery(s string) string {
	// 1. Replace quote/bracket characters with a space.
	s = strings.NewReplacer(
		`"`, " ",
		`'`, " ",
		"`", " ",
		`„`, " ", // german opening quote
		`"`, " ", // left double quotation
		`"`, " ", // right double quotation
		`«`, " ",
		`»`, " ",
		`(`, " ",
		`)`, " ",
		`,`, " ", // commas — e.g. "Haus 75, 76, 79"
		`;`, " ",
	).Replace(s)

	// 2. Spaced-dash separator " - " signals an address or unit suffix.
	//    Truncate at the first occurrence and discard everything after it.
	if idx := strings.Index(s, " - "); idx > 0 {
		s = s[:idx]
	}

	// 3. Remove 5-digit German postal codes ("23769", "10115", …).
	s = germanPLZRe.ReplaceAllString(s, " ")

	// 4. Collapse whitespace.
	s = multiSpaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	// 5. Strip dangling trailing punctuation that corrupts the search URL
	//    (e.g. "ADIMO Immobilien-" from a truncated Excel cell).
	s = strings.TrimRight(s, "-/&+.,;:")
	return strings.TrimSpace(s)
}

// BuildSearchURL constructs the final DuckDuckGo HTML search URL for a company.
func BuildSearchURL(companyName, city string) string {
	clean := CleanQuery(companyName + " " + city + " Germany")
	return searchEndpoint + "?q=" + url.QueryEscape(clean)
}

// Search queries DuckDuckGo for a company website and returns up to 3 candidate
// base URLs (e.g. "https://woge-kiel.de"). It retries on failure with the
// back-off sequence: 1 s → 2 s → 4 s.
func (d *DuckDuckGoSearcher) Search(company model.Company) ([]string, error) {
	// Clean name and city independently before joining so that truncation on
	// " - " inside ReName does not accidentally eat the city token.
	cleanName := CleanQuery(company.ReName)
	cleanCity := CleanQuery(company.ReOrt)
	cleanQ := strings.TrimSpace(cleanName + " " + cleanCity + " Germany")
	// Collapse any leftover double spaces after the join.
	cleanQ = multiSpaceRe.ReplaceAllString(cleanQ, " ")

	encoded := url.QueryEscape(cleanQ)
	searchURL := searchEndpoint + "?q=" + encoded

	logging.Logger.Info("duckduckgo search",
		zap.String("raw_name", company.ReName),
		zap.String("clean_name", cleanName),
		zap.String("query", cleanQ),
		zap.String("encoded_query", encoded),
		zap.String("url", searchURL),
	)

	maxAttempts := d.cfg.RetryCount + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := backoffs[min(attempt-1, len(backoffs)-1)]
			logging.Logger.Info("retrying duckduckgo search",
				zap.String("company", company.ReName),
				zap.Int("attempt", attempt+1),
				zap.Int("max_attempts", maxAttempts),
				zap.Duration("backoff", backoff),
			)
			time.Sleep(backoff)
		}

		candidates, err := d.doSearch(searchURL)
		if err != nil {
			lastErr = err
			logging.Logger.Warn("duckduckgo attempt failed",
				zap.String("company", company.ReName),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			continue
		}
		if len(candidates) > 0 {
			logging.Logger.Info("duckduckgo candidates found",
				zap.String("company", company.ReName),
				zap.Strings("candidates", candidates),
			)
			return candidates, nil
		}
		lastErr = fmt.Errorf("no results for query: %s", cleanQ)
	}

	logging.Logger.Warn("duckduckgo search exhausted all retries",
		zap.String("company", company.ReName),
		zap.String("query", cleanQ),
		zap.NamedError("last_error", lastErr),
	)
	return nil, lastErr
}

// doSearch performs one HTTP GET to DuckDuckGo HTML behind the global semaphore.
//
// Colly is intentionally NOT used here: the Colly LimitRule delay (3 s) ran
// inside the request timeout window and caused workers to time out before DDG
// could ever respond. A plain net/http client gives us deterministic control
// over timing and lets us sleep for ddgMinDelay BEFORE starting the clock.
func (d *DuckDuckGoSearcher) doSearch(searchURL string) ([]string, error) {
	// Serialise all DDG requests — acquire before sleeping so that concurrent
	// workers queue up rather than all sleeping simultaneously.
	ddgSem <- struct{}{}
	defer func() { <-ddgSem }()

	// Polite delay before the request.  This happens while holding the semaphore
	// so successive workers naturally space out without additional coordination.
	time.Sleep(ddgMinDelay)

	req, err := http.NewRequest(http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	// Browser-like headers reduce the chance of being silently dropped by DDG.
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept",
		"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,de;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")

	logging.Logger.Debug("duckduckgo request dispatched", zap.String("url", searchURL))

	// Per-request HTTP client — timeout covers DNS + TCP + TLS + headers + body.
	// 30 s is intentionally shorter than the Colly-era 60 s: if DDG drops the
	// connection (bot detection) we want to fail fast and move on.
	timeout := d.cfg.Timeout
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: ddgTransport,
	}

	resp, err := client.Do(req)
	if err != nil {
		logging.Logger.Warn("duckduckgo request error",
			zap.String("url", searchURL),
			zap.Error(err),
		)
		return nil, fmt.Errorf("duckduckgo visit: %w", err)
	}
	defer resp.Body.Close()

	logging.Logger.Debug("duckduckgo response received",
		zap.String("url", searchURL),
		zap.Int("status", resp.StatusCode),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("duckduckgo status %d", resp.StatusCode)
	}

	// Cap body read at 1 MB — DDG HTML pages are typically < 100 kB.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	candidates := extractCandidates(string(body))
	logging.Logger.Debug("duckduckgo html parsed",
		zap.String("url", searchURL),
		zap.Int("body_bytes", len(body)),
		zap.Int("candidates", len(candidates)),
	)
	return candidates, nil
}

// extractCandidates parses a DDG HTML response body and returns up to
// maxCandidates company-website base URLs, skipping known directory domains.
func extractCandidates(body string) []string {
	var candidates []string
	seenDomains := map[string]bool{}

	for _, m := range uddgValueRe.FindAllStringSubmatch(body, -1) {
		if len(candidates) >= maxCandidates {
			break
		}
		// m[1] is the percent-encoded actual URL from the uddg parameter.
		decoded, err := url.QueryUnescape(m[1])
		if err != nil {
			continue
		}
		domain := domainOf(decoded)
		if domain == "" || DirectoryDomains[domain] || seenDomains[domain] {
			continue
		}
		seenDomains[domain] = true
		if base := schemeHost(decoded); base != "" {
			candidates = append(candidates, base)
		}
	}
	return candidates
}

// domainOf extracts the bare host (without www.) from a raw URL.
func domainOf(rawURL string) string {
	p, err := url.Parse(rawURL)
	if err != nil || p.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(p.Host), "www.")
}

// schemeHost returns "scheme://host" from a raw URL.
func schemeHost(rawURL string) string {
	p, err := url.Parse(rawURL)
	if err != nil || p.Host == "" {
		return ""
	}
	return p.Scheme + "://" + p.Host
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
