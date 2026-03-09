package scraper

import (
	"github.com/ahmet4dev/gol-lib/logging"
	"go.uber.org/zap"

	"webscraper/internal/cache"
	"webscraper/internal/features/model"
	"webscraper/internal/features/scraper/gelbeseiten"
)

// GelbeSeitenSource implements Source by running the unified GelbeSeiten
// pipeline: search → first result → extract phone / email / website.
//
// Cache behaviour (scraper_cache.json):
//   - FOUND table   – Before every search the found-cache is checked by key.
//     A result is written only when at least one contact field was found.
//   - NOT_FOUND table – Companies that returned no GelbeSeiten results are
//     stored so the next run skips them entirely (no HTTP request sent).
//   - Both tables survive server restarts via the shared JSON file.
type GelbeSeitenSource struct {
	cache *cache.Store
}

func (g *GelbeSeitenSource) Name() string { return "gelbeseiten" }

// Scrape runs:
//  1. Check FOUND cache   → return stored data (skip request)
//  2. Check NOT_FOUND cache → return empty  (skip request)
//  3. Scrape GelbeSeiten
//  4. Save result to the appropriate cache table
func (g *GelbeSeitenSource) Scrape(company model.Company, cfg Config) (*ContactInfo, error) {
	empty := &ContactInfo{Source: g.Name()}
	key := cache.BuildKey(company.ReName, company.ReOrt)

	// ── 1. FOUND cache lookup ─────────────────────────────────────────────────
	if hit, ok := g.cache.Get(key); ok {
		logging.Logger.Info("cache_found_company — skipping search",
			zap.String("company", company.ReName),
			zap.String("city", company.ReOrt),
			zap.Time("cachedAt", hit.CachedAt),
			zap.Strings("phones", hit.Phones),
			zap.Strings("emails", hit.Emails),
			zap.String("website", hit.Website),
		)
		return &ContactInfo{
			Phones:  hit.Phones,
			Emails:  hit.Emails,
			Website: hit.Website,
			Source:  "cache_found",
		}, nil
	}

	// ── 2. NOT_FOUND cache lookup ─────────────────────────────────────────────
	if g.cache.IsNotFound(key) {
		logging.Logger.Info("cache_not_found_company — skipping search",
			zap.String("company", company.ReName),
			zap.String("city", company.ReOrt),
		)
		return empty, nil
	}

	// ── 3. Scrape GelbeSeiten ─────────────────────────────────────────────────
	logging.Logger.Info("scrape_started",
		zap.String("company", company.ReName),
		zap.String("city", company.ReOrt),
		zap.String("key", key),
	)

	gsCfg := gelbeseiten.Config{
		RequestTimeout: cfg.RequestTimeout,
		RequestDelay:   cfg.RequestDelay,
		RandomDelay:    cfg.RandomDelay,
		RetryCount:     cfg.RetryCount,
	}

	contact, err := gelbeseiten.Scrape(company, gsCfg)
	if err != nil {
		logging.Logger.Warn("scrape_failed",
			zap.String("company", company.ReName),
			zap.Error(err),
		)
		return empty, nil
	}

	// nil → no search result at all (company not listed on GelbeSeiten).
	if contact == nil {
		logging.Logger.Info("scrape_completed — not found on GelbeSeiten",
			zap.String("company", company.ReName),
			zap.String("city", company.ReOrt),
		)
		// ── 4a. Save to NOT_FOUND cache ──────────────────────────────────────
		g.cache.SetNotFound(key, g.Name())
		logging.Logger.Info("company_saved_not_found",
			zap.String("company", company.ReName),
			zap.String("city", company.ReOrt),
			zap.Int("totalNotFound", g.cache.LenNotFound()),
		)
		return empty, nil
	}

	result := &ContactInfo{
		Phones:  contact.Phones,
		Emails:  contact.Emails,
		Website: contact.Website,
		Source:  g.Name(),
	}

	logging.Logger.Info("scrape_completed",
		zap.String("company", company.ReName),
		zap.Strings("phones", result.Phones),
		zap.Strings("emails", result.Emails),
		zap.String("website", result.Website),
	)

	// ── 4b. Save to FOUND cache (only when at least one contact field found) ──
	//
	// Empty profiles (profile page opened but no phone / email / website
	// present) are intentionally NOT cached so the company will be retried
	// on the next run rather than being permanently marked as "found but empty".
	if hasContactData(result) {
		g.cache.Set(key, cache.CachedContact{
			Phones:  result.Phones,
			Emails:  result.Emails,
			Website: result.Website,
			Source:  result.Source,
		})
		logging.Logger.Info("company_saved_found",
			zap.String("company", company.ReName),
			zap.String("key", key),
			zap.Strings("phones", result.Phones),
			zap.Strings("emails", result.Emails),
			zap.String("website", result.Website),
			zap.Int("totalFound", g.cache.Len()),
		)
	} else {
		logging.Logger.Info("profile found but no contact data — not archived",
			zap.String("company", company.ReName),
			zap.String("city", company.ReOrt),
		)
	}

	return result, nil
}

// hasContactData returns true when at least one useful field is non-empty.
func hasContactData(info *ContactInfo) bool {
	return len(info.Phones) > 0 || len(info.Emails) > 0 || info.Website != ""
}
