package config

import (
	"strings"
	"testing"
)

// Headers are free text a person typed, so they are dropped from the session
// snapshot rather than trusted to the key-name rules: a header named
// Authorization matches none of them.
func TestSessionConfigJSON_DropsRequestHeaders(t *testing.T) {
	cfg := &Config{}
	cfg.Crawler.UserAgent = "TestBot/1.0"
	cfg.Crawler.Headers = map[string]string{
		"Signature-Agent": `"https://asight.fr/"`,
		"Authorization":   "Bearer a-real-credential",
	}

	got, err := SessionConfigJSON(cfg)
	if err != nil {
		t.Fatalf("SessionConfigJSON: %v", err)
	}
	for _, unwanted := range []string{"Signature-Agent", "Authorization", "a-real-credential"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("session config contains %q, want it dropped:\n%s", unwanted, got)
		}
	}
	// The rest of the crawler config still has to be there.
	if !strings.Contains(got, "TestBot/1.0") {
		t.Errorf("session config lost the user agent:\n%s", got)
	}
}
