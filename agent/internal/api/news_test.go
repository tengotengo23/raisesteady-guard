package api

import (
	"net/url"
	"strings"
	"sync"
	"testing"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/news"
)

// fakeNews counts how often the page asked for news (P2-252).
type fakeNews struct {
	mu    sync.Mutex
	calls int
	items []news.Item
}

func (f *fakeNews) Items() []news.Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.items
}

func (f *fakeNews) n() int { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

func newsRig(t *testing.T, choice string, items ...news.Item) (*rig, *fakeNews, *browser) {
	t.Helper()
	r := newRig(t)
	f := &fakeNews{items: items}
	r.s.News = f
	r.cfg.News = choice
	b := newBrowser(t, r)
	if w := b.login("2468"); w.Code != 303 {
		t.Fatalf("login: %d", w.Code)
	}
	return r, f, b
}

func TestNewsNotChosenShowsTurnOnAndReadsNothing(t *testing.T) {
	for _, choice := range []string{"", core.NewsOff} {
		_, f, b := newsRig(t, choice)
		page := b.send("GET", "/parent", nil, "").Body.String()
		for _, want := range []string{"News from RaiseSteady", "Show short news from the makers of RaiseSteady Guard on this page: new versions, fixes, tips.",
			"When this page opens, it reads a small news file from raisesteady.com.", `name="action" value="on">Turn on</button>`} {
			if !strings.Contains(page, want) {
				t.Errorf("choice %q: page lacks %q", choice, want)
			}
		}
		if strings.Contains(page, "Turn off news") {
			t.Errorf("choice %q: shows Turn off", choice)
		}
		if f.n() != 0 {
			t.Errorf("choice %q: news read %d times before the parent turned it on", choice, f.n())
		}
	}
}

func TestNewsNotReadWhenLoggedOut(t *testing.T) {
	r := newRig(t)
	f := &fakeNews{}
	r.s.News = f
	r.cfg.News = core.NewsOn
	page := newBrowser(t, r).send("GET", "/parent", nil, "").Body.String()
	if f.n() != 0 || strings.Contains(page, "News from RaiseSteady") {
		t.Fatalf("logged out: calls=%d, box shown=%v", f.n(), strings.Contains(page, "News from RaiseSteady"))
	}
}

func TestNewsTurnOnAndOff(t *testing.T) {
	r, f, b := newsRig(t, "")
	csrf := b.csrf()
	w := b.send("POST", "/parent/news", url.Values{"csrf": {csrf}, "lang": {"en"}, "a": {"1"}, "action": {"on"}}, "http://127.0.0.1:47615")
	if w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "sec=news") || !strings.Contains(w.Header().Get("Location"), "a=1") {
		t.Fatalf("on: %d %s", w.Code, w.Header().Get("Location"))
	}
	if r.cfg.News != core.NewsOn {
		t.Fatalf("news = %q", r.cfg.News)
	}
	if f.n() != 0 {
		t.Fatalf("turning on read the news %d times", f.n())
	}
	page := b.send("GET", "/parent", nil, "").Body.String()
	if f.n() != 1 || !strings.Contains(page, "Turn off news") || strings.Contains(page, "Turn on</button>") {
		t.Fatalf("on and empty: calls=%d", f.n())
	}
	w = b.send("POST", "/parent/news", url.Values{"csrf": {csrf}, "lang": {"en"}, "action": {"off"}}, "http://127.0.0.1:47615")
	if w.Code != 303 || r.cfg.News != core.NewsOff {
		t.Fatalf("off: %d %q", w.Code, r.cfg.News)
	}
}

