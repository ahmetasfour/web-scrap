# Web Scraper Project - Instructions

## Project Overview

A web scraping application that extracts contact information (emails, phone numbers, and websites) for German property management companies from public directories and the web.

---

## Architecture

### Backend (Go) — `api/`

**Entry point:** `cmd/server/main.go`
**Module name:** `webscraper`

**Framework & libraries:**
- `github.com/gofiber/fiber/v2` — HTTP server
- `github.com/gocolly/colly/v2` — HTML scraping
- `github.com/ahmet4dev/gol-lib` — Logging (Zap) + middleware helpers
- `go.uber.org/zap` — Structured logging

**Default port:** `10000` (overridden by `PORT` env var)

---

### Frontend (Next.js) — `frontend/`

- Next.js 15 (App Router)
- Ant Design v5
- Tailwind CSS
- TypeScript
- SWR for data fetching
- XLSX for Excel parsing

---

## Backend Directory Structure

```
api/
├── cmd/server/main.go              # Entry point: loads config, builds engine, starts Fiber
├── config.json                     # Runtime configuration (see below)
├── go.mod / go.sum
├── handlers/http/
│   ├── handler.go                  # HTTP handler methods (Scrape, ScrapeStream, StopScrape, History)
│   └── router.go                   # Route registration
└── internal/
    ├── cache/
    │   └── store.go                # Thread-safe in-memory cache + JSON file persistence
    ├── configs/
    │   └── config.go               # Config struct, Load(), defaults
    └── features/
        ├── model/
        │   └── company.go          # Company, ScrapeResult, ScrapeRequest, FilterMode types
        ├── matcher/
        │   └── matcher.go          # Fuzzy name matching (Levenshtein distance)
        └── scraper/
            ├── engine.go           # Engine: Run(), RunStream(), scrapeOne(), deduplication
            ├── website.go          # GelbeSeitenSource (implements Source interface)
            ├── gelbeseiten/
            │   ├── search.go       # Two-stage search→profile pipeline for gelbeseiten.de
            │   ├── profile.go      # ScrapeProfile() for direct profile URL scraping
            │   └── parser.go       # ParsePhone(), ParseWebsite(), ParseEmail()
            ├── discover/
            │   └── website_finder.go  # WebsiteFinder: DDG search → domain validation
            └── search/
                ├── duckduckgo.go   # DuckDuckGoSearcher (net/http + regex, no Colly)
                ├── searxng.go      # SearXNGSearcher (JSON API)
                └── domain_guess.go # DomainGuesser: slug-based domain candidates + HEAD check
```

---

## Data Models (`internal/features/model/company.go`)

### `Company` — input row from Excel
| Field | Type | Description |
|---|---|---|
| `id` | int | Row identifier |
| `enObjekt` | int | Object number |
| `reName` | string | Primary company name |
| `reName2` | string | Alternative name |
| `objektRechnung` | string | Invoice object |
| `reOrt` | string | City |
| `reHausnummer` | string | House number |
| `rePlz` | string | Postal code |
| `reStrasse` | string | Street |
| `reNummer` | string | Number |
| `email` | string | Existing email (may be pre-filled) |
| `telefonnummer` | string | Existing phone (may be pre-filled) |

### `ScrapeResult` — output (embeds Company)
| Field | Type | Description |
|---|---|---|
| `status` | string | `"done"` \| `"not_found"` \| `"error"` |
| `emails` | []string | Scraped email addresses |
| `phones` | []string | Scraped phone numbers |
| `source` | string | Where data was found (`"gelbeseiten"`, `"excel"`, etc.) |
| `website` | string | Company website URL (optional) |
| `error` | string | Error message (optional) |

### `FilterMode`
Controls which companies are skipped:
- `"and"` (default, switch **ON**): skip only when BOTH email AND phone exist → scrape if **either** is missing
- `"or"` (switch **OFF**): skip when EITHER email OR phone exists → scrape only if **both** are missing

### `ScrapeRequest` — POST body
```json
{
  "companies": [...],
  "filterMode": "and"
}
```

---

## API Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/scrape` | Synchronous — returns all results at once |
| `POST` | `/api/scrape/stream` | Async SSE stream — results sent as they complete |
| `POST` | `/api/scrape/stop/:sessionId` | Cancel an active stream session |
| `GET` | `/api/scrape/history` | Last 500 scrape results (in-memory) |
| `GET` | `/health` | Health check — returns `"ok"` |

### SSE Event format (`/api/scrape/stream`)
```
data: {"type":"session","sessionId":"<hex>"}      // first event
data: {"type":"total","total":<n>}                 // second event
data: <ScrapeResult JSON>                          // one per company
event: done\ndata: {}                              // final event
```

---

## Configuration (`api/config.json`)

