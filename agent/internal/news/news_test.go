package news

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func feed(items string) []byte { return []byte(`{"v":1,"items":[` + items + `]}`) }

const good = `{"id":"a-1","date":"2026-11-18","title":{"en":"T","ka":"ტ"},"body":{"en":"one\ntwo"},"link":"https://raisesteady.com/guard/"}`

func TestParseGood(t *testing.T) {
	items, err := Parse(feed(good + `,{"id":"b","date":"2026-11-17","until":"2026-12-31","title":{"en":"U"},"body":{"en":"x"},"extra":5}`))
	if err != nil || len(items) != 2 {
		t.Fatalf("%v %d", err, len(items))
	}
	if items[0].Link == "" || items[0].TitleIn("ka") != "ტ" || items[0].TitleIn("ru") != "T" || len(items[0].BodyLines("en")) != 2 {
		t.Fatalf("%+v", items[0])
	}
	if items[1].Until != "2026-12-31" {
		t.Fatal("until lost")
	}
}

func TestParseRejectsFile(t *testing.T) {
	big := feed(`{"id":"a","date":"2026-11-18","title":{"en":"T"},"body":{"en":"` + strings.Repeat("x", MaxBytes) + `"}}`)
	for name, data := range map[string][]byte{
		"oversize": big, "v2": []byte(`{"v":2,"items":[` + good + `]}`), "v string": []byte(`{"v":"1","items":[]}`),
		"no v": []byte(`{"items":[` + good + `]}`), "not json": []byte(`<html>`), "not utf8": {'{', 0xff, '}'},
	} {
		if items, err := Parse(data); err == nil {
			t.Errorf("%s: accepted %d items", name, len(items))
		}
	}
}

func TestParseDropsBadItems(t *testing.T) {
	bad := []string{
		`{"id":"Bad","date":"2026-11-18","title":{"en":"T"},"body":{"en":"b"}}`,
		`{"id":"` + strings.Repeat("a", 41) + `","date":"2026-11-18","title":{"en":"T"},"body":{"en":"b"}}`,
		`{"id":"a","date":"18.11.2026","title":{"en":"T"},"body":{"en":"b"}}`,
		`{"id":"a","date":"2026-02-30","title":{"en":"T"},"body":{"en":"b"}}`,
		`{"id":"a","date":"2026-11-18","until":"soon","title":{"en":"T"},"body":{"en":"b"}}`,
		`{"id":"a","date":"2026-11-18","title":{"ka":"T"},"body":{"en":"b"}}`,
		`{"id":"a","date":"2026-11-18","title":{"en":"` + strings.Repeat("т", MaxTitle+1) + `"},"body":{"en":"b"}}`,
		`{"id":"a","date":"2026-11-18","title":{"en":"T"},"body":{"en":"` + strings.Repeat("b", MaxBody+1) + `"}}`,
		`{"id":"a","date":"2026-11-18","title":{"en":"line\nbreak"},"body":{"en":"b"}}`,
		`{"id":"a","date":"2026-11-18","title":{"en":"T"},"body":{"en":"bell\u0007"}}`,
		`{"id":"a","date":"2026-11-18","title":{"en":"T"},"body":{"en":"   "}}`,
		`{"id":"a","date":"2026-11-18","title":"T","body":{"en":"b"}}`,
		`"just a string"`,
	}
	for _, b := range bad {
		items, err := Parse(feed(b))
		if err != nil || len(items) != 0 {
			t.Errorf("kept %s: %v %d", b, err, len(items))
		}
	}
	// Exactly the limits pass; a bad other language falls back to English.
	ok := `{"id":"` + strings.Repeat("a", 40) + `","date":"2026-11-18","title":{"en":"` + strings.Repeat("т", MaxTitle) + `","ru":"` + strings.Repeat("я", MaxTitle+1) + `"},"body":{"en":"` + strings.Repeat("b", MaxBody) + `"}}`
	items, _ := Parse(feed(ok))
	if len(items) != 1 || items[0].TitleIn("ru") != items[0].TitleIn("en") {
		t.Fatalf("limits: %d", len(items))
	}
}

func TestParseKeepsFiveNewest(t *testing.T) {
	var parts []string
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		parts = append(parts, `{"id":"`+id+`","date":"2026-11-18","title":{"en":"T"},"body":{"en":"b"}}`)
	}
	items, _ := Parse(feed(strings.Join(parts, ",")))
	if len(items) != MaxItems || items[0].ID != "a" || items[4].ID != "e" {
		t.Fatalf("%d", len(items))
	}
}

