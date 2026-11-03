package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/store"
)

const (
	devID = "jihggkhiiiinnflomjjnpomgeijgfnlg"
	base  = "http://127.0.0.1:47615"
	kidA  = "S-1-5-21-1-2-3-1001"
	kidB  = "S-1-5-21-1-2-3-1002"
)

var (
	tokA = strings.Repeat("a1", 32)
	tokB = strings.Repeat("b2", 32)
)

type rig struct {
	s     *Server
	mu    sync.Mutex
	cfg   core.Config
	saves int
	now   time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	h, _ := core.HashPin("2468")
	b6, _ := d.Bands.Get("6-8")
	b13, _ := d.Bands.Get("13-15")
	r := &rig{now: time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)}
	r.cfg = core.Config{Version: 1, InstallID: "i", Port: 47615, Mode: core.ModeUsers, Pin: h, SettingsVersion: 1,
		ExtIDs: core.ExtIDs{ChromiumDevID: devID, FirefoxID: "guard@raisesteady.com"},
		Accounts: []core.Account{
			{SID: kidA, Name: "Ana", Token: tokA, Settings: core.SettingsForBand(b6)},
			{SID: kidB, Name: "Luka", Token: tokB, Settings: core.SettingsForBand(b13)},
		}}
	now := func() time.Time { return r.now }
	st, _ := store.Open("", now)
	r.s = &Server{
		Port: 47615, Bands: &d.Bands, Store: st, Lock: &core.Lockout{Now: now}, Now: now,
		Config: func() core.Config { r.mu.Lock(); defer r.mu.Unlock(); return r.cfg.Clone() },
		Save: func(c core.Config) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.cfg = c
			r.saves++
			return nil
		},
		Browsers: func() []BrowserStatus {
			return []BrowserStatus{{Name: "Google Chrome", Account: "Ana", State: "active", AgoMin: 3}, {Name: "Microsoft Edge", Account: "Ana", State: "set"}, {Name: "Firefox", Account: "Luka", State: "waiting"}, {Name: "Opera", State: "closed"}, {Name: "Odd", State: "protected"}}
		},
		LocalAccounts: func() []LocalAccount {
			return []LocalAccount{{SID: kidA, Name: "Ana"}, {SID: kidB, Name: "Luka"}, {SID: "S-1-5-21-1-2-3-1000", Name: "Parent", Admin: true}}
		},
	}
	return r
}

type req struct {
	method, path, body, token, origin, ctype, host string
	hdr                                            map[string]string
}