```json
{
  "server": {
    "domain": "",
    "port": 8080,
    "cors": {
      "allowOrigins": "*",
      "allowHeaders": "Origin, Content-Type, Accept, Authorization",
      "allowMethods": "GET, POST, PUT, DELETE, OPTIONS"
    }
  },
  "scraper": {
    "concurrency": 5,
    "requestDelayMs": 2000,
    "randomDelayMs": 1000,
    "retryCount": 2,
    "requestTimeoutSec": 30,
    "searchEngineURL": "https://searx.be",
    "cacheFile": "scraper_cache.json"
  },
  "matcher": {
    "threshold": 0.72
  }
}
```

**Config fields:**
- `concurrency` — max parallel company scrapes
- `requestDelayMs` / `randomDelayMs` — rate limiting delays (ms)
- `retryCount` — HTTP retry attempts on failure
- `requestTimeoutSec` — per-request timeout (seconds)
- `searchEngineURL` — SearXNG instance base URL (not currently wired into GelbeSeitenSource; reserved for future search pipeline)
- `cacheFile` — path to JSON persistence file; empty = in-memory only
- `matcher.threshold` — fuzzy match score cutoff (0.0–1.0)

**Environment variables:**
- `PORT` — overrides server listen port (default: `10000`)

---

## Scraping Pipeline

### Engine (`internal/features/scraper/engine.go`)

**Deduplication:** companies with the same `reName|reOrt` key are scraped once; results are fanned out to all duplicates.

**Concurrency:** semaphore-based worker pool (`cfg.Concurrency` slots).

**FilterMode check:** companies already having both email + phone (or either, depending on mode) are returned immediately with `source: "excel"`.

**`scrapeOne` flow:**
1. Check `shouldSkip()` → return existing data if already complete
2. Iterate `sources` (currently only `GelbeSeitenSource`)
3. First source with any contact data wins; return `ScrapeResult{status: "done"}`
4. If all sources empty → `ScrapeResult{status: "not_found"}`

**Panic recovery:** every goroutine recovers panics and emits an `"error"` result.

**Shared HTTP transport:** `sharedTransport` with 200 max idle conns is used by all Colly collectors to reuse TCP connections.

### `Source` Interface
```go
type Source interface {
    Name() string
    Scrape(company model.Company, cfg Config) (*ContactInfo, error)
}
```

`ContactInfo` fields: `Emails []string`, `Phones []string`, `Source string`, `Website string`

---

## GelbeSeiten Scraper (`internal/features/scraper/gelbeseiten/`)

### Search pipeline (`search.go`)
Single Colly collector at **MaxDepth 2**:

**Stage 1 — Search page** (`depth=1`):
```
GET https://www.gelbeseiten.de/suche/{name-slug}/{city-slug}
```
- Slug built from company name + city: lowercase, umlauts transliterated (`ä→ae`, `ö→oe`, `ü→ue`, `ß→ss`), non-alphanumeric replaced with hyphens
- Selects **first** `article[id^="treffer_"]` → extracts profile URL from `a[href]`

**Stage 2 — Profile page** (`depth=2`):
- Phone: `a[href^="tel:"]` → `ParsePhone()`
- Website: `.contains-icon-big-homepage a` → `ParseWebsite()`
- Email: `#email_versenden[data-link]` → `ParseEmail()` (strips `mailto:` + query string)

Depth guard (`e.Request.Depth != 2`) prevents stage-2 selectors from firing on the search page.

### Parser (`parser.go`)
- `ParsePhone(telHref)` — strips `tel:`, keeps digits and leading `+`, rejects < 7 digits
- `ParseWebsite(href)` — rejects empty, `#`, and `javascript:` URIs
- `ParseEmail(dataLink)` — strips `mailto:`, query string, URL-decodes, validates with regex

### GelbeSeitenSource (`website.go`)
Wraps the gelbeseiten package and adds cache layer:
1. Check `cache.Store` by key `name|city`
2. If miss → call `gelbeseiten.Scrape()`
3. Archive to cache **only if** at least one contact field was found (empty profiles are not cached)
4. Cache is persisted to `scraper_cache.json` asynchronously after each write

---

## Cache (`internal/cache/store.go`)

**Type:** `cache.Store` — thread-safe `map[string]CachedContact` backed by a JSON file.

**Key format:** `"<lowercase name>|<lowercase city>"` (via `cache.BuildKey()`)

**`CachedContact` fields:** `Phones`, `Emails`, `Website`, `Source`, `CachedAt`

**Persistence:** atomic write — marshals to a `.tmp` file, then `os.Rename` to the target.

**Concurrency:** `sync.RWMutex` — reads are non-blocking; writes trigger async file save in a goroutine.

---

## Website Finder (`internal/features/scraper/discover/website_finder.go`)

`WebsiteFinder` finds the official company website from name + city. **Not used in the main scraping pipeline yet** — available for future integration.

**Pipeline:**
1. Clean company name (strip legal suffixes like GmbH, AG, KG)
2. Build search query: `"<clean name> <city> Germany"`
3. Search DuckDuckGo HTML (`scraper/search/duckduckgo.go` — `DDGClient`)
4. Filter results against `directoryDomains` blocklist (40+ domains)
5. Score candidates by keyword overlap with company name
6. Validate top candidates via HTTP HEAD request (accepts 200/3xx/403)
7. Fallback: domain guessing (`search.DomainGuesser`) if DDG returns nothing

