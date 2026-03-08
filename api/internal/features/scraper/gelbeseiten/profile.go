package gelbeseiten

import (
	"fmt"

	"github.com/ahmet4dev/gol-lib/logging"
	"github.com/gocolly/colly/v2"
	"go.uber.org/zap"
)

// ContactResult holds contact data extracted from a single GelbeSeiten detail
// page.
type ContactResult struct {
	Phones  []string
	Emails  []string
	Website string
}

// ScrapeProfile visits a GelbeSeiten company detail page directly (when the
// profile URL is already known) and returns the extracted contact data.
//
// This is a standalone single-depth scraper.  For the full search → profile
// pipeline use Scrape() in search.go.
//
// Selectors:
//   - Phone:   a[href^="tel:"]
//   - Website: .contains-icon-big-homepage a
//   - Email:   #email_versenden[data-link]
func ScrapeProfile(profileURL string, cfg Config) (*ContactResult, error) {
	logging.Logger.Info("gelbeseiten profile scrape",
		zap.String("url", profileURL),
	)

	res := &ContactResult{}
	phoneSet := map[string]bool{}
	emailSet := map[string]bool{}

	c := newCollector(cfg, 1)

	c.OnHTML("a[href^='tel:']", func(e *colly.HTMLElement) {
		phone := ParsePhone(e.Attr("href"))
		if phone == "" || phoneSet[phone] {
			return
		}
		phoneSet[phone] = true
		res.Phones = append(res.Phones, phone)
		logging.Logger.Debug("gelbeseiten phone extracted",
			zap.String("phone", phone),
			zap.String("url", profileURL),
		)
	})

	c.OnHTML(".contains-icon-big-homepage a", func(e *colly.HTMLElement) {
		if res.Website != "" {
			return
		}
		website := ParseWebsite(e.Attr("href"))
		if website == "" {
			return
		}
		res.Website = website
		logging.Logger.Debug("gelbeseiten website extracted",
			zap.String("website", website),
			zap.String("url", profileURL),
		)
	})

	c.OnHTML("#email_versenden", func(e *colly.HTMLElement) {
		email := ParseEmail(e.Attr("data-link"))
		if email == "" || emailSet[email] {
			return
		}
		emailSet[email] = true
		res.Emails = append(res.Emails, email)
		logging.Logger.Debug("gelbeseiten email extracted",
			zap.String("email", email),
			zap.String("url", profileURL),
		)
	})

	c.OnError(func(r *colly.Response, err error) {
		logging.Logger.Warn("gelbeseiten profile HTTP error",
			zap.String("url", r.Request.URL.String()),
			zap.Int("status", r.StatusCode),
			zap.Error(err),
		)
	})

	if err := c.Visit(profileURL); err != nil {
		return nil, fmt.Errorf("gelbeseiten profile visit: %w", err)
	}

	logging.Logger.Info("gelbeseiten profile complete",
		zap.String("url", profileURL),
		zap.Strings("phones", res.Phones),
		zap.Strings("emails", res.Emails),
		zap.String("website", res.Website),
	)

	return res, nil
}
