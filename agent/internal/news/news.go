// Package news reads the small public news file of RaiseSteady Guard
// (P2-252). It is read only when a logged-in parent opens the parent page and
// has turned news on; never on a child screen and never on a timer. Nothing
// about the family is sent: no cookies, no identifiers, no query string.
package news

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"raisesteady-guard/agent/internal/core"
)

// feedURL is the only address read. It is not exported and not configurable;
// tests in this module change it to a local test server.
var feedURL = "https://raisesteady.com/guard/news.json"

const (
	MaxBytes = 32 << 10        // largest feed accepted
	TTL      = 6 * time.Hour   // cache lifetime
	Timeout  = 3 * time.Second // whole request
	MaxItems = 5               // items kept, newest first
	MaxTitle = 80              // characters
	MaxBody  = 600             // characters
	retry    = 10 * time.Minute
)

// Langs are the languages kept from a feed; others are ignored.
var Langs = []string{"en", "ka", "ru"}

var dateRe = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// Item is one validated note.
type Item struct {
	ID    string            `json:"id"`
	Date  string            `json:"date"`
	Until string            `json:"until,omitempty"`
	Title map[string]string `json:"title"`
	Body  map[string]string `json:"body"`
	Link  string            `json:"link,omitempty"`
}

// TitleIn returns the title in lang, or in English.
func (it Item) TitleIn(lang string) string { return pick(it.Title, lang) }

// BodyIn returns the body in lang, or in English.
func (it Item) BodyIn(lang string) string { return pick(it.Body, lang) }

// BodyLines returns the body in lang split at line breaks.
func (it Item) BodyLines(lang string) []string { return strings.Split(it.BodyIn(lang), "\n") }

func pick(m map[string]string, lang string) string {
	if s, ok := m[lang]; ok && s != "" {
		return s
	}
	return m["en"]
}

// Parse validates a feed. A file over MaxBytes, not UTF-8, not JSON or with v
// other than 1 is an error; items that fail the rules are dropped; a bad link
// is dropped and its item kept; unknown fields are ignored.
func Parse(data []byte) ([]Item, error) {
	if len(data) > MaxBytes {
		return nil, errors.New("news: too large")
	}
	if !utf8.Valid(data) {
		return nil, errors.New("news: not UTF-8")
	}
	var f struct {
		V     json.RawMessage   `json:"v"`
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, errors.New("news: not JSON")
	}
	if string(bytes.TrimSpace(f.V)) != "1" {
		return nil, errors.New("news: unknown version")
	}
	out := []Item{}
	for _, raw := range f.Items {
		if len(out) == MaxItems {
			break
		}
		if it, ok := parseItem(raw); ok {
			out = append(out, it)
		}
	}
	return out, nil
}

