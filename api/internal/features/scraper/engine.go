package scraper

import (
	"context"
	"net/url"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/extensions"
	"go.uber.org/zap"

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
	Emails []string
	Phones []string
	Source string
}

// Source is the interface implemented by each scraping target.
type Source interface {
	Name() string
	Scrape(company model.Company, cfg Config) (*ContactInfo, error)
}

// Config controls the scraping behaviour.
type Config struct {
	Concurrency    int
	RequestDelay   time.Duration
	RandomDelay    time.Duration
	RetryCount     int
	RequestTimeout time.Duration
	MatchThreshold float64
}

// Engine manages parallel scraping with rate limiting and retries.
type Engine struct {
	cfg     Config
	sources []Source
}

// New creates an Engine with the given config.
func New(cfg Config) *Engine {
	return &Engine{
		cfg: cfg,
		sources: []Source{
			&GelbeSeitenScraper{},
			&DasOertlicheScraper{},
		},
	}
}

// newCollector creates a fresh collector for each scrape call.
// Each collector visits exactly one URL, so per-domain rate-limit rules have
// no effect and are omitted. The shared transport enables TCP keep-alive
// connection reuse across all concurrent goroutines.
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
// The channel is closed when all companies have been processed.
// Cancelling ctx stops dispatching new work; in-flight goroutines finish naturally.
func (e *Engine) RunStream(ctx context.Context, companies []model.Company, resultCh chan<- model.ScrapeResult) {
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
		// Stop dispatching new work when context is cancelled.
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

			result := e.scrapeOne(g.canonical)
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
func (e *Engine) Run(companies []model.Company) []model.ScrapeResult {
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
			uniqueResults[idx] = e.scrapeOne(g.canonical)
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

// scrapeOne runs all sources in parallel and returns on the first successful hit.
func (e *Engine) scrapeOne(company model.Company) model.ScrapeResult {
	if company.Email != "" && company.Telefonnummer != "" {
		logging.Logger.Info("skipping — already complete", zap.String("company", company.ReName))
		return model.ScrapeResult{
			Company: company,
			Status:  "done",
			Emails:  []string{company.Email},
			Phones:  []string{company.Telefonnummer},
			Source:  "excel",
		}
	}

	type hit struct {
		emails []string
		phones []string
		source string
	}

	hitCh := make(chan hit, len(e.sources))
	var wg sync.WaitGroup

	for _, src := range e.sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logging.Logger.Error("panic in source",
						zap.String("source", s.Name()),
						zap.String("company", company.ReName),
						zap.Any("panic", r),
					)
				}
			}()

			info, err := s.Scrape(company, e.cfg)
			if err != nil {
				logging.Logger.Warn("scrape error",
					zap.String("source", s.Name()),
					zap.String("company", company.ReName),
					zap.Error(err),
				)
				return
			}
			if len(info.Emails) > 0 || len(info.Phones) > 0 {
				hitCh <- hit{info.Emails, info.Phones, info.Source}
			}
		}(src)
	}

	go func() {
		wg.Wait()
		close(hitCh)
	}()

	var best *hit
	for h := range hitCh {
		hCopy := h
		if best == nil || isBetterHit(hCopy.emails, hCopy.phones, best.emails, best.phones) {
			best = &hCopy
		}
	}
	if best != nil {
		logging.Logger.Info("found",
			zap.String("source", best.source),
			zap.String("company", company.ReName),
			zap.Strings("emails", best.emails),
			zap.Strings("phones", best.phones),
		)
		return model.ScrapeResult{
			Company: company,
			Status:  "done",
			Emails:  best.emails,
			Phones:  best.phones,
			Source:  best.source,
		}
	}

	logging.Logger.Info("not found",
		zap.String("company", company.ReName),
		zap.String("city", company.ReOrt),
	)
	return model.ScrapeResult{Company: company, Status: "not_found"}
}

func isBetterHit(aEmails, aPhones, bEmails, bPhones []string) bool {
	aHasEmail := len(aEmails) > 0
	bHasEmail := len(bEmails) > 0
	if aHasEmail != bHasEmail {
		return aHasEmail
	}
	if len(aEmails) != len(bEmails) {
		return len(aEmails) > len(bEmails)
	}
	if len(aPhones) != len(bPhones) {
		return len(aPhones) > len(bPhones)
	}
	return false
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
	if digits < 9 || digits > 14 {
		return ""
	}
	d := onlyDigits.String()
	if strings.HasPrefix(d, "00") && !strings.HasPrefix(d, "0049") {
		return ""
	}
	return result
}
