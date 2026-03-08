package scraper

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/extensions"
	"go.uber.org/zap"

	"webscraper/internal/cache"
	"webscraper/internal/features/model"
)

// sharedTransport is reused across all collectors so goroutines can share
// keep-alive TCP connections to the same target domains.
var sharedTransport = &http.Transport{
	MaxIdleConns:        200,
	MaxIdleConnsPerHost: 20,
	MaxConnsPerHost:     50,
	IdleConnTimeout:     60 * time.Second,
}

// ContactInfo holds extracted contact data from a single source.
type ContactInfo struct {
	Emails  []string
	Phones  []string
	Source  string
	Website string
}

// Source is the interface implemented by each scraping target.
type Source interface {
	Name() string
	Scrape(company model.Company, cfg Config) (*ContactInfo, error)
}

// Config controls the scraping behaviour.
type Config struct {
	Concurrency     int
	RequestDelay    time.Duration
	RandomDelay     time.Duration
	RetryCount      int
	RequestTimeout  time.Duration
	MatchThreshold  float64
	SearchEngineURL string // base URL of the SearXNG instance; empty = use default
	CacheFile       string // path to JSON cache file; empty = in-memory only
}

// Engine manages parallel scraping with rate limiting and retries.
type Engine struct {
	cfg     Config
	sources []Source
}

// New creates an Engine with the given config.
// GelbeSeitenSource is the sole scraping backend; the cache is shared across
// all concurrent goroutines via the injected *cache.Store.
func New(cfg Config) *Engine {
	c := cache.New(cfg.CacheFile)
	return &Engine{
		cfg: cfg,
		sources: []Source{
			&GelbeSeitenSource{cache: c},
		},
	}
}

// newCollector creates a fresh collector for each scrape call.
func newCollector(cfg Config, allowedDomains ...string) *colly.Collector {
	c := colly.NewCollector(
		colly.AllowedDomains(allowedDomains...),
		colly.MaxDepth(1),
	)
	extensions.RandomUserAgent(c)
	c.WithTransport(sharedTransport)
	c.SetRequestTimeout(cfg.RequestTimeout)
	return c
}

// dedupeKey returns a canonical key for a company so duplicates are scraped once.
func dedupeKey(c model.Company) string {
	name := strings.ToLower(strings.TrimSpace(c.ReName))
	city := strings.ToLower(strings.TrimSpace(c.ReOrt))
	return name + "|" + city
}

// RunStream scrapes all companies concurrently and sends each result to resultCh
// as soon as it completes, enabling real-time streaming to the client.
func (e *Engine) RunStream(ctx context.Context, companies []model.Company, filterMode model.FilterMode, resultCh chan<- model.ScrapeResult) {
	defer close(resultCh)

	type group struct {
		canonical model.Company
		indices   []int
	}
	groups := map[string]*group{}
	var orderedKeys []string

	for i, c := range companies {
		key := dedupeKey(c)
		if _, exists := groups[key]; !exists {
			groups[key] = &group{canonical: c}
			orderedKeys = append(orderedKeys, key)
		}
		groups[key].indices = append(groups[key].indices, i)
	}

	sem := make(chan struct{}, e.cfg.Concurrency)
	var wg sync.WaitGroup

	for _, key := range orderedKeys {
		select {
		case <-ctx.Done():
			goto done
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(g *group) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					logging.Logger.Error("panic in RunStream scrapeOne",
						zap.Any("panic", r),
						zap.String("company", g.canonical.ReName),
					)
					for _, idx := range g.indices {
						resultCh <- model.ScrapeResult{
							Company: companies[idx],
							Status:  "error",
							Error:   "internal scraper panic",
						}
					}
				}
			}()

			result := e.scrapeOne(g.canonical, filterMode)
			for _, idx := range g.indices {
				r := result
				r.Company = companies[idx]
				resultCh <- r
			}
		}(groups[key])
	}

done:
	wg.Wait()
}

// Run scrapes all companies concurrently and returns results in the same order.
func (e *Engine) Run(companies []model.Company, filterMode model.FilterMode) []model.ScrapeResult {
	results := make([]model.ScrapeResult, len(companies))

	type group struct {
		canonical model.Company
		indices   []int
	}
	groups := map[string]*group{}
	var orderedKeys []string

	for i, c := range companies {
		key := dedupeKey(c)
		if _, exists := groups[key]; !exists {
			groups[key] = &group{canonical: c}
			orderedKeys = append(orderedKeys, key)
		}
		groups[key].indices = append(groups[key].indices, i)
	}

	uniqueResults := make([]model.ScrapeResult, len(orderedKeys))
	sem := make(chan struct{}, e.cfg.Concurrency)
	var wg sync.WaitGroup

	for j, key := range orderedKeys {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, g *group) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					logging.Logger.Error("panic in scrapeOne",
						zap.Any("panic", r),
						zap.String("company", g.canonical.ReName),
					)
					uniqueResults[idx] = model.ScrapeResult{
						Company: g.canonical,
						Status:  "error",
						Error:   "internal scraper panic",
					}
				}
			}()
			uniqueResults[idx] = e.scrapeOne(g.canonical, filterMode)
		}(j, groups[key])
	}
	wg.Wait()

	for j, key := range orderedKeys {
		base := uniqueResults[j]
		for _, idx := range groups[key].indices {
			r := base
			r.Company = companies[idx]
			results[idx] = r
		}
	}

	return results
}

