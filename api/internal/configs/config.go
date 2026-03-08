package configs

import (
	"encoding/json"
	"os"
	"time"

	"github.com/ahmet4dev/gol-lib/server"
)

type Config struct {
	Server  server.Config `json:"server"`
	Scraper ScraperConfig `json:"scraper"`
	Matcher MatcherConfig `json:"matcher"`
}

type ScraperConfig struct {
	Concurrency       int    `json:"concurrency"`
	RequestDelayMs    int    `json:"requestDelayMs"`
	RandomDelayMs     int    `json:"randomDelayMs"`
	RetryCount        int    `json:"retryCount"`
	RequestTimeoutSec int    `json:"requestTimeoutSec"`
	SearchEngineURL   string `json:"searchEngineURL"`
	// CacheFile is the path to the JSON file used to persist scrape results
	// across server restarts.  Set to "" to disable file persistence.
	CacheFile string `json:"cacheFile"`
}

type MatcherConfig struct {
	Threshold float64 `json:"threshold"`
}

// Defaults returns safe fallback values.
func Defaults() Config {
	return Config{
		Server: server.Config{
			Domain: "",
			Port:   8080,
		},
		Scraper: ScraperConfig{
			Concurrency:       5,
			RequestDelayMs:    2000,
			RandomDelayMs:     1000,
			RetryCount:        3,
			RequestTimeoutSec: 60,
			CacheFile:         "scraper_cache.json",
		},
		Matcher: MatcherConfig{Threshold: 0.55},
	}
}

// Load reads config.json from the given path, falling back to defaults.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &cfg, nil
		}
		return nil, err
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// RequestDelay converts the millisecond value to time.Duration.
func (s ScraperConfig) RequestDelay() time.Duration {
	return time.Duration(s.RequestDelayMs) * time.Millisecond
}

// RandomDelay converts the millisecond value to time.Duration.
func (s ScraperConfig) RandomDelay() time.Duration {
	return time.Duration(s.RandomDelayMs) * time.Millisecond
}

// RequestTimeout converts the second value to time.Duration.
func (s ScraperConfig) RequestTimeout() time.Duration {
	return time.Duration(s.RequestTimeoutSec) * time.Second
}