---

## Search Backends (`internal/features/search/`)

### `DuckDuckGoSearcher` (`duckduckgo.go`)
- Uses plain `net/http` (not Colly) to avoid Colly's LimitRule timeout interference
- Global semaphore (`ddgSem`, capacity 1) — serialises all DDG requests process-wide
- 2 s minimum delay before each request (held under semaphore)
- Parses DDG HTML via `uddgValueRe` regex to extract `uddg=` encoded URLs
- Returns up to 3 candidate base URLs after directory filtering
- `CleanQuery()` strips quotes, German postal codes, address fragments, trailing punctuation

### `SearXNGSearcher` (`searxng.go`)
- Queries SearXNG JSON API: `GET /search?q=...&format=json&categories=general&language=de`
- Preferred over DDG because SearXNG allows programmatic access by design
- Retry with exponential backoff: 1s → 2s → 4s
- Returns up to 3 candidate base URLs after directory filtering

### `DomainGuesser` (`domain_guess.go`)
Fallback when search engines fail. Generates `.de` domain candidates from company name:
1. First word only (if ≥ 6 chars): `efomed.de`
2. First two words hyphenated: `alves-baumschulen.de`
3. First two words compact: `alvesbaumschulen.de`
4. Full slug (up to 4 words): `alves-baumschulen-hamburg.de`

Validates each candidate with HTTP HEAD → falls back to GET if HEAD fails. Accepts any non-error HTTP response (2xx, 3xx, 4xx all confirm a live server).

---

## Contact Extraction Helpers (`internal/features/scraper/engine.go`)

### Email extraction
- Regex: `[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,24}`
- Normalises obfuscation: `[at]` → `@`, `[dot]` → `.`
- Validates: single `@`, no leading/trailing dots in local part, alphabetic TLD only, no image extensions
- Strips `mailto:`, query params, URL encoding, surrounding punctuation

### Phone extraction
- Regex: `(?:(?:\+49|0049|0)\s*(?:\d[\s\-/.]?){7,13}\d)` (German numbers only)
- Validates: 10–15 digits total, rejects non-German `00` prefixes
- Deduplication via `seen` map

---

## Handler (`handlers/http/handler.go`)

**`Handler` struct:**
- `engine *scraper.Engine`
- `history []model.ScrapeResult` — last 500 results, guarded by `sync.RWMutex`
- `sessions map[string]context.CancelFunc` — active stream sessions, guarded by `sync.Mutex`

**Session management:**
- Each stream gets a random 8-byte hex session ID
- `context.CancelFunc` stored in `sessions` map
- Client can cancel via `POST /api/scrape/stop/:sessionId`
- Session cleaned up automatically when stream ends

**History:** capped at 500 entries (oldest evicted). In-memory only — lost on restart.

---

## Excel File Format

Expected columns (German headers):
- `ID`, `EnObjekt`, `ReName`, `ReName2`, `ObjektRechnung`
- `ReOrt` (City), `ReHausnummer`, `RePlz`, `ReStrasse`, `ReNummer`
- `Email`, `Telefonnummer`

---

## Development Setup

### Prerequisites
- Go 1.24+
- Node.js 18+

### Backend
```bash
cd api
go mod download
go run cmd/server/main.go
```

### Frontend
```bash
cd frontend
npm install
npm run dev
```

Frontend: `http://localhost:3000`
Backend API: `http://localhost:10000`

---

## Error Handling

- Network timeouts → retried up to `retryCount` times
- Panics in goroutines → recovered, emit `status: "error"` result
- Missing search results → `status: "not_found"` (not an error)
- Cache miss + empty profile → not archived, retried on next run
- Config file missing → falls back to defaults (server still starts)

---

## Performance

- Concurrent scraping: semaphore-based pool (`concurrency` workers)
- Shared HTTP transport: connection reuse across all Colly collectors
- Deduplication: identical company+city pairs scraped once, result fanned out
- Cache: eliminates redundant GelbeSeiten requests across runs
- Streaming: SSE sends each result immediately rather than buffering all

---

## Security & Ethics

- Only scrapes publicly accessible German business directories
- Rate limiting + random delays to avoid overloading target sites
- Data used for legitimate property management contact resolution
- No authentication scraping; no private data targeted

---

## Adding a New Scraper Source

1. Create `internal/features/scraper/<sourcename>/` package
2. Implement `Scrape(company model.Company, cfg Config) (*ContactInfo, error)`
3. Create a wrapper type implementing the `Source` interface (like `GelbeSeitenSource`)
4. Register it in `engine.go` `New()` function inside the `sources` slice
5. The engine will automatically try it as a fallback if previous sources return no data

---

## Deployment

- Binary: `go build -o server ./cmd/server`
- Port: set `PORT` env var
- Cache file: set `scraper.cacheFile` in `config.json` to a writable path
- CORS: configure `server.cors.allowOrigins` in `config.json` for production
- Reverse proxy: Nginx recommended for SSL and port 443 forwarding