// shouldSkip reports whether a company should be skipped based on its existing
// data and the requested filter mode.
func shouldSkip(company model.Company, filterMode model.FilterMode) bool {
	switch filterMode {
	case model.FilterOr:
		return company.Email != "" || company.Telefonnummer != ""
	default:
		return company.Email != "" && company.Telefonnummer != ""
	}
}

// scrapeOne runs the website pipeline and returns the result.
func (e *Engine) scrapeOne(company model.Company, filterMode model.FilterMode) model.ScrapeResult {
	if shouldSkip(company, filterMode) {
		logging.Logger.Info("skipping — already complete",
			zap.String("company", company.ReName),
			zap.String("filterMode", filterMode),
		)
		return model.ScrapeResult{
			Company: company,
			Status:  "done",
			Emails:  []string{company.Email},
			Phones:  []string{company.Telefonnummer},
			Source:  "excel",
		}
	}

	for _, src := range e.sources {
		info, err := src.Scrape(company, e.cfg)
		if err != nil {
			logging.Logger.Warn("scrape error",
				zap.String("source", src.Name()),
				zap.String("company", company.ReName),
				zap.Error(err),
			)
			continue
		}
		if len(info.Emails) > 0 || len(info.Phones) > 0 {
			logging.Logger.Info("found",
				zap.String("source", info.Source),
				zap.String("company", company.ReName),
				zap.Strings("emails", info.Emails),
				zap.Strings("phones", info.Phones),
			)
			return model.ScrapeResult{
				Company: company,
				Status:  "done",
				Emails:  info.Emails,
				Phones:  info.Phones,
				Source:  info.Source,
				Website: info.Website,
			}
		}
	}

	logging.Logger.Info("not found",
		zap.String("company", company.ReName),
		zap.String("city", company.ReOrt),
	)
	return model.ScrapeResult{Company: company, Status: "not_found"}
}

// --- helpers shared across sources ---

var emailRe = regexp.MustCompile(
	`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,24}`,
)

var phoneRe = regexp.MustCompile(`(?:(?:\+49|0049|0)\s*(?:\d[\s\-/.]?){7,13}\d)`)

func extractEmails(text string) []string {
	seen := map[string]bool{}
	var out []string
	normalized := strings.NewReplacer(
		"[at]", "@",
		"(at)", "@",
		"[dot]", ".",
		"(dot)", ".",
	).Replace(text)
	for _, m := range emailRe.FindAllString(normalized, -1) {
		m = cleanEmail(m)
		if m == "" {
			continue
		}
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func cleanEmail(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "mailto:")
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s = s[:i]
	}
	if decoded, err := url.QueryUnescape(s); err == nil {
		s = decoded
	}
	s = strings.Trim(s, " <>\"'`.,;:()[]{}")

	if strings.Count(s, "@") != 1 {
		return ""
	}
	if strings.HasSuffix(s, ".png") || strings.HasSuffix(s, ".jpg") ||
		strings.HasSuffix(s, ".jpeg") || strings.HasSuffix(s, ".gif") ||
		strings.HasSuffix(s, ".svg") || strings.HasSuffix(s, ".webp") {
		return ""
	}

	// Structural validation: local part and domain
	at := strings.IndexByte(s, '@')
	local := s[:at]
	domain := s[at+1:]

	// Local part: must be non-empty, no leading/trailing/consecutive dots
	if len(local) == 0 ||
		strings.HasPrefix(local, ".") ||
		strings.HasSuffix(local, ".") ||
		strings.Contains(local, "..") {
		return ""
	}

	// Domain: must have a valid alphabetic TLD (letters only, 2–24 chars)
	dotIdx := strings.LastIndexByte(domain, '.')
	if dotIdx < 1 || dotIdx == len(domain)-1 {
		return ""
	}
	tld := domain[dotIdx+1:]
	if len(tld) < 2 || len(tld) > 24 {
		return ""
	}
	for _, r := range tld {
		if r < 'a' || r > 'z' {
			return ""
		}
	}

	return s
}

func extractPhones(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range phoneRe.FindAllString(text, -1) {
		p := cleanPhone(m)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func cleanPhone(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for i, r := range s {
		if r == '+' && i == 0 {
			b.WriteRune(r)
		} else if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if (r == ' ' || r == '-' || r == '/') && b.Len() > 0 {
			b.WriteRune(' ')
		}
	}
	result := strings.TrimSpace(b.String())
	digits := 0
	var onlyDigits strings.Builder
	for _, r := range result {
		if r >= '0' && r <= '9' {
			digits++
			onlyDigits.WriteRune(r)
		}
	}
	if digits < 10 || digits > 15 {
		return ""
	}
	d := onlyDigits.String()
	if strings.HasPrefix(d, "00") && !strings.HasPrefix(d, "0049") {
		return ""
	}
	return result
}