func (r *rig) do(q req) *httptest.ResponseRecorder {
	if q.method == "" {
		q.method = http.MethodGet
	}
	hr := httptest.NewRequest(q.method, base+q.path, strings.NewReader(q.body))
	if q.host != "" {
		hr.Host = q.host
	}
	if q.token != "" {
		hr.Header.Set("X-KG-Token", q.token)
	}
	if q.origin != "" {
		hr.Header.Set("Origin", q.origin)
	}
	if q.ctype == "" && q.method == http.MethodPost {
		q.ctype = "application/json"
	}
	if q.ctype != "" {
		hr.Header.Set("Content-Type", q.ctype)
	}
	for k, v := range q.hdr {
		hr.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.s.Handler().ServeHTTP(w, hr)
	return w
}

func decodeState(t *testing.T, w *httptest.ResponseRecorder) State {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var s State
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTokenMapsToAccount(t *testing.T) {
	r := newRig(t)
	a := decodeState(t, r.do(req{path: "/v1/state", token: tokA}))
	b := decodeState(t, r.do(req{path: "/v1/state", token: tokB}))
	if a.Band != "6-8" || a.Limits.BrowseMin != 90 || b.Band != "13-15" || b.Limits.VideoMin != 90 {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
	if a.Day != "2026-11-03" || a.Allow == nil || a.Block == nil || a.SettingsVersion != 1 || a.Now == "" {
		t.Fatalf("state fields: %+v", a)
	}
	for _, tok := range []string{"", strings.Repeat("0", 64), "xyz", tokA + "0"} {
		if w := r.do(req{path: "/v1/state", token: tok}); w.Code != http.StatusUnauthorized {
			t.Errorf("token %q: %d", tok, w.Code)
		}
	}
	// Usage goes to the right account only.
	decodeState(t, r.do(req{method: "POST", path: "/v1/usage", token: tokA, body: `{"browser":"chrome","browseSec":30}`}))
	if r.s.Store.Today(kidA).BrowseSec != 30 || r.s.Store.Today(kidB).BrowseSec != 0 {
		t.Fatal("usage counted on the wrong account")
	}
}

func TestOriginChecks(t *testing.T) {
	r := newRig(t)
	for _, o := range []string{"https://evil.example", "http://127.0.0.1:47615", "null", "chrome-extension://" + strings.Repeat("p", 32), "moz-extension://not-a-uuid"} {
		w := r.do(req{path: "/v1/state", token: tokA, origin: o})
		if w.Code != http.StatusForbidden {
			t.Errorf("origin %q: %d", o, w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("origin %q got ACAO", o)
		}
	}
	for _, o := range []string{"chrome-extension://" + devID, "moz-extension://0b1c2d3e-4f50-6172-8394-a5b6c7d8e9f0"} {
		w := r.do(req{path: "/v1/state", token: tokA, origin: o})
		if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != o {
			t.Errorf("origin %q: %d ACAO=%q", o, w.Code, w.Header().Get("Access-Control-Allow-Origin"))
		}
	}
	if w := r.do(req{path: "/v1/state", token: tokA}); w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("no Origin, no ACAO")
	}
}

func TestPreflight(t *testing.T) {
	r := newRig(t)
	w := r.do(req{method: "OPTIONS", path: "/v1/usage", origin: "https://evil.example", hdr: map[string]string{"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "x-kg-token"}})
	if w.Code != http.StatusForbidden || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("web preflight: %d %v", w.Code, w.Header())
	}
	if w := r.do(req{method: "OPTIONS", path: "/v1/usage"}); w.Code != http.StatusForbidden {
		t.Fatalf("preflight without origin: %d", w.Code)
	}
	o := "chrome-extension://" + devID
	w = r.do(req{method: "OPTIONS", path: "/v1/usage", origin: o})
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != o || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-KG-Token") {
		t.Fatalf("extension preflight: %d %v", w.Code, w.Header())
	}
}

func TestHostAndContentType(t *testing.T) {
	r := newRig(t)
	for _, h := range []string{"evil.example:47615", "127.0.0.1:80", "192.168.1.5:47615"} {
		if w := r.do(req{path: "/v1/state", token: tokA, host: h}); w.Code != http.StatusMisdirectedRequest {
			t.Errorf("host %q: %d", h, w.Code)
		}
	}
	if w := r.do(req{path: "/v1/state", token: tokA, host: "localhost:47615"}); w.Code != 200 {
		t.Errorf("localhost: %d", w.Code)
	}
	if w := r.do(req{method: "POST", path: "/v1/usage", token: tokA, ctype: "text/plain", body: `{"browser":"chrome"}`}); w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain: %d", w.Code)
	}
	if w := r.do(req{method: "POST", path: "/v1/state", token: tokA}); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST state: %d", w.Code)
	}
	big := `{"browser":"chrome","domains":{"` + strings.Repeat("a", 20000) + `.com":1}}`
	if w := r.do(req{method: "POST", path: "/v1/usage", token: tokA, body: big}); w.Code != http.StatusBadRequest {
		t.Errorf("oversized body: %d", w.Code)
	}
	if w := r.do(req{path: "/v1/state", token: tokA}); w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Error("security headers missing")
	}
}

func TestUsageCapsThroughAPI(t *testing.T) {
	r := newRig(t)
	post := func(body string) State {
		return decodeState(t, r.do(req{method: "POST", path: "/v1/usage", token: tokA, body: body}))
	}
	s := post(`{"browser":"edge","browseSec":9999,"videoSec":9999,"learningSec":9999}`)
	if s.Used.BrowseSec != store.FirstReportCap || s.Used.VideoSec != store.FirstReportCap {
		t.Fatalf("first report not capped: %+v", s.Used)
	}
	r.now = r.now.Add(10 * time.Second)
	s = post(`{"browser":"edge","browseSec":60}`)
	if s.Used.BrowseSec != store.FirstReportCap+10 {
		t.Fatalf("not capped to the elapsed 10 s: %+v", s.Used)
	}
	r.now = r.now.Add(time.Hour)
	s = post(`{"browser":"edge","browseSec":3600}`)
	if s.Used.BrowseSec != store.FirstReportCap+10+store.MaxPerCall {
		t.Fatalf("not capped to 120: %+v", s.Used)
	}
	if w := r.do(req{method: "POST", path: "/v1/usage", token: tokA, body: `{"browser":"Chrome!"}`}); w.Code != 400 {
		t.Errorf("bad browser name: %d", w.Code)
	}
}

func TestRateLimit(t *testing.T) {
	r := newRig(t)
	code := 0
	for i := 0; i < ratePerMinute+1; i++ {
		code = r.do(req{path: "/v1/state", token: tokA}).Code
	}
	if code != http.StatusTooManyRequests {
		t.Fatalf("no rate limit: %d", code)
	}
	if r.do(req{path: "/v1/state", token: tokB}).Code != 200 {
		t.Fatal("the other account must not be limited")
	}
	r.now = r.now.Add(time.Minute)
	if r.do(req{path: "/v1/state", token: tokA}).Code != 200 {
		t.Fatal("limit should reset after a minute")
	}
}

func TestPinLockout(t *testing.T) {
	r := newRig(t)
	pin := func(p string) (int, map[string]any) {
		w := r.do(req{method: "POST", path: "/v1/pin", token: tokA, body: `{"pin":"` + p + `"}`})
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}
	if c, m := pin("2468"); c != 200 || m["ok"] != true {
		t.Fatalf("right PIN: %d %v", c, m)
	}
	for i := 0; i < 4; i++ {
		if c, m := pin("1111"); c != 200 || m["ok"] != false || m["triesLeft"] != float64(4-i) {
			t.Fatalf("wrong PIN %d: %d %v", i, c, m)
		}
	}
	if c, m := pin("1111"); c != http.StatusTooManyRequests || m["locked"] != true {
		t.Fatalf("fifth wrong PIN: %d %v", c, m)
	}
	if c, _ := pin("2468"); c != http.StatusTooManyRequests {
		t.Fatal("locked: the right PIN is refused too")
	}
	// The grant endpoint shares the lock.
	if w := r.do(req{method: "POST", path: "/v1/grant", token: tokA, body: `{"pin":"2468","kind":"extra","minutes":15}`}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("grant while locked: %d", w.Code)
	}
	r.now = r.now.Add(core.LockFor + time.Second)
	if c, m := pin("2468"); c != 200 || m["ok"] != true {
		t.Fatalf("after 15 minutes: %d %v", c, m)
	}
}

func TestGrantAndEvent(t *testing.T) {
	r := newRig(t)
	g := func(body string) *httptest.ResponseRecorder {
		return r.do(req{method: "POST", path: "/v1/grant", token: tokA, body: body})
	}
	if w := g(`{"pin":"9999","kind":"extra","minutes":15}`); w.Code != http.StatusForbidden {
		t.Fatalf("wrong PIN grant: %d", w.Code)
	}
	s := decodeState(t, g(`{"pin":"2468","kind":"extra","minutes":15}`))
	if s.ExtraMin != 15 {
		t.Fatalf("extra: %+v", s)
	}
	for _, bad := range []string{`{"pin":"2468","kind":"extra","minutes":0}`, `{"pin":"2468","kind":"extra","minutes":500}`, `{"pin":"2468","kind":"allow","domain":"not a domain"}`, `{"pin":"2468","kind":"nope"}`} {
		if w := g(bad); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	r.mu.Lock()
	r.cfg.Accounts[0].Block = []string{"wiki.example"}
	r.mu.Unlock()
	s = decodeState(t, g(`{"pin":"2468","kind":"allow","domain":"https://Wiki.example/page"}`))
	if len(s.Allow) != 1 || s.Allow[0] != "wiki.example" || len(s.Block) != 0 || s.SettingsVersion != 2 || r.saves != 1 {
		t.Fatalf("allow: %+v saves=%d", s, r.saves)
	}
	if w := r.do(req{method: "POST", path: "/v1/event", token: tokA, body: `{"type":"blocked","reason":"adult","domain":"x.example"}`}); w.Code != 200 {
		t.Fatalf("event: %d", w.Code)
	}
	for _, bad := range []string{`{"type":"blocked","reason":"other"}`, `{"type":"visit","reason":"adult"}`, `{"type":"blocked","reason":"adult"} {}`} {
		if w := r.do(req{method: "POST", path: "/v1/event", token: tokA, body: bad}); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	for _, reason := range []string{"parent", "video"} {
		if w := r.do(req{method: "POST", path: "/v1/event", token: tokA, body: `{"type":"blocked","reason":"` + reason + `"}`}); w.Code != 200 {
			t.Fatalf("event %s: %d", reason, w.Code)
		}
	}
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	page := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	for _, want := range []string{"Blocked pages by reason", "Adult content", "Blocked by you", "Videos"} {
		if !strings.Contains(page, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	rep, _ := r.s.Store.Report(kidA, 1)
	if rep[0].Blocked != 3 || rep[0].ByReason["adult"] != 1 || rep[0].ByReason["parent"] != 1 {
		t.Fatalf("event not counted: %+v", rep)
	}
}

func TestBedtime(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 3, h, m, 0, 0, time.Local) }
	b := core.Bedtime{Start: "21:00", End: "07:00"}
	for _, c := range []struct {
		t    time.Time
		want bool
	}{{at(20, 59), false}, {at(21, 0), true}, {at(23, 30), true}, {at(3, 0), true}, {at(6, 59), true}, {at(7, 0), false}, {at(12, 0), false}} {
		if BedtimeActive(c.t, b) != c.want {
			t.Errorf("%s: want %v", c.t.Format("15:04"), c.want)
		}
	}
	if !BedtimeActive(at(13, 30), core.Bedtime{Start: "13:00", End: "14:00"}) || BedtimeActive(at(9, 0), core.Bedtime{Start: "09:00", End: "09:00"}) {
		t.Error("same-day or empty window")
	}
}

// ---- parent page ----

type browser struct {
	r       *rig
	t       *testing.T
	cookies map[string]*http.Cookie
}

func (b *browser) send(method, path string, form url.Values, origin string) *httptest.ResponseRecorder {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	hr := httptest.NewRequest(method, base+path, body)
	if form != nil {
		hr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if origin != "" {
		hr.Header.Set("Origin", origin)
	}
	for _, c := range b.cookies {
		hr.AddCookie(c)
	}
	w := httptest.NewRecorder()
	b.r.s.Handler().ServeHTTP(w, hr)
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c
		}
	}
	return w
}

var csrfRe = regexp.MustCompile(`name="csrf" value="([0-9a-f]{64})"`)

func (b *browser) login(pin string) *httptest.ResponseRecorder {
	return b.send("POST", "/parent/login", url.Values{"pin": {pin}, "lang": {"en"}}, "http://127.0.0.1:47615")
}

func (b *browser) csrf() string {
	w := b.send("GET", "/parent", nil, "")
	m := csrfRe.FindStringSubmatch(w.Body.String())
	if m == nil {
		b.t.Fatalf("no CSRF token on the page: %s", w.Body.String()[:200])
	}
	return m[1]
}

func TestParentLoginAndCookie(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	w := b.send("GET", "/parent", nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `action="/parent/login"`) || strings.Contains(w.Body.String(), "Ana") {
		t.Fatal("login page expected, without account data")
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("CSP: %q", csp)
	}
	w = b.login("1357")
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "err=wrong_pin") || b.cookies[sessionCookie] != nil {
		t.Fatalf("wrong PIN: %s", loc)
	}
	if body := b.send("GET", w.Header().Get("Location"), nil, "").Body.String(); !strings.Contains(body, "Tries left: 4") {
		t.Fatal("tries left not shown")
	}
	w = b.login("2468")
	c := b.cookies[sessionCookie]
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/parent" || !core.ValidToken(c.Value) {
		t.Fatalf("session cookie: %+v", c)
	}
	if sc := w.Header().Get("Set-Cookie"); strings.Contains(sc, "Max-Age") || strings.Contains(sc, "Expires") {
		t.Fatalf("the session cookie must end with the browser: %s", sc)
	}
	en := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	for _, want := range []string{"Active · extension seen 3 min ago", "Settings set · extension not active yet", "Closed · not supported", "Log out",
		"What stays on this PC: minutes per day, the names of sites visited (no full addresses) and blocked pages, for 30 days. Nothing is sent to RaiseSteady.",
		"What leaves this PC: while the family DNS filter is on, your child&#39;s browsers look up site names through AdGuard DNS Family (a public filter); while focus mode is on, the chosen videos are loaded from YouTube.",
		"It protects web browsers only", "Summer time is not followed", `name="familyDns" aria-describedby="familyDns-hint" checked`, "Family DNS filter (AdGuard DNS Family)", `<option value="0" selected>`, "1 wrong PIN tries today"} {
		if !strings.Contains(en, want) {
			t.Errorf("English page lacks %q", want)
		}
	}
	if strings.Contains(en, "protected") || strings.Contains(en, "Odd") {
		t.Error(`the page must never say "protected" without a report`)
	}
	if strings.Contains(en, "current-password") || strings.Contains(en, "new-password") {
		t.Error("PIN inputs must not ask the browser to save the PIN")
	}
	page := b.send("GET", "/parent?lang=ka", nil, "").Body.String()
	for _, want := range []string{"Ana", "Luka", "მშობლის პარამეტრები", "Google Chrome", "აქტიურია · გაფართოება ბოლოს ჩანდა 3 წუთის წინ", "პარამეტრები დაყენებულია · გაფართოება ჯერ არ მუშაობს", "იხურება · არ არის მხარდაჭერილი", "რა რჩება ამ კომპიუტერზე", "AdGuard DNS Family-ით", `lang="ka"`} {
		if !strings.Contains(page, want) {
			t.Errorf("settings page lacks %q", want)
		}
	}
	if strings.Contains(page, tokA) || strings.Contains(page, r.cfg.Pin.Hash) {
		t.Fatal("token or PIN hash on the page")
	}
	// Logout ends the session.
	b.send("POST", "/parent/logout", url.Values{"csrf": {b.csrf()}}, "http://127.0.0.1:47615")
	if strings.Contains(b.send("GET", "/parent", nil, "").Body.String(), "Luka") {
		t.Fatal("still signed in after logout")
	}
}

