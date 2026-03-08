package gelbeseiten

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/extensions"
	"go.uber.org/zap"

	"webscraper/internal/features/model"
)

const gelbeSeitenBase = "https://www.gelbeseiten.de"

// Config holds rate-limiting and timeout parameters for all GelbeSeiten requests.
type Config struct {
	RequestTimeout time.Duration
	RequestDelay   time.Duration
	RandomDelay    time.Duration
	RetryCount     int
}

// Scrape is the unified two-stage pipeline for one company.
//
// Stage 1 — Search:
//
//	GET https://www.gelbeseiten.de/suche/{name-slug}/{city-slug}
//	e.g. /suche/alves-baumschulen/borstel-hohenraden
//
// Stage 2 — Detail page (first result only):
//
//	The first <article id="treffer_…"> on the results page is selected.
//	Its primary <a href="…"> link is visited (depth 2).
//	Phone, website, and email are extracted from the detail page.
//
// A single Colly collector is used for both stages so that rate-limit rules
// and browser headers apply uniformly.
//
// Returns nil, nil when no search result exists (caller treats as not_found).
func Scrape(company model.Company, cfg Config) (*ContactResult, error) {
	searchURL := buildSearchURL(company)

	logging.Logger.Info("gelbeseiten scrape start",
		zap.String("company", company.ReName),
		zap.String("city", company.ReOrt),
		zap.String("searchURL", searchURL),
	)

	res := &ContactResult{}
	phoneSet := map[string]bool{}
	emailSet := map[string]bool{}

	var profileURL string
	firstResult := true

	// MaxDepth(2): depth 1 = search page, depth 2 = company detail page.
	c := newCollector(cfg, 2)

	// ── Stage 2: first result article ──────────────────────────────────────
	//
	// HTML structure on the results page:
	//
	//   <article id="treffer_129011538827" class="mod mod-Treffer">
	//     <a href="https://www.gelbeseiten.de/gsbiz/…">
	//       <h2 class="mod-Treffer__name">Company Name</h2>
	//     </a>
	//   </article>
	//
	// We fire on the article element (not on each <a> inside it) to get a
	// clean single callback per result. ChildAttr picks the first matching
	// anchor's href.
	c.OnHTML("article[id^='treffer_']", func(e *colly.HTMLElement) {
		if !firstResult {
			return
		}
		href := strings.TrimSpace(e.ChildAttr("a[href]", "href"))
		if href == "" {
			return
		}
		firstResult = false
		profileURL = e.Request.AbsoluteURL(href)

		logging.Logger.Info("gelbeseiten first result selected",
			zap.String("company", company.ReName),
			zap.String("articleID", e.Attr("id")),
			zap.String("profileURL", profileURL),
		)

		if err := e.Request.Visit(profileURL); err != nil {
			logging.Logger.Warn("gelbeseiten detail visit failed",
				zap.String("url", profileURL),
				zap.Error(err),
			)
		}
	})

	// ── Stage 3/4: extract contact data from the detail page ───────────────
	//
	// All three handlers are guarded by depth == 2 so they only fire on the
	// detail page, not on the search-results page where similar tags may appear.

	// Phone: <a href="tel:+4946515903">
	c.OnHTML("a[href^='tel:']", func(e *colly.HTMLElement) {
		if e.Request.Depth != 2 {
			return
		}
		phone := ParsePhone(e.Attr("href"))
		if phone == "" || phoneSet[phone] {
			return
		}
		phoneSet[phone] = true
		res.Phones = append(res.Phones, phone)
		logging.Logger.Debug("gelbeseiten phone extracted",
			zap.String("phone", phone),
			zap.String("url", e.Request.URL.String()),
		)
	})

	// Website: <div class="… contains-icon-big-homepage …"><a href="http://…">
	c.OnHTML(".contains-icon-big-homepage a", func(e *colly.HTMLElement) {
		if e.Request.Depth != 2 || res.Website != "" {
			return
		}
		website := ParseWebsite(e.Attr("href"))
		if website == "" {
			return
		}
		res.Website = website
		logging.Logger.Debug("gelbeseiten website extracted",
			zap.String("website", website),
			zap.String("url", e.Request.URL.String()),
		)
	})

	// Email: <div id="email_versenden"
	//             data-link="mailto:info@company.de?subject=…">
	c.OnHTML("#email_versenden", func(e *colly.HTMLElement) {
		if e.Request.Depth != 2 {
			return
		}
		email := ParseEmail(e.Attr("data-link"))
		if email == "" || emailSet[email] {
			return
		}
		emailSet[email] = true
		res.Emails = append(res.Emails, email)
		logging.Logger.Debug("gelbeseiten email extracted",
			zap.String("email", email),
			zap.String("url", e.Request.URL.String()),
		)
	})

	c.OnError(func(r *colly.Response, err error) {
		logging.Logger.Warn("gelbeseiten HTTP error",
			zap.String("url", r.Request.URL.String()),
			zap.Int("status", r.StatusCode),
			zap.Error(err),
		)
	})

	if err := c.Visit(searchURL); err != nil {
		return nil, fmt.Errorf("gelbeseiten search visit: %w", err)
	}

	if profileURL == "" {
		logging.Logger.Info("gelbeseiten no results",
			zap.String("company", company.ReName),
			zap.String("city", company.ReOrt),
		)
		return nil, nil
	}

	logging.Logger.Info("gelbeseiten scrape complete",
		zap.String("company", company.ReName),
		zap.String("profileURL", profileURL),
		zap.Strings("phones", res.Phones),
		zap.Strings("emails", res.Emails),
		zap.String("website", res.Website),
	)

	return res, nil
}

