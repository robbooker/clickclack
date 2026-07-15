package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openclaw/clickclack/apps/api/internal/realtime"
	sqlitestore "github.com/openclaw/clickclack/apps/api/internal/store/sqlite"
)

func TestParseLinkPreviewHTML(t *testing.T) {
	t.Parallel()
	pageURL, err := url.Parse("https://example.com/chart/42")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := parseLinkPreviewHTML([]byte(`
<!doctype html>
<html>
  <head>
    <title>Fallback title</title>
    <meta name="description" content="Fallback description">
    <meta property="og:title" content="Chart &amp; signals">
    <meta property="og:description" content="The useful description">
    <meta property="og:site_name" content="Example Charts">
    <meta property="og:image" content="/images/chart.png">
  </head>
</html>`), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Title != "Chart & signals" || preview.Description != "The useful description" {
		t.Fatalf("unexpected preview text: %#v", preview)
	}
	if preview.SiteName != "Example Charts" || preview.ImageURL != "https://example.com/images/chart.png" {
		t.Fatalf("unexpected preview metadata: %#v", preview)
	}
}

func TestLinkPreviewURLValidation(t *testing.T) {
	t.Parallel()
	for _, candidate := range []string{
		"",
		"file:///etc/passwd",
		"https://user:pass@example.com/",
		"https://example.com:8443/",
	} {
		if _, err := normalizeLinkPreviewURL(candidate); err == nil {
			t.Fatalf("expected %q to be rejected", candidate)
		}
	}
	for _, candidate := range []string{
		"http://127.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.4/",
		"http://[::1]/",
		"http://localhost/",
	} {
		if _, err := validateLinkPreviewURL(context.Background(), candidate); err == nil {
			t.Fatalf("expected private target %q to be rejected", candidate)
		}
	}
	if normalized, err := normalizeLinkPreviewURL("https://example.com/path#section"); err != nil || normalized != "https://example.com/path" {
		t.Fatalf("unexpected normalized URL %q: %v", normalized, err)
	}
}

func TestLinkPreviewEndpointCachesResults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	st, err := sqlitestore.Open("sqlite://" + filepath.Join(dataDir, "clickclack.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com"); err != nil {
		t.Fatal(err)
	}

	srv := New(st, realtime.NewHub(), Options{})
	var calls atomic.Int32
	srv.previewFetcher = func(_ context.Context, rawURL string) (linkPreview, error) {
		calls.Add(1)
		return linkPreview{
			URL:      rawURL,
			Title:    "Cached preview",
			SiteName: "example.com",
		}, nil
	}
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)

	endpoint := server.URL + "/api/link-preview?url=" + url.QueryEscape("https://example.com/article#fragment")
	for range 2 {
		response, err := http.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Preview linkPreview `json:"preview"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || decodeErr != nil {
			t.Fatalf("unexpected response status=%d decode=%v", response.StatusCode, decodeErr)
		}
		if body.Preview.Title != "Cached preview" || strings.Contains(body.Preview.URL, "#") {
			t.Fatalf("unexpected preview response: %#v", body.Preview)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one fetch, got %d", calls.Load())
	}
}

func TestLinkPreviewCacheEvictsOldest(t *testing.T) {
	t.Parallel()
	cache := newLinkPreviewCache(1, time.Hour)
	now := time.Now()
	cache.set("first", linkPreview{Title: "First"}, now)
	cache.set("second", linkPreview{Title: "Second"}, now.Add(time.Second))
	if _, ok := cache.get("first", now.Add(2*time.Second)); ok {
		t.Fatal("expected oldest cache entry to be evicted")
	}
	if second, ok := cache.get("second", now.Add(2*time.Second)); !ok || second.Title != "Second" {
		t.Fatalf("expected second cache entry, got %#v, ok=%v", second, ok)
	}
}