func TestParentCSRFAndOrigin(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	form := url.Values{"a": {"0"}, "band": {"9-12"}, "browseMin": {"100"}, "videoMin": {"50"}, "bedStart": {"21:00"}, "bedEnd": {"07:00"}, "allow": {"school.example"}, "block": {""}, "focusMode": {"on"}}
	// No CSRF token: nothing changes.
	w := b.send("POST", "/parent/save", form, "http://127.0.0.1:47615")
	if !strings.Contains(w.Header().Get("Location"), "err=stale") || r.saves != 0 {
		t.Fatalf("save without CSRF: %s saves=%d", w.Header().Get("Location"), r.saves)
	}
	// Wrong origin: refused.
	form.Set("csrf", b.csrf())
	if w := b.send("POST", "/parent/save", form, "https://evil.example"); w.Code != http.StatusForbidden || r.saves != 0 {
		t.Fatalf("cross-origin save: %d", w.Code)
	}
	// Cross-site fetch metadata: refused.
	hr := httptest.NewRequest("POST", base+"/parent/save", strings.NewReader(form.Encode()))
	hr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hr.Header.Set("Sec-Fetch-Site", "cross-site")
	for _, c := range b.cookies {
		hr.AddCookie(c)
	}
	rw := httptest.NewRecorder()
	r.s.Handler().ServeHTTP(rw, hr)
	if rw.Code != http.StatusForbidden {
		t.Fatalf("cross-site save: %d", rw.Code)
	}
	// Right token, same origin: saved and applied.
	w = b.send("POST", "/parent/save", form, "http://127.0.0.1:47615")
	a := r.cfg.Accounts[0]
	if !strings.Contains(w.Header().Get("Location"), "ok=saved") || r.saves != 1 || a.Band != "9-12" || a.BrowseMin != 100 || a.VideoMin != 50 || !a.FocusMode || a.Adblock || !a.FamilyDNSOff || len(a.Allow) != 1 || r.cfg.SettingsVersion != 2 {
		t.Fatalf("save: %s %+v", w.Header().Get("Location"), a)
	}
	if !strings.Contains(b.send("GET", w.Header().Get("Location"), nil, "").Body.String(), "Saved. It applies now.") {
		t.Fatal("saved message not shown")
	}
	// Bad values: nothing saved.
	form.Set("browseMin", "-5")
	b.send("POST", "/parent/save", form, "http://127.0.0.1:47615")
	form.Set("browseMin", "100")
	form.Set("allow", "not a domain")
	b.send("POST", "/parent/save", form, "http://127.0.0.1:47615")
	if r.saves != 1 {
		t.Fatalf("bad input saved: %d", r.saves)
	}
	// Login from another origin is refused too (login CSRF).
	b2 := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	if w := b2.send("POST", "/parent/login", url.Values{"pin": {"2468"}}, "https://evil.example"); w.Code != http.StatusForbidden || b2.cookies[sessionCookie] != nil {
		t.Fatalf("cross-origin login: %d", w.Code)
	}
}

