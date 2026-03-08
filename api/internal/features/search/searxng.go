package search

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"go.uber.org/zap"

	"webscraper/internal/features/model"
)

// DefaultSearXNGBase is the default public SearXNG instance.
// For production use, self-host SearXNG and point this to your instance.
// Docker: https://docs.searxng.org/admin/installation-docker.html
const DefaultSearXNGBase = "https://searx.be"

// searxngTransport is a lightweight transport for SearXNG requests.
// No need for the aggressive timeout settings used for DDG since SearXNG
// is specifically designed for programmatic access and responds quickly.
var searxngTransport = &http.Transport{
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 15 * time.Second,
	IdleConnTimeout:       30 * time.Second,
	MaxIdleConns:          10,
	MaxIdleConnsPerHost:   2,
}

// searxngResponse is the JSON structure returned by the SearXNG search API.
type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

type searxngResult struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// SearXNGSearcher queries a SearXNG instance via its JSON API.
//
// SearXNG is a privacy-preserving meta-search engine that aggregates results
// from Google, Bing, DuckDuckGo and others. It exposes a ?format=json endpoint
// that returns clean, structured results — no HTML parsing, no bot detection,
// no API key required.
//
// Why SearXNG instead of DuckDuckGo directly:
//   - Go's TLS fingerprint (JA3) is fingerprinted by DDG/Google and blocked
//   - SearXNG allows programmatic access by design
//   - JSON results are simpler and more reliable than HTML scraping
//   - Self-hosting removes all rate limits
type SearXNGSearcher struct {
	baseURL string
	cfg     Config
	client  *http.Client
}

// NewSearXNG creates a SearXNGSearcher pointing at the given instance URL.
// Pass an empty string to use DefaultSearXNGBase.
func NewSearXNG(baseURL string, cfg Config) *SearXNGSearcher {
	if baseURL == "" {
		baseURL = DefaultSearXNGBase
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &SearXNGSearcher{
		baseURL: strings.TrimRight(baseURL, "/"),
		cfg:     cfg,
		client: &http.Client{
			Timeout:   timeout,
			Transport: searxngTransport,
		},
	}
}

// Search queries SearXNG for a company's official website and returns up to 3
// candidate base URLs, filtering known directory/social domains.
func (s *SearXNGSearcher) Search(company model.Company) ([]string, error) {
	cleanName := CleanQuery(company.ReName)
	cleanCity := CleanQuery(company.ReOrt)
	cleanQ := strings.TrimSpace(cleanName + " " + cleanCity + " Germany")
	cleanQ = multiSpaceRe.ReplaceAllString(cleanQ, " ")

	// language=de biases results toward German-language sites.
	// categories=general avoids news/image results.
	searchURL := s.baseURL + "/search?q=" + url.QueryEscape(cleanQ) +
		"&format=json&categories=general&language=de"

	logging.Logger.Info("searxng search",
		zap.String("raw_name", company.ReName),
		zap.String("clean_name", cleanName),
		zap.String("query", cleanQ),
		zap.String("url", searchURL),
	)

	maxAttempts := s.cfg.RetryCount + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := backoffs[min(attempt-1, len(backoffs)-1)]
			logging.Logger.Info("retrying searxng search",
				zap.String("company", company.ReName),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff),
			)
			time.Sleep(backoff)
		}

		candidates, err := s.doSearch(searchURL, company.ReName)
		if err != nil {
			lastErr = err
			logging.Logger.Warn("searxng attempt failed",
				zap.String("company", company.ReName),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			continue
		}
		if len(candidates) > 0 {
			logging.Logger.Info("searxng candidates found",
				zap.String("company", company.ReName),
				zap.Strings("candidates", candidates),
			)
			return candidates, nil
		}
		lastErr = fmt.Errorf("no results for query: %s", cleanQ)
	}

	logging.Logger.Warn("searxng search exhausted all retries",
		zap.String("company", company.ReName),
		zap.NamedError("last_error", lastErr),
	)
	return nil, lastErr
}

// doSearch performs one HTTP GET to the SearXNG JSON API and parses the results.
func (s *SearXNGSearcher) doSearch(searchURL, companyName string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; webscraper/1.0)")

	logging.Logger.Debug("searxng request dispatched", zap.String("url", searchURL))

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng request: %w", err)
	}
	defer resp.Body.Close()

	logging.Logger.Debug("searxng response received",
		zap.String("company", companyName),
		zap.Int("status", resp.StatusCode),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result searxngResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	var candidates []string
	seenDomains := map[string]bool{}
	for _, r := range result.Results {
		if len(candidates) >= maxCandidates {
			break
		}
		if r.URL == "" {
			continue
		}
		domain := domainOf(r.URL)
		if domain == "" || DirectoryDomains[domain] || seenDomains[domain] {
			continue
		}
		seenDomains[domain] = true
		if base := schemeHost(r.URL); base != "" {
			candidates = append(candidates, base)
		}
	}
	return candidates, nil
}
