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
// Cache behaviour:
//   - Before every search the cache is checked by key (name|city).
//   - A result is written to the cache ONLY when at least one contact field
//     (phone, email, or website) was actually found.  Empty profiles are never
//     archived so the next run will try GelbeSeiten again.
//   - The cache is persisted to scraper_cache.json after every successful hit
//     so results survive server restarts.
type GelbeSeitenSource struct {
	cache *cache.Store
}

func (g *GelbeSeitenSource) Name() string { return "gelbeseiten" }

// Scrape runs:  cache-lookup → gelbeseiten search → archive result → return.
func (g *GelbeSeitenSource) Scrape(company model.Company, cfg Config) (*ContactInfo, error) {
	empty := &ContactInfo{Source: g.Name()}

	// ── 1. Cache lookup ─────────────────────────────────────────────────────
	key := cache.BuildKey(company.ReName, company.ReOrt)

	if hit, ok := g.cache.Get(key); ok {
		logging.Logger.Info("cache hit — skipping search",
			zap.String("company", company.ReName),
			zap.String("city", company.ReOrt),
			zap.String("key", key),
			zap.Time("cachedAt", hit.CachedAt),
			zap.Strings("phones", hit.Phones),
			zap.Strings("emails", hit.Emails),
			zap.String("website", hit.Website),
		)
		return &ContactInfo{
			Phones:  hit.Phones,
			Emails:  hit.Emails,
			Website: hit.Website,
			Source:  hit.Source,
		}, nil
	}

	logging.Logger.Info("cache miss — searching gelbeseiten",
		zap.String("company", company.ReName),
		zap.String("city", company.ReOrt),
		zap.String("key", key),
	)

	// ── 2. Scrape GelbeSeiten ────────────────────────────────────────────────
	gsCfg := gelbeseiten.Config{
		RequestTimeout: cfg.RequestTimeout,
		RequestDelay:   cfg.RequestDelay,
		RandomDelay:    cfg.RandomDelay,
		RetryCount:     cfg.RetryCount,
	}

	contact, err := gelbeseiten.Scrape(company, gsCfg)
	if err != nil {
		logging.Logger.Warn("gelbeseiten scrape error",
			zap.String("company", company.ReName),
			zap.Error(err),
		)
		return empty, nil
	}

	// nil → no search result at all (company not listed on GelbeSeiten).
	if contact == nil {
		logging.Logger.Info("gelbeseiten not found",
			zap.String("company", company.ReName),
			zap.String("city", company.ReOrt),
		)
		return empty, nil
	}

	result := &ContactInfo{
		Phones:  contact.Phones,
		Emails:  contact.Emails,
		Website: contact.Website,
		Source:  g.Name(),
	}

	// ── 3. Archive ONLY when at least one contact field was found ────────────
	//
	// Empty profiles (profile page opened but no phone / email / website
	// present) are intentionally skipped so the company will be tried again
	// on the next run rather than being permanently marked as "found but empty".
	if hasContactData(result) {
		g.cache.Set(key, cache.CachedContact{
			Phones:  result.Phones,
			Emails:  result.Emails,
			Website: result.Website,
			Source:  result.Source,
		})
		logging.Logger.Info("archived to cache",
			zap.String("company", company.ReName),
			zap.String("key", key),
			zap.Strings("phones", result.Phones),
			zap.Strings("emails", result.Emails),
			zap.String("website", result.Website),
			zap.Int("totalCached", g.cache.Len()),
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
