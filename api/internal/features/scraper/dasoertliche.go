package scraper

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gocolly/colly/v2"
	"webscraper/internal/features/matcher"
	"webscraper/internal/features/model"
)

type DasOertlicheScraper struct{}

func (s *DasOertlicheScraper) Name() string { return "dasoertliche.de" }

func (s *DasOertlicheScraper) Scrape(company model.Company, cfg Config) (*ContactInfo, error) {
	searchURL := fmt.Sprintf(
		"https://www.dasoertliche.de/?search=%s&location=%s",
		url.QueryEscape(company.ReName),
		url.QueryEscape(company.ReOrt),
	)

	info := &ContactInfo{Source: s.Name()}
	emailSet := map[string]bool{}
	phoneSet := map[string]bool{}

	c := newCollector(cfg, "www.dasoertliche.de", "dasoertliche.de")

	// 404 means company doesn't exist in dasoertliche — treat as no results, not a retriable error.
	var notFound bool
	c.OnError(func(r *colly.Response, _ error) {
		if r != nil && r.StatusCode == 404 {
			notFound = true
		}
	})

	c.OnHTML(".hitlist-entry, [class*='result'], [class*='Result'], li[class*='hit']", func(e *colly.HTMLElement) {
		candidateName := e.ChildText("[class*='name'],[class*='Name'],h3,h4")
		if candidateName != "" && !matcher.IsGoodMatch(company.ReName, candidateName, 0.55) {
			return
		}

		e.ForEach("a[href^='mailto:']", func(_ int, el *colly.HTMLElement) {
			email := strings.ToLower(strings.TrimPrefix(el.Attr("href"), "mailto:"))
			if strings.Contains(email, "@") {
				emailSet[email] = true
			}
		})

		e.ForEach("[class*='phone'],[class*='Phone'],[class*='tel'],[class*='Tel'],[data-phone]", func(_ int, el *colly.HTMLElement) {
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
		return nil, fmt.Errorf("dasoertliche: %w", err)
	}

	for e := range emailSet {
		info.Emails = append(info.Emails, e)
	}
	for p := range phoneSet {
		info.Phones = append(info.Phones, p)
	}
	return info, nil
}
