package scraper

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gocolly/colly/v2"
	"webscraper/internal/features/matcher"
	"webscraper/internal/features/model"
)

type GelbeSeitenScraper struct{}

func (s *GelbeSeitenScraper) Name() string { return "gelbeseiten.de" }

func (s *GelbeSeitenScraper) Scrape(company model.Company, cfg Config) (*ContactInfo, error) {
	query := strings.ReplaceAll(company.ReName, " ", "-")
	searchURL := fmt.Sprintf(
		"https://www.gelbeseiten.de/suche/%s/%s",
		url.PathEscape(query),
		url.PathEscape(company.ReOrt),
	)

	info := &ContactInfo{Source: s.Name()}
	emailSet := map[string]bool{}
	phoneSet := map[string]bool{}

	c := newCollector(cfg, "www.gelbeseiten.de", "gelbeseiten.de")

	// 404 means company simply doesn't exist in gelbeseiten — treat as no results, not a retriable error.
	var notFound bool
	c.OnError(func(r *colly.Response, _ error) {
		if r != nil && r.StatusCode == 404 {
			notFound = true
		}
	})

	c.OnHTML("article", func(e *colly.HTMLElement) {
		candidateName := e.ChildText("h2, [class*='name'], [class*='Name']")
		if candidateName == "" {
			return
		}
		if !matcher.IsGoodMatch(company.ReName, candidateName, 0.55) {
			return
		}

		e.ForEach("a[href^='mailto:']", func(_ int, el *colly.HTMLElement) {
			email := strings.ToLower(strings.TrimPrefix(el.Attr("href"), "mailto:"))
			if strings.Contains(email, "@") {
				emailSet[email] = true
			}
		})

		e.ForEach("[data-phone],[class*='phone'],[class*='Phone'],[class*='tel'],[class*='Tel']", func(_ int, el *colly.HTMLElement) {
			phone := el.Attr("data-phone")
			if phone == "" {
				phone = strings.TrimSpace(el.Text)
			}
			if p := cleanPhone(phone); p != "" {
				phoneSet[p] = true
			}
		})

		for _, em := range extractEmails(e.Text) {
			emailSet[em] = true
		}
		for _, p := range extractPhones(e.Text) {
			phoneSet[p] = true
		}
	})

	if err := c.Visit(searchURL); err != nil {
		if notFound {
			return info, nil
		}
		return nil, fmt.Errorf("gelbeseiten: %w", err)
	}

	for e := range emailSet {
		info.Emails = append(info.Emails, e)
	}
	for p := range phoneSet {
		info.Phones = append(info.Phones, p)
	}
	return info, nil
}
