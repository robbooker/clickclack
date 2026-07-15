package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const (
	maxLinkPreviewURLBytes  = 2048
	maxLinkPreviewHTMLBytes = 1 << 20
	linkPreviewTimeout      = 8 * time.Second
)

var (
	errInvalidLinkPreviewURL  = errors.New("invalid link preview URL")
	errLinkPreviewUnavailable = errors.New("link preview unavailable")
	blockedPreviewPrefixes    = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
)

type linkPreview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	SiteName    string `json:"site_name"`
	ImageURL    string `json:"image_url,omitempty"`
}

type linkPreviewCacheEntry struct {
	Preview   linkPreview
	ExpiresAt time.Time
	CreatedAt time.Time
}

type linkPreviewCache struct {
	mu       sync.Mutex
	items    map[string]linkPreviewCacheEntry
	maxItems int
	ttl      time.Duration
}

func newLinkPreviewCache(maxItems int, ttl time.Duration) *linkPreviewCache {
	return &linkPreviewCache{
		items:    make(map[string]linkPreviewCacheEntry),
		maxItems: maxItems,
		ttl:      ttl,
	}
}

func (c *linkPreviewCache) get(key string, now time.Time) (linkPreview, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return linkPreview{}, false
	}
	if !now.Before(entry.ExpiresAt) {
		delete(c.items, key)
		return linkPreview{}, false
	}
	return entry.Preview, true
}

func (c *linkPreviewCache) set(key string, preview linkPreview, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for candidate, entry := range c.items {
		if !now.Before(entry.ExpiresAt) {
			delete(c.items, candidate)
		}
	}
	if c.maxItems > 0 && len(c.items) >= c.maxItems {
		var oldestKey string
		var oldestTime time.Time
		for candidate, entry := range c.items {
			if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
				oldestKey = candidate
				oldestTime = entry.CreatedAt
			}
		}
		delete(c.items, oldestKey)
	}
	c.items[key] = linkPreviewCacheEntry{
		Preview:   preview,
		CreatedAt: now,
		ExpiresAt: now.Add(c.ttl),
	}
}