// buildSearchURL constructs the GelbeSeiten slug-based search URL.
//
// GelbeSeiten uses path segments rather than query parameters:
//
//	https://www.gelbeseiten.de/suche/{name-slug}/{city-slug}
//
// Examples:
//
//	"Alves Baumschulen", "Borstel-Hohenraden"
//	  → /suche/alves-baumschulen/borstel-hohenraden
//
//	"F-I-M Fritzsche Immobilien Management GmbH", "Hamburg"
//	  → /suche/f-i-m-fritzsche-immobilien-management-gmbh/hamburg
func buildSearchURL(company model.Company) string {
	nameSlug := toSlug(company.ReName)
	citySlug := toSlug(company.ReOrt)
	if citySlug != "" {
		return fmt.Sprintf("%s/suche/%s/%s", gelbeSeitenBase, nameSlug, citySlug)
	}
	return fmt.Sprintf("%s/suche/%s", gelbeSeitenBase, nameSlug)
}

// umlautReplacer converts German umlauts to their ASCII equivalents before
// slug generation so that "München" becomes "muenchen", not "m-nchen".
var umlautReplacer = strings.NewReplacer(
	"ä", "ae", "Ä", "ae",
	"ö", "oe", "Ö", "oe",
	"ü", "ue", "Ü", "ue",
	"ß", "ss",
)

// nonAlphanumRe matches any run of characters that are not lowercase letters
// or digits.  Used to replace them with a single hyphen during slugification.
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// toSlug converts an arbitrary string to a URL-safe lowercase slug.
//
//	"F-I-M Fritzsche Immobilien Management GmbH" → "f-i-m-fritzsche-immobilien-management-gmbh"
//	"Borstel-Hohenraden"                         → "borstel-hohenraden"
//	"München"                                     → "muenchen"
func toSlug(s string) string {
	s = strings.ToLower(s)
	s = umlautReplacer.Replace(s)
	s = nonAlphanumRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// newCollector builds a Colly collector locked to gelbeseiten.de.
// maxDepth: 1 = search only, 2 = search + detail page.
func newCollector(cfg Config, maxDepth int) *colly.Collector {
	c := colly.NewCollector(
		colly.AllowedDomains("www.gelbeseiten.de", "gelbeseiten.de"),
		colly.MaxDepth(maxDepth),
	)
	extensions.RandomUserAgent(c)
	c.SetRequestTimeout(cfg.RequestTimeout)

	// Browser-like headers to minimise bot-detection.
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "de-DE,de;q=0.9,en-US;q=0.8,en;q=0.7")
		r.Headers.Set("Cache-Control", "no-cache")
	})

	if cfg.RequestDelay > 0 {
		_ = c.Limit(&colly.LimitRule{
			DomainGlob:  "*gelbeseiten*",
			Delay:       cfg.RequestDelay,
			RandomDelay: cfg.RandomDelay,
		})
	}

	return c
}