func TestNewsPostNeedsCSRFAndLogin(t *testing.T) {
	r, _, b := newsRig(t, "")
	w := b.send("POST", "/parent/news", url.Values{"csrf": {"nope"}, "lang": {"en"}, "action": {"on"}}, "http://127.0.0.1:47615")
	if r.cfg.News != "" || r.saves != 0 {
		t.Fatalf("bad CSRF changed the choice: %d %q", w.Code, r.cfg.News)
	}
	w = b.send("POST", "/parent/news", url.Values{"csrf": {b.csrf()}, "lang": {"en"}, "action": {"on"}}, "http://evil.example")
	if r.cfg.News != "" {
		t.Fatalf("foreign origin changed the choice: %d", w.Code)
	}
	out := newBrowser(t, r)
	out.send("POST", "/parent/news", url.Values{"lang": {"en"}, "action": {"on"}}, "http://127.0.0.1:47615")
	if r.cfg.News != "" {
		t.Fatal("logged out post changed the choice")
	}
	for _, f := range []url.Values{{"action": {"hide"}, "id": {"Bad ID"}}, {"action": {"hide"}}, {"action": {"maybe"}}} {
		f.Set("csrf", b.csrf())
		f.Set("lang", "en")
		w := b.send("POST", "/parent/news", f, "http://127.0.0.1:47615")
		if !strings.Contains(w.Header().Get("Location"), "err=bad_input") || len(r.cfg.NewsHidden) != 0 {
			t.Errorf("%v: %s", f, w.Header().Get("Location"))
		}
	}
}

func TestNewsRendersEscaped(t *testing.T) {
	evil := news.Item{ID: "x-1", Date: "2026-11-01",
		Title: map[string]string{"en": `<script>alert(1)</script>`},
		Body:  map[string]string{"en": "line one <b>bold</b>\nline \"two\""},
		Link:  "https://raisesteady.com/guard/?a=1&b=2"}
	ka := news.Item{ID: "x-2", Date: "2026-11-01", Title: map[string]string{"en": "English title", "ka": "ქართული"}, Body: map[string]string{"en": "Body"}}
	_, _, b := newsRig(t, core.NewsOn, evil, ka)
	page := b.send("GET", "/parent", nil, "").Body.String()
	if strings.Contains(page, "<script>alert") || strings.Contains(page, "<b>bold</b>") {
		t.Fatal("news text not escaped")
	}
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "line one &lt;b&gt;bold&lt;/b&gt;<br>line &#34;two&#34;",
		`<a href="https://raisesteady.com/guard/?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">Read more</a>`,
		`name="id" value="x-1">`, `value="hide">Hide</button>`, "English title", "Turn off news"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Count(page, "Read more") != 1 {
		t.Errorf("Read more shown %d times, want 1 (no link on the second note)", strings.Count(page, "Read more"))
	}
	kaPage := b.send("GET", "/parent?lang=ka", nil, "").Body.String()
	if !strings.Contains(kaPage, "ქართული") || !strings.Contains(kaPage, "სიახლეები RaiseSteady-სგან") {
		t.Error("Georgian title not shown")
	}
	// The box sits under the open requests and above the settings.
	i, j := strings.Index(page, `id="news-h"`), strings.Index(page, `id="pause-h"`)
	if i < 0 || j < 0 || i > j {
		t.Errorf("news box at %d, pause card at %d", i, j)
	}
}