func TestParentAccountsAndPin(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	o := "http://127.0.0.1:47615"
	// Drop Luka, keep Ana (her token stays), refuse unknown SIDs.
	b.send("POST", "/parent/accounts", url.Values{"csrf": {b.csrf()}, "sid": {kidA, "S-1-5-21-9-9-9-1234"}}, o)
	if len(r.cfg.Accounts) != 1 || r.cfg.Accounts[0].Token != tokA || r.cfg.Mode != core.ModeUsers {
		t.Fatalf("accounts: %+v", r.cfg.Accounts)
	}
	// Add the parent account: new random token.
	b.send("POST", "/parent/accounts", url.Values{"csrf": {b.csrf()}, "sid": {kidA, "S-1-5-21-1-2-3-1000"}}, o)
	if len(r.cfg.Accounts) != 2 || !core.ValidToken(r.cfg.Accounts[1].Token) || r.cfg.Accounts[1].Token == tokA {
		t.Fatalf("add account: %+v", r.cfg.Accounts)
	}
	// None chosen: refused.
	w := b.send("POST", "/parent/accounts", url.Values{"csrf": {b.csrf()}}, o)
	if !strings.Contains(w.Header().Get("Location"), "err=no_accounts") || len(r.cfg.Accounts) != 2 {
		t.Fatal("empty account list accepted")
	}
	// Everyone on this PC.
	b.send("POST", "/parent/accounts", url.Values{"csrf": {b.csrf()}, "everyone": {"on"}}, o)
	if r.cfg.Mode != core.ModeAll || len(r.cfg.Accounts) != 1 || r.cfg.Accounts[0].SID != core.AllSID {
		t.Fatalf("everyone: %+v", r.cfg)
	}
	// Change the PIN.
	w = b.send("POST", "/parent/pin", url.Values{"csrf": {b.csrf()}, "pinNow": {"2468"}, "pinNew": {"97531"}, "pinAgain": {"97531"}}, o)
	if !strings.Contains(w.Header().Get("Location"), "ok=pin_changed") || !r.cfg.Pin.Check("97531") {
		t.Fatalf("pin change: %s", w.Header().Get("Location"))
	}
	w = b.send("POST", "/parent/pin", url.Values{"csrf": {b.csrf()}, "pinNow": {"97531"}, "pinNew": {"1234"}, "pinAgain": {"4321"}}, o)
	if !strings.Contains(w.Header().Get("Location"), "err=pin_mismatch") || !r.cfg.Pin.Check("97531") {
		t.Fatal("mismatched PINs accepted")
	}
}

