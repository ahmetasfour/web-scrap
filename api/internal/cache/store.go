// Package cache provides a thread-safe in-memory contact cache with optional
// JSON file persistence so that previously scraped results survive server
// restarts and avoid redundant GelbeSeiten requests.
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

// Store is a concurrency-safe in-memory cache backed by a JSON file.
//
// All public methods are safe for simultaneous use from multiple goroutines.
type Store struct {
	mu       sync.RWMutex
	data     map[string]CachedContact
	filePath string
}

// New creates a Store and loads any previously persisted data from filePath.
// If filePath is empty, persistence is disabled (in-memory only).
func New(filePath string) *Store {
	s := &Store{
		data:     make(map[string]CachedContact),
		filePath: filePath,
	}
	if filePath != "" {
		if err := s.load(); err != nil {
			// A missing file is normal on first run — log only real errors.
			if !os.IsNotExist(err) {
				logging.Logger.Warn("cache load failed — starting empty",
					zap.String("file", filePath),
					zap.Error(err),
				)
			}
		} else {
			logging.Logger.Info("cache loaded from disk",
				zap.String("file", filePath),
				zap.Int("entries", len(s.data)),
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

// Get returns the cached contact for key and true when a hit exists.
func (s *Store) Get(key string) (CachedContact, bool) {
	s.mu.RLock()
	c, ok := s.data[key]
	s.mu.RUnlock()
	return c, ok
}

// Set stores contact under key and asynchronously persists the cache to disk.
func (s *Store) Set(key string, c CachedContact) {
	c.CachedAt = time.Now()

	s.mu.Lock()
	s.data[key] = c
	s.mu.Unlock()

	// Persist in a goroutine so Set never blocks the scraping pipeline.
	if s.filePath != "" {
		go func() {
			if err := s.save(); err != nil {
				logging.Logger.Warn("cache persist failed",
					zap.String("file", s.filePath),
					zap.Error(err),
				)
			}
		}()
	}
}

// Len returns the number of entries currently in the cache.
func (s *Store) Len() int {
	s.mu.RLock()
	n := len(s.data)
	s.mu.RUnlock()
	return n
}

// load reads the JSON file at s.filePath and populates s.data.
// Must only be called during initialisation (no lock needed yet).
func (s *Store) load() error {
	f, err := os.Open(s.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var snapshot map[string]CachedContact
	if err := json.NewDecoder(f).Decode(&snapshot); err != nil {
		return err
	}
	s.data = snapshot
	return nil
}

// save serialises the current cache to the JSON file.
// It copies the map under a read lock, then writes without holding the lock
// so concurrent reads/writes are not blocked during I/O.
func (s *Store) save() error {
	// Snapshot under read lock.
	s.mu.RLock()
	snapshot := make(map[string]CachedContact, len(s.data))
	for k, v := range s.data {
		snapshot[k] = v
	}
	s.mu.RUnlock()

	b, err := json.MarshalIndent(snapshot, "", "  ")
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
