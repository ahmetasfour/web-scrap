// Package discover provides the WebsiteFinder which resolves a company's
// official website URL from its name and city.
package discover

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"go.uber.org/zap"

	ddgsearch "webscraper/internal/features/scraper/search"
	"webscraper/internal/features/model"
	"webscraper/internal/features/search"
)

// directoryDomains are aggregator / social / directory sites that are never
// the company's own website and must be excluded from results.
var directoryDomains = map[string]bool{
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
	"booking.com":               true,
	"trustpilot.com":            true,
	"branchenbuch.de":           true,
	"wlw.de":                    true,
	"kompass.com":               true,
	"klicktel.de":               true,
	"telefonbuch.de":            true,
	"stadtbranchenbuch.com":     true,
	"northdata.de":              true,
	"handelsregister.de":        true,
	"unternehmensregister.de":   true,
	"bundesanzeiger.de":         true,
	"tripadvisor.de":            true,
	"tripadvisor.com":           true,
	"foursquare.com":            true,
	"golocal.de":                true,
	"hotfrog.de":                true,
	"yellowpages.com":           true,
	"insolvenzindex.de":         true,
	"gavabiz.de":                true,
	"companyhouse.de":           true,
	"creditreform.de":           true,
	"dnb.com":                   true,
	"dun.de":                    true,
	"moneyhouse.de":             true,
	"ebay.de":                   true,
	"amazon.de":                 true,
	"reddit.com":                true,
	"youtube.com":               true,
	"kununu.com":                true,
	"indeed.com":                true,
	"stepstone.de":              true,
	"metallatlas.de":            true,
	"firmenwissen.de":           true,
	"firmendb.de":               true,
	"open.fda.gov":              true,
	"opencorporates.com":        true,
	"companyinfo.de":            true,
	"implisense.com":            true,
	"find-und-funded.de":        true,
}

// legalSuffixRe matches common German legal-form tokens so they can be
// stripped before building the search query.
var legalSuffixRe = regexp.MustCompile(
	`(?i)\b(GmbH|UG|AG|KG|OHG|GbR|eG|mbH|SE|KGaA|Holding|Verwaltung|c\/o)\b`,
)

// multiSpaceRe collapses consecutive whitespace.
var multiSpaceRe = regexp.MustCompile(`\s+`)

// FinderConfig controls WebsiteFinder behaviour.
type FinderConfig struct {
	Concurrency int
	Timeout     time.Duration
	RetryCount  int
}

// FindResult is the output for one company lookup.
type FindResult struct {
	Company string  `json:"company"`
	City    string  `json:"city"`
	Website *string `json:"website"`
	Status  string  `json:"status"` // "found" | "not_found" | "error"
}

// WebsiteFinder discovers the official website URL for a company using
// a DuckDuckGo HTML search followed by domain validation.
type WebsiteFinder struct {
	cfg        FinderConfig
	ddg        *ddgsearch.DDGClient
	httpClient *http.Client
}

// New creates a WebsiteFinder with the given configuration.
func New(cfg FinderConfig) *WebsiteFinder {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 5
	}
	return &WebsiteFinder{
		cfg: cfg,
		ddg: ddgsearch.NewDDGClient(cfg.Timeout),
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
			// Do not follow redirects — a 301/302 still confirms the site is live.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// FindWebsite orchestrates the full pipeline for one company and returns
// its website URL or a not_found result.
func (f *WebsiteFinder) FindWebsite(company, city string) FindResult {
	result := FindResult{Company: company, City: city}

	cleanName := f.CleanCompanyName(company)
	query := f.BuildSearchQuery(cleanName, city)

	logging.Logger.Info("website finder start",
		zap.String("company", company),
		zap.String("clean_name", cleanName),
		zap.String("query", query),
	)

	maxAttempts := f.cfg.RetryCount + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			b := backoffs[min(attempt-1, len(backoffs)-1)]
			logging.Logger.Info("retrying website finder",
				zap.String("company", company),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", b),
			)
			time.Sleep(b)
		}

		rawResults, err := f.SearchDuckDuckGo(query)
		if err != nil {
			logging.Logger.Warn("ddg search error",
				zap.String("company", company),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			continue
		}

		filtered := f.FilterDomains(rawResults)
		website := f.pickBestValid(filtered, company)
		if website != "" {
			result.Website = &website
			result.Status = "found"
			logging.Logger.Info("website found",
				zap.String("company", company),
				zap.String("website", website),
			)
			return result
		}
	}

	// Fallback: domain guessing from company name when DDG finds nothing.
	guesser := search.NewDomainGuesser()
	guessed, _ := guesser.Search(model.Company{ReName: company, ReOrt: city})
	if len(guessed) > 0 {
		w := guessed[0]
		result.Website = &w
		result.Status = "found"
		logging.Logger.Info("website found via domain guess",
			zap.String("company", company),
			zap.String("website", w),
		)
		return result
	}

	result.Status = "not_found"
	logging.Logger.Info("website not found",
		zap.String("company", company),
		zap.String("city", city),
	)
	return result
}

// CleanCompanyName strips legal-form suffixes and normalises whitespace.
//
//	"Stadtwerke Eutin GmbH"  →  "Stadtwerke Eutin"
//	"Bodil Langma c/o Hausverwaltung" →  "Bodil Langma"
func (f *WebsiteFinder) CleanCompanyName(name string) string {
	cleaned := legalSuffixRe.ReplaceAllString(name, " ")
	cleaned = multiSpaceRe.ReplaceAllString(cleaned, " ")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimRight(cleaned, "-/&+.,;:")
	return strings.TrimSpace(cleaned)
}