func TestNewsUntilAndHidePersist(t *testing.T) {
	old := news.Item{ID: "old", Date: "2026-10-01", Until: "2026-11-03", Title: map[string]string{"en": "Old note"}, Body: map[string]string{"en": "b"}}
	keep := news.Item{ID: "keep", Date: "2026-11-01", Until: "2026-11-04", Title: map[string]string{"en": "Kept note"}, Body: map[string]string{"en": "b"}}
	r, _, b := newsRig(t, core.NewsOn, old, keep)
	page := b.send("GET", "/parent", nil, "").Body.String()
	if strings.Contains(page, "Old note") || !strings.Contains(page, "Kept note") {
		t.Fatal("until day not applied (the rig's day is 2026-11-03)")
	}
	csrf := b.csrf()
	for range 2 {
		w := b.send("POST", "/parent/news", url.Values{"csrf": {csrf}, "lang": {"en"}, "action": {"hide"}, "id": {"keep"}}, "http://127.0.0.1:47615")
		if w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "ok=saved") {
			t.Fatalf("hide: %d %s", w.Code, w.Header().Get("Location"))
		}
	}
	if len(r.cfg.NewsHidden) != 1 || r.cfg.NewsHidden[0] != "keep" {
		t.Fatalf("hidden = %v", r.cfg.NewsHidden)
	}
	if err := r.cfg.Validate(r.s.Bands); err != nil {
		t.Fatal(err)
	}
	page = b.send("GET", "/parent", nil, "").Body.String()
	if strings.Contains(page, "Kept note") || !strings.Contains(page, "Turn off news") {
		t.Fatal("a hidden note came back, or the switch went")
	}
	// The list of hidden ids stays bounded.
	r.cfg.NewsHidden = nil
	for i := range core.MaxNewsHidden {
		r.cfg.NewsHidden = append(r.cfg.NewsHidden, "n"+strings.Repeat("a", i%5)+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	b.send("POST", "/parent/news", url.Values{"csrf": {csrf}, "lang": {"en"}, "action": {"hide"}, "id": {"newest"}}, "http://127.0.0.1:47615")
	if n := len(r.cfg.NewsHidden); n != core.MaxNewsHidden || r.cfg.NewsHidden[n-1] != "newest" {
		t.Fatalf("hidden list: %d, last %q", n, r.cfg.NewsHidden[n-1])
	}
}

func TestSetupNewsCheckbox(t *testing.T) {
	for _, c := range []struct {
		val, want string
	}{{"on", core.NewsOn}, {"", core.NewsOff}} {
		r := waitingRig(t)
		f := &fakeNews{}
		r.s.News = f
		b := newBrowser(t, r)
		page := b.send("GET", "/parent", nil, "").Body.String()
		box := `<input type="checkbox" name="news" value="on" checked> <span>News from RaiseSteady on the parent page (reads a small file from raisesteady.com when the page opens)</span>`
		if !strings.Contains(page, box) {
			t.Fatal("setup page lacks the ticked news checkbox")
		}
		if strings.Index(page, `name="dns"`) > strings.Index(page, `name="news"`) {
			t.Error("news checkbox is not after the family DNS filter")
		}
		form := setupForm(b)
		if c.val != "" {
			form.Set("news", c.val)
		}
		w := b.send("POST", "/parent/setup", form, "http://127.0.0.1:47615")
		if w.Code != 303 || r.cfg.SetupPending {
			t.Fatalf("setup: %d %s", w.Code, w.Header().Get("Location"))
		}
		if r.cfg.News != c.want {
			t.Errorf("news %q: stored %q, want %q", c.val, r.cfg.News, c.want)
		}
		if f.n() != 0 {
			t.Error("the setup page read news")
		}
	}
	// A refused form keeps the unticked box unticked.
	r := waitingRig(t)
	b := newBrowser(t, r)
	form := setupForm(b)
	form.Del("agree")
	loc := b.send("POST", "/parent/setup", form, "http://127.0.0.1:47615").Header().Get("Location")
	if !strings.Contains(loc, "news=off") {
		t.Fatalf("redirect %s lacks news=off", loc)
	}
	u, _ := url.Parse(loc)
	page := b.send("GET", u.RequestURI(), nil, "").Body.String()
	if strings.Contains(page, `name="news" value="on" checked`) {
		t.Error("the unticked news box came back ticked")
	}
}

func TestConfigValidatesNews(t *testing.T) {
	r := newRig(t)
	for _, c := range []struct {
		news   string
		hidden []string
		ok     bool
	}{{"", nil, true}, {"on", []string{"a-1"}, true}, {"off", nil, true}, {"yes", nil, false}, {"on", []string{"Bad"}, false}, {"on", []string{""}, false}} {
		cfg := r.cfg.Clone()
		cfg.News, cfg.NewsHidden = c.news, c.hidden
		if err := cfg.Validate(r.s.Bands); (err == nil) != c.ok {
			t.Errorf("news %q hidden %v: err %v", c.news, c.hidden, err)
		}
	}
}