func TestSafeLink(t *testing.T) {
	for _, l := range []string{"https://raisesteady.com/guard/", "https://raisesteady.com/guard/?a=1#x", "https://github.com/tengotengo23/raisesteady-guard/releases"} {
		if !SafeLink(l) {
			t.Errorf("refused %s", l)
		}
	}
	for _, l := range []string{"http://raisesteady.com/", "https://raisesteady.com", "https://raisesteady.com.evil.example/", "https://raisesteady.com@evil.example/",
		"https://user@raisesteady.com/", "https://raisesteady.com:8443/", "https://evil.example/https://raisesteady.com/", "javascript:alert(1)",
		"https://github.com/someoneelse/x", "https://github.com/tengotengo23", "https://github.com/tengotengo23/../evil/", "https://github.com/tengotengo23/%2E%2E/evil/", "https://github.com/tengotengo23/./x", "https://raisesteady.com/a b",
		"https://raisesteady.com/\"onmouseover=x", "https://raisesteady.com/<x>", "https://raisesteady.com/\\evil", "data:text/html,x", "", "https://raisesteady.com/" + strings.Repeat("a", 300)} {
		if SafeLink(l) {
			t.Errorf("allowed %s", l)
		}
	}
	// A bad link is dropped; the note stays.
	items, _ := Parse(feed(`{"id":"a","date":"2026-11-18","title":{"en":"T"},"body":{"en":"b"},"link":"https://evil.example/"}`))
	if len(items) != 1 || items[0].Link != "" {
		t.Fatalf("%+v", items)
	}
}

func TestVisible(t *testing.T) {
	items := []Item{{ID: "past", Until: "2026-11-17"}, {ID: "today", Until: "2026-11-18"}, {ID: "later", Until: "2026-11-19"}, {ID: "open"}, {ID: "hid"}}
	got := Visible(items, "2026-11-18", []string{"hid"})
	if len(got) != 2 || got[0].ID != "later" || got[1].ID != "open" {
		t.Fatalf("%+v", got)
	}
}

type srv struct {
	mu    sync.Mutex
	hits  int
	body  string
	code  int
	bad   []string
	ready chan struct{}
}

func (s *srv) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits++
	if r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" || r.Method != "GET" {
		s.bad = append(s.bad, r.Method+" "+r.URL.String()+" cookie="+r.Header.Get("Cookie"))
	}
	http.SetCookie(w, &http.Cookie{Name: "track", Value: "1"})
	w.WriteHeader(s.code)
	w.Write([]byte(s.body))
}

func start(t *testing.T, s *srv) {
	t.Helper()
	ts := httptest.NewServer(s)
	old := feedURL
	feedURL = ts.URL + "/guard/news.json"
	t.Cleanup(func() { feedURL = old; ts.Close() })
}

func TestReaderCachesAndNeverBlocks(t *testing.T) {
	s := &srv{code: 200, body: string(feed(good))}
	start(t, s)
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "news.json")
	r := New(path, clock)
	if got := r.Items(); len(got) != 0 {
		t.Fatalf("first call should return the empty cache at once, got %d", len(got))
	}
	r.Wait()
	if got := r.Items(); len(got) != 1 {
		t.Fatalf("after refresh: %d", len(got))
	}
	r.Wait()
	// The second call within 6 h makes no second request; the cookie set by the server is never sent back.
	now = now.Add(TTL - time.Minute)
	r.Items()
	r.Wait()
	if s.hits != 1 {
		t.Fatalf("hits %d", s.hits)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("no cache file")
	}
	// A new reader (a restart) uses the cache file without a request.
	r2 := New(path, clock)
	if got := r2.Items(); len(got) != 1 {
		t.Fatalf("cache file: %d", len(got))
	}
	r2.Wait()
	if s.hits != 1 {
		t.Fatalf("restart fetched again: %d", s.hits)
	}
	// After 6 h: refresh; a server error keeps the cached copy.
	now = now.Add(2 * time.Minute)
	s.mu.Lock()
	s.code, s.body = 500, "oops"
	s.mu.Unlock()
	r.Items()
	r.Wait()
	if got := r.Items(); len(got) != 1 || s.hits != 2 {
		t.Fatalf("error lost the cache: %d items, %d hits", len(got), s.hits)
	}
	r.Wait()
	if s.hits != 2 {
		t.Fatalf("retried at once after an error: %d", s.hits)
	}
	// A bad version keeps the cache too; later a good feed replaces it.
	now = now.Add(retry)
	s.mu.Lock()
	s.code, s.body = 200, `{"v":2,"items":[]}`
	s.mu.Unlock()
	r.Items()
	r.Wait()
	if got := r.Items(); len(got) != 1 {
		t.Fatal("bad version replaced the cache")
	}
	r.Wait()
	now = now.Add(retry)
	s.mu.Lock()
	s.body = string(feed(""))
	s.mu.Unlock()
	r.Items()
	r.Wait()
	if got := r.Items(); len(got) != 0 {
		t.Fatalf("empty feed not taken: %d", len(got))
	}
	r.Wait()
	if len(s.bad) != 0 {
		t.Fatalf("requests carried data: %v", s.bad)
	}
}

func TestReaderRefusesOversizeAndRedirect(t *testing.T) {
	s := &srv{code: 200, body: string(feed(`{"id":"a","date":"2026-11-18","title":{"en":"T"},"body":{"en":"` + strings.Repeat("x", MaxBytes) + `"}}`))}
	start(t, s)
	r := New(filepath.Join(t.TempDir(), "n.json"), nil)
	r.Items()
	r.Wait()
	if len(r.Items()) != 0 {
		t.Fatal("oversize accepted")
	}
	r.Wait()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(feed(good)) }))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redir.Close()
	feedURL = redir.URL
	r2 := New(filepath.Join(t.TempDir(), "n.json"), nil)
	r2.Items()
	r2.Wait()
	if len(r2.Items()) != 0 {
		t.Fatal("redirect followed")
	}
	r2.Wait()
}