func (s *Server) getLinkPreview(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("messages:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	rawURL, err := normalizeLinkPreviewURL(r.URL.Query().Get("url"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if preview, ok := s.linkPreviews.get(rawURL, time.Now()); ok {
		writeJSON(w, http.StatusOK, map[string]any{"preview": preview})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), linkPreviewTimeout)
	defer cancel()
	preview, err := s.previewFetcher(ctx, rawURL)
	if err != nil {
		if errors.Is(err, errInvalidLinkPreviewURL) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.linkPreviews.set(rawURL, preview, time.Now())
	w.Header().Set("Cache-Control", "private, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{"preview": preview})
}

func fetchLinkPreview(ctx context.Context, rawURL string) (linkPreview, error) {
	parsed, err := validateLinkPreviewURL(ctx, rawURL)
	if err != nil {
		return linkPreview{}, err
	}
	client := &http.Client{
		Timeout: linkPreviewTimeout,
		Transport: &http.Transport{
			DialContext:           dialPublicLinkPreviewAddress,
			DisableCompression:    false,
			ForceAttemptHTTP2:     true,
			IdleConnTimeout:       30 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("too many link preview redirects")
			}
			_, err := validateLinkPreviewURL(req.Context(), req.URL.String())
			return err
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return linkPreview{}, errInvalidLinkPreviewURL
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9")
	req.Header.Set("User-Agent", "ClickClack-LinkPreview/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return linkPreview{}, errLinkPreviewUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return linkPreview{}, errLinkPreviewUnavailable
	}
	if contentLength := resp.ContentLength; contentLength > maxLinkPreviewHTMLBytes {
		return linkPreview{}, errLinkPreviewUnavailable
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, parseErr := mime.ParseMediaType(contentType)
		if parseErr != nil || (mediaType != "text/html" && mediaType != "application/xhtml+xml") {
			return linkPreview{}, errLinkPreviewUnavailable
		}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLinkPreviewHTMLBytes+1))
	if err != nil || len(body) > maxLinkPreviewHTMLBytes {
		return linkPreview{}, errLinkPreviewUnavailable
	}
	preview, err := parseLinkPreviewHTML(body, resp.Request.URL)
	if err != nil {
		return linkPreview{}, err
	}
	if preview.ImageURL != "" {
		if _, err := validateLinkPreviewURL(ctx, preview.ImageURL); err != nil {
			preview.ImageURL = ""
		}
	}
	return preview, nil
}

func parseLinkPreviewHTML(body []byte, pageURL *url.URL) (linkPreview, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return linkPreview{}, errLinkPreviewUnavailable
	}
	metadata := make(map[string]string)
	var documentTitle string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "meta":
				var key, value string
				for _, attribute := range node.Attr {
					switch strings.ToLower(attribute.Key) {
					case "property", "name":
						key = strings.ToLower(strings.TrimSpace(attribute.Val))
					case "content":
						value = strings.TrimSpace(attribute.Val)
					}
				}
				if key != "" && value != "" {
					if _, exists := metadata[key]; !exists {
						metadata[key] = value
					}
				}
			case "title":
				if documentTitle == "" {
					documentTitle = strings.TrimSpace(nodeText(node))
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	title := firstMetadata(metadata, "og:title", "twitter:title")
	if title == "" {
		title = documentTitle
	}
	description := firstMetadata(metadata, "og:description", "twitter:description", "description")
	imageURL := firstMetadata(metadata, "og:image:secure_url", "og:image", "twitter:image")
	if imageURL != "" {
		image, parseErr := url.Parse(imageURL)
		if parseErr != nil {
			imageURL = ""
		} else {
			imageURL = pageURL.ResolveReference(image).String()
		}
	}
	siteName := firstMetadata(metadata, "og:site_name")
	if siteName == "" {
		siteName = strings.TrimPrefix(strings.ToLower(pageURL.Hostname()), "www.")
	}
	if title == "" && description == "" && imageURL == "" {
		return linkPreview{}, errLinkPreviewUnavailable
	}
	return linkPreview{
		URL:         pageURL.String(),
		Title:       truncatePreviewText(title, 300),
		Description: truncatePreviewText(description, 500),
		SiteName:    truncatePreviewText(siteName, 100),
		ImageURL:    imageURL,
	}, nil
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(candidate *html.Node) {
		if candidate.Type == html.TextNode {
			builder.WriteString(candidate.Data)
			builder.WriteByte(' ')
		}
		for child := candidate.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func firstMetadata(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func truncatePreviewText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.ToValidUTF8(value, "")), " ")
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes-1])) + "…"
}

func normalizeLinkPreviewURL(rawURL string) (string, error) {
	if len(rawURL) == 0 || len(rawURL) > maxLinkPreviewURLBytes {
		return "", errInvalidLinkPreviewURL
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return "", errInvalidLinkPreviewURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errInvalidLinkPreviewURL
	}
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		return "", errInvalidLinkPreviewURL
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func validateLinkPreviewURL(ctx context.Context, rawURL string) (*url.URL, error) {
	normalized, err := normalizeLinkPreviewURL(rawURL)
	if err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(normalized)
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") ||
		strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".internal") ||
		strings.HasSuffix(hostname, ".home.arpa") {
		return nil, errInvalidLinkPreviewURL
	}
	if _, err := resolvePublicLinkPreviewIPs(ctx, hostname); err != nil {
		return nil, err
	}
	return parsed, nil
}

func dialPublicLinkPreviewAddress(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errInvalidLinkPreviewURL
	}
	if port != "80" && port != "443" {
		return nil, errInvalidLinkPreviewURL
	}
	addresses, err := resolvePublicLinkPreviewIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
}

func resolvePublicLinkPreviewIPs(ctx context.Context, hostname string) ([]netip.Addr, error) {
	if direct, err := netip.ParseAddr(hostname); err == nil {
		direct = direct.Unmap()
		if !isPublicLinkPreviewIP(direct) {
			return nil, errInvalidLinkPreviewURL
		}
		return []netip.Addr{direct}, nil
	}
	resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", hostname)
	if err != nil || len(resolved) == 0 {
		return nil, errLinkPreviewUnavailable
	}
	addresses := make([]netip.Addr, 0, len(resolved))
	for _, address := range resolved {
		address = address.Unmap()
		if !isPublicLinkPreviewIP(address) {
			return nil, errInvalidLinkPreviewURL
		}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

func isPublicLinkPreviewIP(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() ||
		address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedPreviewPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}