func TestParentMessagesAreNotReflected(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	body := b.send("GET", "/parent?err=<script>alert(1)</script>&left=99", nil, "").Body.String()
	if strings.Contains(body, "<script>") || strings.Contains(body, "alert(1)") {
		t.Fatal("query text reflected into the page")
	}
	css := b.send("GET", "/parent/app.css", nil, "")
	if css.Code != 200 || !strings.Contains(css.Body.String(), "--kg-navy") || !strings.HasPrefix(css.Header().Get("Content-Type"), "text/css") {
		t.Fatal("stylesheet with tokens missing")
	}
}

func TestParentSessionAbsoluteLifetime(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	if w := b.send("GET", "/parent", nil, ""); strings.Contains(w.Body.String(), `autocomplete="current-password"`) || !strings.Contains(w.Body.String(), `autocomplete="off"`) {
		t.Fatal("login PIN input must have autocomplete off")
	}
	b.login("2468")
	for i := 0; i < 3; i++ { // activity does not extend the session
		r.now = r.now.Add(3 * time.Minute)
		if !strings.Contains(b.send("GET", "/parent", nil, "").Body.String(), "Luka") {
			t.Fatalf("signed out too early (%d min)", 3*(i+1))
		}
	}
	r.now = r.now.Add(time.Minute + time.Second) // 10 min 1 s after login
	if strings.Contains(b.send("GET", "/parent", nil, "").Body.String(), "Luka") {
		t.Fatal("the session must end 10 minutes after login")
	}
}

