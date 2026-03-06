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
	detailURLs := map[string]bool{}
	threshold := cfg.MatchThreshold
	if threshold <= 0 {
		threshold = 0.55
	}

	c := newCollector(cfg, "www.gelbeseiten.de", "gelbeseiten.de")
	detailCollector := newCollector(cfg, "www.gelbeseiten.de", "gelbeseiten.de")

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
		if !matcher.IsGoodMatch(company.ReName, candidateName, threshold) {
			return
		}

		e.ForEach("a[href^='mailto:']", func(_ int, el *colly.HTMLElement) {
			email := cleanEmail(el.Attr("href"))
			if email != "" {
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

		e.ForEach("a[href]", func(_ int, el *colly.HTMLElement) {
			href := strings.TrimSpace(el.Attr("href"))
			if href == "" || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
				return
			}
			abs := e.Request.AbsoluteURL(href)
			if abs == "" || strings.Contains(abs, "/suche/") {
				return
			}
			detailURLs[abs] = true
		})
	})

	detailCollector.OnHTML("body", func(e *colly.HTMLElement) {
		e.ForEach("a[href^='mailto:']", func(_ int, el *colly.HTMLElement) {
			if email := cleanEmail(el.Attr("href")); email != "" {
				emailSet[email] = true
			}
		})
		e.ForEach("a[href^='tel:']", func(_ int, el *colly.HTMLElement) {
			if p := cleanPhone(strings.TrimPrefix(el.Attr("href"), "tel:")); p != "" {
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
	if len(emailSet) == 0 {
		visited := 0
		for u := range detailURLs {
			if visited >= 5 {
				break
			}
			visited++
			_ = detailCollector.Visit(u)
		}
	}

	for e := range emailSet {
		info.Emails = append(info.Emails, e)
	}
	for p := range phoneSet {
		info.Phones = append(info.Phones, p)
	}
	return info, nil
}
