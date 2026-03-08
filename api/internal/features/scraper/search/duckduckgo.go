// Package search provides DuckDuckGo HTML search for company website discovery.
package search

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/extensions"
	"go.uber.org/zap"
)

const (
	ddgEndpoint   = "https://html.duckduckgo.com/html/"
	maxResults    = 10
	requestDelay  = 2 * time.Second
)

// DDGResult holds a single search result returned by DuckDuckGo.
type DDGResult struct {
	Title string
	URL   string
}

// DDGClient scrapes DuckDuckGo HTML search results using Colly.
type DDGClient struct {
	timeout time.Duration
}

// NewDDGClient creates a DDGClient with the given request timeout.
func NewDDGClient(timeout time.Duration) *DDGClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &DDGClient{timeout: timeout}
}

// Search queries DuckDuckGo for the given query string and returns up to
// maxResults raw results (title + URL) before any domain filtering.
func (d *DDGClient) Search(query string) ([]DDGResult, error) {
	searchURL := ddgEndpoint + "?q=" + url.QueryEscape(query)

	logging.Logger.Info("ddg search",
		zap.String("query", query),
		zap.String("url", searchURL),
	)

	c := colly.NewCollector(
		colly.MaxDepth(1),
	)
	extensions.RandomUserAgent(c)
	c.SetRequestTimeout(d.timeout)

	// Browser-like headers reduce the chance of bot detection.
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "de-DE,de;q=0.9,en-US;q=0.8,en;q=0.7")
		r.Headers.Set("Referer", "https://html.duckduckgo.com/")
		r.Headers.Set("Connection", "keep-alive")
		r.Headers.Set("Cache-Control", "no-cache")
	})

	// Rate limit: one request per domain, 2 s delay.
	_ = c.Limit(&colly.LimitRule{
		DomainGlob:  "*duckduckgo.com*",
		Delay:       requestDelay,
		RandomDelay: 500 * time.Millisecond,
	})

	var results []DDGResult

	// DDG HTML result links use the class "result__a".
	c.OnHTML("a.result__a", func(e *colly.HTMLElement) {
		if len(results) >= maxResults {
			return
		}
		href := strings.TrimSpace(e.Attr("href"))
		title := strings.TrimSpace(e.Text)
		if href == "" {
			return
		}
		// DDG may encode the real URL inside a redirect — unwrap it.
		realURL := unwrapDDGRedirect(href)
		if realURL == "" {
			realURL = href
		}
		results = append(results, DDGResult{Title: title, URL: realURL})
	})

	c.OnError(func(r *colly.Response, err error) {
		logging.Logger.Warn("ddg request failed",
			zap.String("url", r.Request.URL.String()),
			zap.Int("status", r.StatusCode),
			zap.Error(err),
		)
	})

	if err := c.Visit(searchURL); err != nil {
		return nil, fmt.Errorf("ddg visit: %w", err)
	}

	logging.Logger.Info("ddg results",
		zap.String("query", query),
		zap.Int("count", len(results)),
	)
	return results, nil
}

// unwrapDDGRedirect extracts the real destination URL from a DDG redirect link.
// DDG sometimes wraps results as: /l/?uddg=https%3A%2F%2Fexample.com&...
func unwrapDDGRedirect(href string) string {
	if !strings.Contains(href, "uddg=") {
		return ""
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if v := parsed.Query().Get("uddg"); v != "" {
		decoded, err := url.QueryUnescape(v)
		if err == nil {
			return decoded
		}
	}
	return ""
}
