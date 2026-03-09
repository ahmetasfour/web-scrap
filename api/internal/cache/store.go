// Package cache provides a thread-safe in-memory contact cache with optional
// JSON file persistence so that previously scraped results survive server
// restarts and avoid redundant GelbeSeiten requests.
//
// Two maps are maintained:
//   - Found    – companies where at least one contact field was scraped
//   - NotFound – companies that returned no results on GelbeSeiten
//
// Both are persisted in the same JSON file so the next run can skip
// requests for companies already known to be absent on GelbeSeiten.
package cache

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ahmet4dev/gol-lib/logging"
	"go.uber.org/zap"
)

// CachedContact holds the scraped contact data stored for a single company.
type CachedContact struct {
	Phones   []string  `json:"phones"`
	Emails   []string  `json:"emails"`
	Website  string    `json:"website"`
	Source   string    `json:"source"`
	CachedAt time.Time `json:"cachedAt"`
}

// CachedNotFound marks a company that returned no results on GelbeSeiten.
type CachedNotFound struct {
	Source   string    `json:"source"`
	CachedAt time.Time `json:"cachedAt"`
}

// fileSnapshot is the JSON structure written to disk.
type fileSnapshot struct {
	Found    map[string]CachedContact  `json:"found"`
	NotFound map[string]CachedNotFound `json:"notFound"`
}

// Store is a concurrency-safe in-memory cache backed by a JSON file.
//
// All public methods are safe for simultaneous use from multiple goroutines.
type Store struct {
	mu       sync.RWMutex
	found    map[string]CachedContact
	notFound map[string]CachedNotFound
	filePath string
}

// New creates a Store and loads any previously persisted data from filePath.
// If filePath is empty, persistence is disabled (in-memory only).
func New(filePath string) *Store {
	s := &Store{
		found:    make(map[string]CachedContact),
		notFound: make(map[string]CachedNotFound),
		filePath: filePath,
	}
	if filePath != "" {
		if err := s.load(); err != nil {
			if !os.IsNotExist(err) {
				logging.Logger.Warn("cache load failed — starting empty",
					zap.String("file", filePath),
					zap.Error(err),
				)
			}
		} else {
			logging.Logger.Info("cache loaded from disk",
				zap.String("file", filePath),
				zap.Int("found", len(s.found)),
				zap.Int("notFound", len(s.notFound)),
			)
		}
	}
	return s
}

// BuildKey normalises company name and city into a canonical cache key.
//
// Format: "<lowercase-name>|<lowercase-city>"
//
// Example: "Stolle GmbH", "Bleckede" → "stolle gmbh|bleckede"
func BuildKey(name, city string) string {
	return strings.ToLower(strings.TrimSpace(name)) +
		"|" +
		strings.ToLower(strings.TrimSpace(city))
}

// Get returns the cached contact for key and true when a found-hit exists.
func (s *Store) Get(key string) (CachedContact, bool) {
	s.mu.RLock()
	c, ok := s.found[key]
	s.mu.RUnlock()
	return c, ok
}

// IsNotFound returns true when the key is stored in the not-found table.
func (s *Store) IsNotFound(key string) bool {
	s.mu.RLock()
	_, ok := s.notFound[key]
	s.mu.RUnlock()
	return ok
}

// Set stores contact under key and asynchronously persists the cache to disk.
func (s *Store) Set(key string, c CachedContact) {
	c.CachedAt = time.Now()

	s.mu.Lock()
	s.found[key] = c
	s.mu.Unlock()

	s.persistAsync()
}

// SetNotFound marks key as a company that was not found on GelbeSeiten
// and asynchronously persists the cache to disk.
func (s *Store) SetNotFound(key, source string) {
	s.mu.Lock()
	s.notFound[key] = CachedNotFound{Source: source, CachedAt: time.Now()}
	s.mu.Unlock()

	s.persistAsync()
}

// Len returns the number of found entries currently in the cache.
func (s *Store) Len() int {
	s.mu.RLock()
	n := len(s.found)
	s.mu.RUnlock()
	return n
}

// LenNotFound returns the number of not-found entries in the cache.
func (s *Store) LenNotFound() int {
	s.mu.RLock()
	n := len(s.notFound)
	s.mu.RUnlock()
	return n
}

func (s *Store) persistAsync() {
	if s.filePath == "" {
		return
	}
	go func() {
		if err := s.save(); err != nil {
			logging.Logger.Warn("cache persist failed",
				zap.String("file", s.filePath),
				zap.Error(err),
			)
		}
	}()
}

// load reads the JSON file at s.filePath and populates the store.
// Must only be called during initialisation (no lock needed yet).
func (s *Store) load() error {
	f, err := os.Open(s.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var snap fileSnapshot
	if err := json.NewDecoder(f).Decode(&snap); err != nil {
		return s.loadLegacy(f)
	}

	// Accept both new format (snap.Found != nil) and legacy flat format.
	if snap.Found != nil {
		s.found = snap.Found
		if snap.NotFound != nil {
			s.notFound = snap.NotFound
		}
		return nil
	}

	// Legacy file: the JSON root is a flat map[string]CachedContact.
	// Rewind and decode as the old format.
	return s.loadLegacy(f)
}

// loadLegacy handles old scraper_cache.json files that were a flat
// map[string]CachedContact without a "found" / "notFound" wrapper.
func (s *Store) loadLegacy(f *os.File) error {
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	var legacy map[string]CachedContact
	if err := json.NewDecoder(f).Decode(&legacy); err != nil {
		return err
	}
	s.found = legacy
	return nil
}

// save serialises the current cache to the JSON file atomically.
func (s *Store) save() error {
	s.mu.RLock()
	snap := fileSnapshot{
		Found:    make(map[string]CachedContact, len(s.found)),
		NotFound: make(map[string]CachedNotFound, len(s.notFound)),
	}
	for k, v := range s.found {
		snap.Found[k] = v
	}
	for k, v := range s.notFound {
		snap.NotFound[k] = v
	}
	s.mu.RUnlock()

	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	// Write to a temp file then rename for atomic replacement.
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}