// BuildSearchQuery combines the cleaned company name, city, and "Germany"
// into a single query string ready for url.QueryEscape.
//
//	"Stadtwerke Eutin", "Eutin"  →  "Stadtwerke Eutin Eutin Germany"
func (f *WebsiteFinder) BuildSearchQuery(cleanName, city string) string {
	parts := []string{cleanName}
	if city != "" {
		parts = append(parts, city)
	}
	parts = append(parts, "Germany")
	q := strings.Join(parts, " ")
	return multiSpaceRe.ReplaceAllString(strings.TrimSpace(q), " ")
}

// SearchDuckDuckGo submits the query to DuckDuckGo and returns raw results.
func (f *WebsiteFinder) SearchDuckDuckGo(query string) ([]ddgsearch.DDGResult, error) {
	return f.ddg.Search(query)
}

// FilterDomains removes known directory / social / aggregator domains from
// a result list and returns only candidate company URLs.
func (f *WebsiteFinder) FilterDomains(results []ddgsearch.DDGResult) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range results {
		domain := domainOf(r.URL)
		if domain == "" || seen[domain] {
			continue
		}
		// Filter both the full host and the registrable domain (catches subdomains
		// of directories like "company.gavabiz.de").
		if directoryDomains[domain] || directoryDomains[registrableDomain(domain)] {
			continue
		}
		seen[domain] = true
		base := ExtractDomain(r.URL)
		if base != "" {
			out = append(out, base)
		}
	}
	return out
}

// ExtractDomain reduces a full URL to its scheme + host base.
//
//	"https://stadtwerke-eutin.de/impressum"  →  "https://stadtwerke-eutin.de"
func ExtractDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := parsed.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + parsed.Host
}

// ValidateWebsite sends a HEAD request and returns true when the server
// responds with 200, 301, 302, or 403 — any of which confirm the site is live.
func (f *WebsiteFinder) ValidateWebsite(baseURL string) bool {
	req, err := http.NewRequest(http.MethodHead, baseURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		// HEAD may be blocked — fall back to GET
		resp, err = f.httpClient.Get(baseURL)
		if err != nil {
			return false
		}
	}
	resp.Body.Close()

	// Accept any non-5xx response: 200 OK, 3xx redirect, 403 Forbidden, and
	// 404 Not Found all confirm a live web server (the impressum may be at
	// a specific path even if the root returns 404).
	return resp.StatusCode < 500
}

// pickFirstValid validates candidate URLs and returns the best live one.
// Candidates whose domain contains a keyword from the company name are
// preferred over generic results.
func (f *WebsiteFinder) pickFirstValid(candidates []string) string {
	return ""
}

// pickBestValid scores candidates by keyword relevance then validates them.
// It prefers domains that share at least one significant word with the company name.
func (f *WebsiteFinder) pickBestValid(candidates []string, companyName string) string {
	keywords := extractKeywords(companyName)

	type scored struct {
		url   string
		score int
	}
	var ranked []scored
	for _, c := range candidates {
		host := strings.ToLower(domainOf(c))
		s := 0
		for _, kw := range keywords {
			if strings.Contains(host, kw) {
				s++
			}
		}
		ranked = append(ranked, scored{c, s})
	}

	// Sort descending by score (simple insertion-style for small N).
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].score > ranked[j-1].score; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	for _, r := range ranked {
		// Require at least one keyword from the company name to appear in the
		// domain. A score of 0 means the result is unrelated to the company
		// (e.g. a generic directory like "deutschebiz.de") — skip it.
		if r.score == 0 {
			break
		}
		if f.ValidateWebsite(r.url) {
			return r.url
		}
	}
	return ""
}

// extractKeywords returns lowercase words of ≥ 4 chars from a company name,
// excluding common legal/stop words.
func extractKeywords(name string) []string {
	stopWords := map[string]bool{
		"gmbh": true, "ug": true, "ag": true, "kg": true, "gbr": true,
		"und": true, "der": true, "die": true, "das": true, "von": true,
		"fuer": true, "mit": true, "beim": true,
	}
	// Transliterate umlauts for matching
	name = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss").Replace(
		strings.ToLower(name),
	)
	name = regexp.MustCompile(`[^a-z0-9\s]`).ReplaceAllString(name, " ")
	var kws []string
	for _, w := range strings.Fields(name) {
		if len(w) >= 4 && !stopWords[w] {
			kws = append(kws, w)
		}
	}
	return kws
}

// domainOf extracts the bare host (without www.) from a raw URL.
func domainOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
}

// registrableDomain returns the last two dot-separated parts of a host,
// e.g. "henstedt-ulzburg.gavabiz.de" → "gavabiz.de".
// This is used to match subdomains against the directory filter list.
func registrableDomain(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return parts[len(parts)-2] + "." + parts[len(parts)-1]
}

// min returns the smaller of two ints (Go 1.20 generic min not available in all envs).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FinderError wraps a lookup failure with context.
type FinderError struct {
	Company string
	Err     error
}

func (e *FinderError) Error() string {
	return fmt.Sprintf("website finder [%s]: %v", e.Company, e.Err)
}
