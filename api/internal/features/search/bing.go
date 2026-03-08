package search

import (
	"encoding/base64"
	"fmt"
	"html"
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

// bingCKRe captures Bing redirect URLs from organic result headings.
//
// Bing wraps every organic result link in an internal redirect:
//
//	<h2><a href="https://www.bing.com/ck/a?...&u=a1<BASE64>&ntb=1">Title</a></h2>
//
// The real target URL is base64-encoded in the "u" query parameter (after the "a1" prefix).
var bingCKRe = regexp.MustCompile(`<h2[^>]*>\s*<a\b[^>]*\bhref="(https://(?:www\.)?bing\.com/ck/a[^"]+)"`)

// bingDirectRe is a fallback for any direct https:// href inside an h2 (older Bing format).
// Bing-internal links are filtered out downstream by DirectoryDomains.
var bingDirectRe = regexp.MustCompile(`<h2[^>]*>\s*<a\b[^>]*\bhref="(https://[^"]+)"`)

// decodeBingURL extracts the real target URL from a Bing redirect URL.
// Bing encodes: https://www.bing.com/ck/a?!&&...&u=a1<BASE64_URL>&ntb=1
func decodeBingURL(bingURL string) string {
	unescaped := html.UnescapeString(bingURL)
	parsed, err := url.Parse(unescaped)
	if err != nil {
		return ""
	}
	u := parsed.Query().Get("u")
	if !strings.HasPrefix(u, "a1") {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(u[2:])
	if err != nil {
		return ""
	}
	return string(decoded)
}

// bingTransport is dedicated to Bing requests.
// Bing does not fingerprint Go's TLS handshake as aggressively as DDG/Google,
// so a plain transport with sensible timeouts works reliably.
var bingTransport = &http.Transport{
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 15 * time.Second,
	IdleConnTimeout:       30 * time.Second,
	MaxIdleConns:          10,
	MaxIdleConnsPerHost:   2,
}

// BingSearcher scrapes Bing HTML search results to find a company's own website.
//
// Why Bing instead of DuckDuckGo or public SearXNG:
//   - DDG blocks Go's net/http via TLS fingerprint (JA3)
//   - Public SearXNG instances disable format=json at any time (403)
//   - Bing HTML scraping works reliably without API keys or bot blocking
//   - Bing result links are wrapped in /ck/a redirects; real URLs decoded via base64 "u" param
type BingSearcher struct {
	cfg    Config
	client *http.Client
}

// NewBing creates a BingSearcher with the given config.
func NewBing(cfg Config) *BingSearcher {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &BingSearcher{
		cfg: cfg,
		client: &http.Client{
			Timeout:   timeout,
			Transport: bingTransport,
		},
	}
}

// Search queries Bing for a company's official website and returns up to 3
// candidate base URLs, filtering known directory and social domains.
func (b *BingSearcher) Search(company model.Company) ([]string, error) {
	cleanName := CleanQuery(company.ReName)
	cleanCity := CleanQuery(company.ReOrt)
	cleanQ := strings.TrimSpace(cleanName + " " + cleanCity)
	cleanQ = multiSpaceRe.ReplaceAllString(cleanQ, " ")

	// mkt=de-DE + cc=DE forces Bing to serve German-language results regardless
	// of the server's IP geolocation. "Impressum" is legally required on German
	// company sites and boosts the official page above directories.
	// "Germany" is intentionally omitted from the query — it causes Bing to
	// geo-route results to Chinese/Asian pages when queried from non-EU IPs.
	searchURL := "https://www.bing.com/search?q=" +
		url.QueryEscape(cleanQ+" Impressum") +
		"&mkt=de-DE&cc=DE&setlang=de&count=10"

	logging.Logger.Info("bing search",
		zap.String("raw_name", company.ReName),
		zap.String("clean_name", cleanName),
		zap.String("query", cleanQ),
		zap.String("url", searchURL),
	)

	maxAttempts := b.cfg.RetryCount + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := backoffs[min(attempt-1, len(backoffs)-1)]
			logging.Logger.Info("retrying bing search",
				zap.String("company", company.ReName),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff),
			)
			time.Sleep(backoff)
		}

		candidates, err := b.doSearch(searchURL, company.ReName)
		if err != nil {
			lastErr = err
			logging.Logger.Warn("bing attempt failed",
				zap.String("company", company.ReName),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			continue
		}
		if len(candidates) > 0 {
			logging.Logger.Info("bing candidates found",
				zap.String("company", company.ReName),
				zap.Strings("candidates", candidates),
			)
			return candidates, nil
		}
		lastErr = fmt.Errorf("no results for query: %s", cleanQ)
	}

	logging.Logger.Warn("bing search exhausted all retries",
		zap.String("company", company.ReName),
		zap.NamedError("last_error", lastErr),
	)
	return nil, lastErr
}

// doSearch performs one HTTP GET to Bing and extracts candidate URLs from the HTML.
func (b *BingSearcher) doSearch(searchURL, companyName string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	// Browser-like headers improve result quality and reduce the chance of
	// Bing serving a captcha or simplified/mobile page.
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept",
		"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Connection", "keep-alive")

	logging.Logger.Debug("bing request dispatched", zap.String("url", searchURL))

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bing request: %w", err)
	}
	defer resp.Body.Close()

	logging.Logger.Debug("bing response received",
		zap.String("company", companyName),
		zap.Int("status", resp.StatusCode),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bing status %d", resp.StatusCode)
	}

	// Bing pages are ~200-400 kB; cap at 2 MB for safety.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	htmlStr := string(body)
	var candidates []string
	seenDomains := map[string]bool{}

	addCandidate := func(rawURL string) {
		if len(candidates) >= maxCandidates {
			return
		}
		domain := domainOf(rawURL)
		if domain == "" || DirectoryDomains[domain] || seenDomains[domain] {
			return
		}
		seenDomains[domain] = true
		if base := schemeHost(rawURL); base != "" {
			candidates = append(candidates, base)
		}
	}

	// Primary: decode Bing redirect URLs (current Bing HTML format).
	for _, m := range bingCKRe.FindAllStringSubmatch(htmlStr, -1) {
		if len(candidates) >= maxCandidates {
			break
		}
		if real := decodeBingURL(m[1]); real != "" {
			addCandidate(real)
		}
	}

	// Fallback: direct href in h2 (older Bing format or A/B test variants).
	if len(candidates) == 0 {
		for _, m := range bingDirectRe.FindAllStringSubmatch(htmlStr, -1) {
			if len(candidates) >= maxCandidates {
				break
			}
			addCandidate(m[1])
		}
	}

	logging.Logger.Debug("bing html parsed",
		zap.String("company", companyName),
		zap.Int("body_bytes", len(body)),
		zap.Int("candidates", len(candidates)),
	)
	return candidates, nil
}