func parseItem(raw json.RawMessage) (Item, bool) {
	var in struct {
		ID    string          `json:"id"`
		Date  string          `json:"date"`
		Until *string         `json:"until"`
		Title json.RawMessage `json:"title"`
		Body  json.RawMessage `json:"body"`
		Link  json.RawMessage `json:"link"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return Item{}, false
	}
	if !core.ValidNewsID(in.ID) || !validDate(in.Date) {
		return Item{}, false
	}
	it := Item{ID: in.ID, Date: in.Date}
	if in.Until != nil {
		if !validDate(*in.Until) {
			return Item{}, false
		}
		it.Until = *in.Until
	}
	var ok bool
	if it.Title, ok = texts(in.Title, MaxTitle, false); !ok {
		return Item{}, false
	}
	if it.Body, ok = texts(in.Body, MaxBody, true); !ok {
		return Item{}, false
	}
	var link string
	if len(in.Link) > 0 && json.Unmarshal(in.Link, &link) == nil && SafeLink(link) {
		it.Link = link
	}
	return it, true
}

func validDate(s string) bool {
	if !dateRe.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// texts reads a {lang: text} object. English must be valid; another language
// that is not valid is dropped (it falls back to English).
func texts(raw json.RawMessage, max int, lines bool) (map[string]string, bool) {
	var m map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	out := map[string]string{}
	for _, l := range Langs {
		var s string
		if v, ok := m[l]; !ok || json.Unmarshal(v, &s) != nil || !plain(s, max, lines) {
			continue
		}
		out[l] = s
	}
	if out["en"] == "" {
		return nil, false
	}
	return out, true
}

// plain reports whether s is non-empty plain text of at most max characters,
// with no control characters other than a line break where lines is true.
func plain(s string, max int, lines bool) bool {
	if strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if r == '\n' && lines {
			continue
		}
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

// SafeLink reports whether link may be shown: https on raisesteady.com, or
// on github.com under /tengotengo23/.
func SafeLink(link string) bool {
	if len(link) > 300 || strings.ContainsAny(link, " \t\r\n\\\"'<>") {
		return false
	}
	var host string
	switch {
	case strings.HasPrefix(link, "https://raisesteady.com/"):
		host = "raisesteady.com"
	case strings.HasPrefix(link, "https://github.com/tengotengo23/"):
		host = "github.com"
	default:
		return false
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != host || u.User != nil || u.Opaque != "" {
		return false
	}
	// No dot segments (plain or %2e): a browser would resolve them out of the
	// allowed path.
	if strings.Contains(u.Path, "/.") || strings.Contains(strings.ToLower(link), "%2e") {
		return false
	}
	if host == "github.com" && !strings.HasPrefix(u.Path, "/tengotengo23/") {
		return false
	}
	return true
}

// Visible returns the items to show on day today (YYYY-MM-DD, the reader's
// local date): not hidden and before their until day.
func Visible(items []Item, today string, hidden []string) []Item {
	h := map[string]bool{}
	for _, id := range hidden {
		h[id] = true
	}
	out := []Item{}
	for _, it := range items {
		if h[it.ID] || (it.Until != "" && today >= it.Until) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// Reader keeps the feed cached in memory and in a file, and refreshes it in
// the background. Items never waits for the network.
type Reader struct {
	path   string
	now    func() time.Time
	client *http.Client

	mu       sync.Mutex
	loaded   bool
	items    []Item
	fetched  time.Time
	tried    time.Time
	fetching bool
	wg       sync.WaitGroup
}

type cacheFile struct {
	Fetched time.Time       `json:"fetched"`
	Feed    json.RawMessage `json:"feed"`
}

// New returns a reader that caches in the file at path.
func New(path string, now func() time.Time) *Reader {
	if now == nil {
		now = time.Now
	}
	return &Reader{
		path: path, now: now,
		client: &http.Client{
			Timeout: Timeout,
			// No cookie jar. Redirects are not followed: the feed has one address.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Items returns the cached items (maybe none) at once and, when the cache is
// older than TTL, starts one background refresh.
func (r *Reader) Items() []Item {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded {
		r.loaded = true
		r.loadFile()
	}
	now := r.now()
	stale := r.fetched.IsZero() || now.Sub(r.fetched) >= TTL || now.Before(r.fetched)
	if stale && !r.fetching && (r.tried.IsZero() || now.Sub(r.tried) >= retry || now.Before(r.tried)) {
		r.fetching, r.tried = true, now
		r.wg.Add(1)
		go r.refresh()
	}
	return append([]Item(nil), r.items...)
}

// Wait waits for a running refresh (for tests and shutdown).
func (r *Reader) Wait() { r.wg.Wait() }

func (r *Reader) loadFile() {
	data, err := os.ReadFile(r.path)
	if err != nil || len(data) > 2*MaxBytes {
		return
	}
	var c cacheFile
	if json.Unmarshal(data, &c) != nil {
		return
	}
	items, err := Parse(c.Feed)
	if err != nil {
		return
	}
	r.items, r.fetched = items, c.Fetched
}

func (r *Reader) refresh() {
	defer r.wg.Done()
	data, err := r.fetch()
	var items []Item
	if err == nil {
		items, err = Parse(data)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fetching = false
	if err != nil {
		return // keep the cached copy
	}
	r.items, r.fetched = items, r.now()
	if b, err := json.Marshal(cacheFile{Fetched: r.fetched, Feed: data}); err == nil {
		_ = core.WriteFileAtomic(r.path, b)
	}
}

func (r *Reader) fetch() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RaiseSteady-Guard")
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("news: status " + resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, errors.New("news: too large")
	}
	return data, nil
}
