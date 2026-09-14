package crawler

import (
	"encoding/json"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/config"
)

// A session stored before a crawler setting existed has no key for it in its
// saved config. Restoring that session must leave the setting at the running
// configuration's value rather than at the zero value, or every resumed or
// retried crawl silently loses a feature that defaults to on.
func TestRestoreSavedCrawlerConfigKeepsSettingsAbsentFromTheSnapshot(t *testing.T) {
	// The shape a session saved before store_link_position existed has.
	const savedBeforeUpgrade = `{"Crawler":{"Workers":7,"UserAgent":"OldBot/1.0","RespectRobots":true}}`

	running := config.Config{Crawler: config.CrawlerConfig{
		Workers:           10,
		UserAgent:         "CrawlObserver/1.0",
		RespectRobots:     true,
		StoreLinkPosition: true,
	}}

	cfg := running
	saved, err := decodeSavedConfig(savedBeforeUpgrade, &running)
	if err != nil {
		t.Fatalf("decodeSavedConfig() error = %v", err)
	}
	applySavedCrawlerConfig(&cfg, saved.Crawler)

	if !cfg.Crawler.StoreLinkPosition {
		t.Error("StoreLinkPosition = false, want the running configuration's true")
	}
	// What the snapshot does carry must still win.
	if cfg.Crawler.Workers != 7 {
		t.Errorf("Workers = %d, want the saved 7", cfg.Crawler.Workers)
	}
	if cfg.Crawler.UserAgent != "OldBot/1.0" {
		t.Errorf("UserAgent = %q, want the saved %q", cfg.Crawler.UserAgent, "OldBot/1.0")
	}
}

// A session saved after the upgrade carries the setting explicitly, and an
// explicit false must survive the restore.
func TestRestoreSavedCrawlerConfigKeepsAnExplicitFalse(t *testing.T) {
	running := config.Config{Crawler: config.CrawlerConfig{StoreLinkPosition: true}}
	snapshot, err := json.Marshal(struct{ Crawler config.CrawlerConfig }{
		Crawler: config.CrawlerConfig{Workers: 4, StoreLinkPosition: false},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	cfg := running
	saved, err := decodeSavedConfig(string(snapshot), &running)
	if err != nil {
		t.Fatalf("decodeSavedConfig() error = %v", err)
	}
	applySavedCrawlerConfig(&cfg, saved.Crawler)

	if cfg.Crawler.StoreLinkPosition {
		t.Error("StoreLinkPosition = true, want the saved false")
	}
}