func TestPinCountersBySource(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 5; i++ {
		r.do(req{method: "POST", path: "/v1/pin", token: tokA, body: `{"pin":"1111"}`})
	}
	if w := r.do(req{method: "POST", path: "/v1/pin", token: tokA, body: `{"pin":"2468"}`}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("Ana's token should be locked: %d", w.Code)
	}
	// Luka's token and the parent page have their own counters.
	if w := r.do(req{method: "POST", path: "/v1/pin", token: tokB, body: `{"pin":"2468"}`}); w.Code != 200 {
		t.Fatalf("another token must not be locked: %d", w.Code)
	}
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	page := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	if !strings.Contains(page, "Luka") {
		t.Fatal("the parent page must not be locked by the child's API tries")
	}
	want := "5 wrong PIN tries today, last at " + r.now.In(core.Zone(r.cfg.UTCOffsetMin)).Format("15:04")
	if !strings.Contains(page, want) {
		t.Fatalf("page lacks %q", want)
	}
	if !strings.Contains(b.send("GET", "/parent?lang=ru", nil, "").Body.String(), "Сегодня неверный PIN-код вводили 5 раз") {
		t.Fatal("Russian count missing")
	}
}

func TestParentSavesTimeZone(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	form := url.Values{"a": {"0"}, "band": {"6-8"}, "browseMin": {"90"}, "videoMin": {"45"}, "bedStart": {"20:00"}, "bedEnd": {"07:00"}, "tz": {"240"}, "csrf": {b.csrf()}}
	b.send("POST", "/parent/save", form, "http://127.0.0.1:47615")
	if r.cfg.UTCOffsetMin != 240 {
		t.Fatalf("time zone not saved: %d", r.cfg.UTCOffsetMin)
	}
	for _, bad := range []string{"241", "900", "x"} {
		form.Set("tz", bad)
		form.Set("csrf", b.csrf())
		b.send("POST", "/parent/save", form, "http://127.0.0.1:47615")
		if r.cfg.UTCOffsetMin != 240 {
			t.Fatalf("bad offset %q saved", bad)
		}
	}
	if !strings.Contains(b.send("GET", "/parent?lang=en", nil, "").Body.String(), `<option value="240" selected>`) {
		t.Fatal("the saved offset is not selected")
	}
}
