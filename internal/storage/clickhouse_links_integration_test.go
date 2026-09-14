//go:build integration

package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestLinkPositionRoundTrip checks that where a link sits in its page survives a
// trip through ClickHouse, on every read path that returns links.
func TestLinkPositionRoundTrip(t *testing.T) {
	s := testStore(t)
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	sessionID := uuid.New().String()
	now := time.Now()
	t.Cleanup(func() { cleanupSession(t, s, sessionID) })

	const source = "https://example.com/page"
	menu := LinkRow{
		CrawlSessionID: sessionID, SourceURL: source, TargetURL: "https://example.com/products",
		AnchorText: "Products", IsInternal: true, Tag: "a",
		Landmark: "nav", XPath: "/html/body/nav/ul/li[1]/a",
		Depth: 5, DocumentIndex: 0, BlockSignature: 1234567890123456789,
		CrawledAt: now,
	}
	external := LinkRow{
		CrawlSessionID: sessionID, SourceURL: source, TargetURL: "https://elsewhere.example/ref",
		AnchorText: "Elsewhere", IsInternal: false, Tag: "a",
		Landmark: "main", XPath: "/html/body/main/p/a",
		Depth: 4, DocumentIndex: 1, BlockSignature: 987654321098765432,
		CrawledAt: now,
	}
	// A crawl run with store_link_position off writes links with no position.
	bare := LinkRow{
		CrawlSessionID: sessionID, SourceURL: source, TargetURL: "https://example.com/plain",
		AnchorText: "Plain", IsInternal: true, Tag: "a", CrawledAt: now,
	}

	if err := s.InsertLinks(ctx, []LinkRow{menu, external, bare}); err != nil {
		t.Fatalf("inserting links: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	assertSame := func(t *testing.T, path string, got, want LinkRow) {
		t.Helper()
		if got.Landmark != want.Landmark {
			t.Errorf("%s: Landmark = %q, want %q", path, got.Landmark, want.Landmark)
		}
		if got.XPath != want.XPath {
			t.Errorf("%s: XPath = %q, want %q", path, got.XPath, want.XPath)
		}
		if got.Depth != want.Depth {
			t.Errorf("%s: Depth = %d, want %d", path, got.Depth, want.Depth)
		}
		if got.DocumentIndex != want.DocumentIndex {
			t.Errorf("%s: DocumentIndex = %d, want %d", path, got.DocumentIndex, want.DocumentIndex)
		}
		if got.BlockSignature != want.BlockSignature {
			t.Errorf("%s: BlockSignature = %d, want %d", path, got.BlockSignature, want.BlockSignature)
		}
	}

	find := func(t *testing.T, rows []LinkRow, target string) LinkRow {
		t.Helper()
		for _, l := range rows {
			if l.TargetURL == target {
				return l
			}
		}
		t.Fatalf("no link to %q among %d rows", target, len(rows))
		return LinkRow{}
	}

	internalRows, err := s.InternalLinksPaginated(ctx, sessionID, 100, 0, nil, nil)
	if err != nil {
		t.Fatalf("InternalLinksPaginated: %v", err)
	}
	assertSame(t, "InternalLinksPaginated", find(t, internalRows, menu.TargetURL), menu)
	assertSame(t, "InternalLinksPaginated (no position)", find(t, internalRows, bare.TargetURL), bare)

	externalRows, err := s.ExternalLinksPaginated(ctx, sessionID, 100, 0, nil, nil)
	if err != nil {
		t.Fatalf("ExternalLinksPaginated: %v", err)
	}
	assertSame(t, "ExternalLinksPaginated", find(t, externalRows, external.TargetURL), external)

	allExternal, err := s.ExternalLinks(ctx, sessionID)
	if err != nil {
		t.Fatalf("ExternalLinks: %v", err)
	}
	assertSame(t, "ExternalLinks", find(t, allExternal, external.TargetURL), external)

	pageLinks, err := s.GetPageLinks(ctx, sessionID, source, 100, 0, 100, 0)
	if err != nil {
		t.Fatalf("GetPageLinks: %v", err)
	}
	assertSame(t, "GetPageLinks out", find(t, pageLinks.OutLinks, menu.TargetURL), menu)

	inbound, err := s.GetPageLinks(ctx, sessionID, menu.TargetURL, 100, 0, 100, 0)
	if err != nil {
		t.Fatalf("GetPageLinks inbound: %v", err)
	}
	assertSame(t, "GetPageLinks in", find(t, inbound.InLinks, menu.TargetURL), menu)
}

// TestLinkPositionFilterAndSort checks that the new columns are usable from the
// API's filter and sort parameters, not just readable.
func TestLinkPositionFilterAndSort(t *testing.T) {
	s := testStore(t)
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	sessionID := uuid.New().String()
	now := time.Now()
	t.Cleanup(func() { cleanupSession(t, s, sessionID) })

	rows := []LinkRow{
		{CrawlSessionID: sessionID, SourceURL: "https://example.com/a", TargetURL: "https://example.com/menu",
			IsInternal: true, Landmark: "nav", XPath: "/html/body/nav/a", Depth: 3, DocumentIndex: 0, CrawledAt: now},
		{CrawlSessionID: sessionID, SourceURL: "https://example.com/a", TargetURL: "https://example.com/body",
			IsInternal: true, Landmark: "main", XPath: "/html/body/main/p/a", Depth: 4, DocumentIndex: 1, CrawledAt: now},
	}
	if err := s.InsertLinks(ctx, rows); err != nil {
		t.Fatalf("inserting links: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	filters := []ParsedFilter{{Def: LinkFilters["landmark"], Value: "nav"}}
	got, err := s.InternalLinksPaginated(ctx, sessionID, 100, 0, filters, nil)
	if err != nil {
		t.Fatalf("InternalLinksPaginated: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("landmark filter returned %d rows, want 1: %+v", len(got), got)
	}
	if got[0].TargetURL != "https://example.com/menu" {
		t.Errorf("landmark filter returned %q, want the nav link", got[0].TargetURL)
	}

	// Both directions are checked with the shallow link sorted last by the
	// default order, so that a sort silently falling back to that default
	// cannot satisfy the expectation.
	for _, tc := range []struct {
		order string
		first string
	}{
		{"asc", "https://example.com/menu"},
		{"desc", "https://example.com/body"},
	} {
		sort := ParseSort("depth", tc.order, LinkSortColumns)
		if sort == nil {
			t.Fatalf("depth is not an accepted sort column")
		}
		sorted, err := s.InternalLinksPaginated(ctx, sessionID, 100, 0, nil, sort)
		if err != nil {
			t.Fatalf("InternalLinksPaginated sorted %s: %v", tc.order, err)
		}
		if len(sorted) != 2 {
			t.Fatalf("depth %s: expected 2 rows, got %d", tc.order, len(sorted))
		}
		if sorted[0].TargetURL != tc.first {
			t.Errorf("depth %s: first row = %q, want %q", tc.order, sorted[0].TargetURL, tc.first)
		}
	}
}

// TestExportSessionCarriesLinkPosition reads the links back out through the
// session export. That query scans into its own struct, so a column listed in
// one order and scanned in another would swap values without any error.
func TestExportSessionCarriesLinkPosition(t *testing.T) {
	s := testStore(t)
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	sessionID := uuid.New().String()
	t.Cleanup(func() { cleanupSession(t, s, sessionID) })

	want := LinkRow{
		CrawlSessionID: sessionID,
		SourceURL:      "https://example.com/page",
		TargetURL:      "https://example.com/products",
		AnchorText:     "Products",
		IsInternal:     true,
		Tag:            "a",
		// Deliberately all different, so that any swap between them shows.
		Landmark:       "footer",
		XPath:          "/html/body/footer/ul/li[3]/a",
		Depth:          6,
		DocumentIndex:  41,
		BlockSignature: 1234567890123456789,
		CrawledAt:      time.Now(),
	}
	if err := s.InsertSession(ctx, &CrawlSession{
		ID:        sessionID,
		StartedAt: time.Now(),
		Status:    "completed",
		SeedURLs:  []string{"https://example.com/"},
		UserAgent: "TestBot/1.0",
	}); err != nil {
		t.Fatalf("inserting session: %v", err)
	}
	if err := s.InsertLinks(ctx, []LinkRow{want}); err != nil {
		t.Fatalf("inserting link: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	var buf bytes.Buffer
	if err := s.ExportSession(ctx, sessionID, &buf, false); err != nil {
		t.Fatalf("ExportSession: %v", err)
	}

	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("opening the export: %v", err)
	}
	defer gz.Close()

	var got *exportLink
	dec := json.NewDecoder(gz)
	for {
		var rec exportRecord
		if err := dec.Decode(&rec); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("reading the export: %v", err)
		}
		if rec.Type != RecordLink {
			continue
		}
		var l exportLink
		if err := json.Unmarshal(rec.Data, &l); err != nil {
			t.Fatalf("decoding exported link: %v", err)
		}
		if l.TargetURL == want.TargetURL {
			got = &l
			break
		}
	}
	if got == nil {
		t.Fatalf("the export has no link record for %s", want.TargetURL)
	}

	if got.Landmark != want.Landmark {
		t.Errorf("Landmark = %q, want %q", got.Landmark, want.Landmark)
	}
	if got.XPath != want.XPath {
		t.Errorf("XPath = %q, want %q", got.XPath, want.XPath)
	}
	if got.Depth != want.Depth {
		t.Errorf("Depth = %d, want %d", got.Depth, want.Depth)
	}
	if got.DocumentIndex != want.DocumentIndex {
		t.Errorf("DocumentIndex = %d, want %d", got.DocumentIndex, want.DocumentIndex)
	}
	if got.BlockSignature != want.BlockSignature {
		t.Errorf("BlockSignature = %d, want %d", got.BlockSignature, want.BlockSignature)
	}
}
