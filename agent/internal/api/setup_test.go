package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"raisesteady-guard/agent/internal/core"
)

// The first-run setup on the parent page (P2-194): after the package is
// installed there is no PIN yet, and the page shows the setup and nothing else.

const (
	own      = "http://127.0.0.1:47615"
	tokWait  = "c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3"
	waitBand = "16-17"
)

// waitingRig is a server as the package's post-install step leaves it.
func waitingRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	b, ok := r.s.Bands.Get(waitBand)
	if !ok {
		t.Fatalf("no band %s", waitBand)
	}
	r.cfg.Mode = core.ModeAll
	r.cfg.Pin = core.PinHash{}
	r.cfg.SetupPending = true
	r.cfg.Accounts = []core.Account{{SID: core.AllSID, Name: "*", Token: tokWait, Settings: core.PendingSettings(b)}}
	if err := r.cfg.Validate(r.s.Bands); err != nil {
		t.Fatal(err)
	}
	r.s.WholeComputer = true
	return r
}

func newBrowser(t *testing.T, r *rig) *browser {
	return &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
}

func setupForm(b *browser) url.Values {
	return url.Values{"csrf": {b.csrf()}, "lang": {"en"}, "agree": {"yes"}, "band": {"9-12"}, "pinNew": {"2468"}, "pinAgain": {"2468"}, "dns": {"on"}}
}

func TestSetupPageIsAllThereIsWhileWaiting(t *testing.T) {
	r := waitingRig(t)
	b := newBrowser(t, r)
	w := b.send("GET", "/parent", nil, "")
	page := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	for _, want := range []string{
		"<title>Set up RaiseSteady Guard", `action="/parent/setup"`, "Browsers take the safe settings (safe search, the family filter, no private windows) when they next start.",
		"NO GUARANTEES", `name="agree" value="yes" required>`, "I have read the terms and I agree.",
		`<option value="">Choose the age</option>`, `<option value="3-5">`, `<option value="6-8">`, `<option value="9-12">`, `<option value="13-15">`, `<option value="16-17">`,
		`name="pinNew"`, `name="pinAgain"`, `pattern="[0-9]{4,8}"`,
		`name="dns" value="on"`, "Finish the setup",
		"Your browser is managed by your organization", "for every account on this computer, including yours",
		"These accounts are administrators and can turn the protection off: Parent.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("setup page lacks %q", want)
		}
	}
	for _, not := range []string{`action="/parent/login"`, `action="/parent/save"`, `action="/parent/pin"`, "Log out", tokWait, "raisesteady-guard setup", "terminal"} {
		if strings.Contains(page, not) {
			t.Errorf("setup page must not have %q", not)
		}
	}
	if !strings.Contains(page, `name="dns" value="on" checked`) {
		t.Error("the family filter must be ticked to start with")
	}
	if strings.Contains(page, ` selected`) {
		t.Error("no age may be chosen for the parent")
	}
	if strings.Contains(page, `name="agree" value="yes" required checked`) {
		t.Error("the terms box must start empty")
	}
	if strings.Contains(page, "current-password") || strings.Contains(page, "new-password") {
		t.Error("PIN inputs must not ask the browser to save the PIN")
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("headers: CSP %q, cache %q", csp, w.Header().Get("Cache-Control"))
	}
	c := b.cookies[sessionCookie]
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/parent" {
		t.Fatalf("setup cookie: %+v", c)
	}
	// The same visitor keeps one form session.
	first := c.Value
	b.send("GET", "/parent?lang=ka", nil, "")
	if b.cookies[sessionCookie].Value != first {
		t.Error("a second visit made a second session")
	}
	for lang, wants := range map[string][]string{
		"ka": {`lang="ka"`, "დაყენების დასრულება", "პირობებს გავეცანი და ვეთანხმები.", "აირჩიეთ ასაკი"},
		"ru": {`lang="ru"`, "Завершить настройку", "Я прочитал(а) условия и принимаю их.", "Выберите возраст"},
	} {
		p := b.send("GET", "/parent?lang="+lang, nil, "").Body.String()
		for _, want := range wants {
			if !strings.Contains(p, want) {
				t.Errorf("%s setup page lacks %q", lang, want)
			}
		}
		if strings.Contains(p, "Finish the setup") {
			t.Errorf("%s setup page has English button text", lang)
		}
	}
	if r.saves != 0 {
		t.Error("looking at the page saved something")
	}
}

func TestSetupStateAnswer(t *testing.T) {
	r := waitingRig(t)
	get := func() (int, map[string]any, string) {
		w := r.do(req{path: "/parent/state"})
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m, w.Body.String()
	}
	code, m, raw := get()
	if code != 200 || m["app"] != "RaiseSteady Guard" || m["setup"] != true || len(m) != 2 {
		t.Fatalf("state: %d %s", code, raw)
	}
	if strings.Contains(raw, tokWait) {
		t.Fatal("the state answer carries the token")
	}
	if w := r.do(req{method: "POST", path: "/parent/state", body: "{}"}); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST state: %d", w.Code)
	}
	if w := r.do(req{path: "/parent/state", host: "evil.example"}); w.Code == 200 {
		t.Error("the state answers under a foreign host name")
	}
	r.cfg.SetupPending = false
	if _, m, _ := get(); m["setup"] != false {
		t.Errorf("after the setup: %v", m)
	}
}

func TestSetupRefusesWhatIsMissing(t *testing.T) {
	r := waitingRig(t)
	b := newBrowser(t, r)
	post := func(f url.Values) string {
		t.Helper()
		w := b.send("POST", "/parent/setup", f, own)
		if w.Code != http.StatusSeeOther {
			t.Fatalf("status %d", w.Code)
		}
		return w.Header().Get("Location")
	}
	change := func(k, v string) url.Values {
		f := setupForm(b)
		if v == "" {
			f.Del(k)
		} else {
			f.Set(k, v)
		}
		return f
	}
	cases := []struct {
		name string
		form url.Values
		err  string
	}{
		{"no form token", change("csrf", ""), "err=stale"},
		{"a wrong form token", change("csrf", strings.Repeat("0", 64)), "err=stale"},
		{"terms not agreed", change("agree", ""), "err=setup_agree"},
		{"no age", change("band", ""), "err=setup_age"},
		{"an unknown age", change("band", "1-2"), "err=setup_age"},
		{"PINs differ", change("pinAgain", "2469"), "err=setup_pin"},
		{"a short PIN", func() url.Values { f := setupForm(b); f.Set("pinNew", "12"); f.Set("pinAgain", "12"); return f }(), "err=setup_pin"},
		{"letters in the PIN", func() url.Values { f := setupForm(b); f.Set("pinNew", "12ab"); f.Set("pinAgain", "12ab"); return f }(), "err=setup_pin"},
		{"no PIN", func() url.Values { f := setupForm(b); f.Del("pinNew"); f.Del("pinAgain"); return f }(), "err=setup_pin"},
	}
	for _, c := range cases {
		loc := post(c.form)
		if !strings.Contains(loc, c.err) || !strings.HasSuffix(loc, "#msg") {
			t.Errorf("%s: redirected to %s, want %s", c.name, loc, c.err)
		}
		if strings.Contains(loc, "2468") || strings.Contains(loc, "pin") && !strings.Contains(loc, "err=setup_pin") {
			t.Errorf("%s: the PIN is in the address: %s", c.name, loc)
		}
	}
	if r.saves != 0 || !r.cfg.SetupPending || r.cfg.Pin.Check("2468") {
		t.Fatalf("a refused form changed the configuration: saves=%d", r.saves)
	}

	// The message is under the button, and what the parent chose is kept.
	f := setupForm(b)
	f.Set("pinAgain", "0000")
	f.Set("band", "13-15")
	f.Del("dns")
	loc := post(f)
	page := b.send("GET", loc, nil, "").Body.String()
	msg := strings.Index(page, "The PIN must be 4 to 8 digits, and both must be the same.")
	btn := strings.Index(page, "Finish the setup</button>")
	if msg < 0 || btn < 0 || msg < btn {
		t.Errorf("the message must come under the button (button at %d, message at %d)", btn, msg)
	}
	if !strings.Contains(page, `<option value="13-15" selected>`) || !strings.Contains(page, `name="agree" value="yes" required checked`) {
		t.Error("the chosen age and the tick are lost after an error")
	}
	if strings.Contains(page, `name="dns" value="on" checked`) {
		t.Error("the family filter choice (off) is lost after an error")
	}
	if strings.Contains(page, "0000") || strings.Contains(page, `value="2468"`) {
		t.Error("a PIN is written back into the page")
	}
	// A message key that is not ours shows nothing.
	if p := b.send("GET", "/parent?err=%3Cb%3Ehello&lang=en", nil, "").Body.String(); strings.Contains(p, "hello") {
		t.Error("a message from the address is shown")
	}

	// Other sites cannot send the form.
	for _, o := range []string{"https://evil.example", "null"} {
		if w := b.send("POST", "/parent/setup", setupForm(b), o); w.Code != http.StatusForbidden {
			t.Errorf("setup from %s: %d", o, w.Code)
		}
	}
	hr := httptest.NewRequest("POST", base+"/parent/setup", strings.NewReader(setupForm(b).Encode()))
	hr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hr.Header.Set("Sec-Fetch-Site", "cross-site")
	for _, c := range b.cookies {
		hr.AddCookie(c)
	}
	rw := httptest.NewRecorder()
	r.s.Handler().ServeHTTP(rw, hr)
	if rw.Code != http.StatusForbidden {
		t.Errorf("cross-site setup: %d", rw.Code)
	}
	if w := b.send("GET", "/parent/setup", nil, ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET setup: %d", w.Code)
	}
	// A form without the page's cookie (another browser's token) is stale.
	other := newBrowser(t, r)
	if w := other.send("POST", "/parent/setup", setupForm(b), own); !strings.Contains(w.Header().Get("Location"), "err=stale") {
		t.Errorf("a form without its cookie: %s", w.Header().Get("Location"))
	}
	if r.saves != 0 || !r.cfg.SetupPending {
		t.Fatal("something was saved")
	}
}

func TestSetupFinishes(t *testing.T) {
	r := waitingRig(t)
	b := newBrowser(t, r)
	other := newBrowser(t, r) // a second window that was left open on the form
	otherForm := setupForm(other)
	otherForm.Set("pinNew", "9999")
	otherForm.Set("pinAgain", "9999")

	f := setupForm(b)
	f.Del("dns") // the parent switches the family filter off
	setupCookie := b.cookies[sessionCookie].Value
	w := b.send("POST", "/parent/setup", f, own)
	loc := w.Header().Get("Location")
	if w.Code != http.StatusSeeOther || !strings.Contains(loc, "ok=setup_done") || !strings.Contains(loc, "sec=setup") {
		t.Fatalf("finish: %d %s", w.Code, loc)
	}
	cfg := r.cfg
	if cfg.SetupPending || !cfg.Pin.Check("2468") || cfg.Pin.Check("9999") || r.saves != 1 || cfg.SettingsVersion != 2 {
		t.Fatalf("config after setup: pending=%v saves=%d version=%d", cfg.SetupPending, r.saves, cfg.SettingsVersion)
	}
	band, _ := r.s.Bands.Get("9-12")
	a := cfg.Accounts[0]
	if a.Token != tokWait || a.SID != core.AllSID {
		t.Error("the account or its token changed: the browsers already have the token")
	}
	if a.Band != "9-12" || a.BrowseMin != band.BrowseMin || a.VideoMin != band.VideoMin || a.Bedtime.Start != band.Bedtime.Start || a.GameMin != nil || !a.FamilyDNSOff {
		t.Errorf("the chosen age's limits must apply: %+v", a.Settings)
	}
	// The parent is signed in with a new session; the form's cookie is dead.
	c := b.cookies[sessionCookie]
	if c == nil || c.Value == setupCookie || !core.ValidToken(c.Value) {
		t.Fatalf("no new session after the setup: %+v", c)
	}
	page := b.send("GET", loc, nil, "").Body.String()
	for _, want := range []string{"The setup is finished and the limits apply now.", "Log out", `action="/parent/save"`, "Your browser is managed by your organization"} {
		if !strings.Contains(page, want) {
			t.Errorf("the settings page after setup lacks %q", want)
		}
	}
	if strings.Contains(page, `action="/parent/setup"`) {
		t.Error("the setup form is still shown")
	}

	// The second window: its form no longer works and it is not signed in.
	w = other.send("POST", "/parent/setup", otherForm, own)
	if r.saves != 1 || !r.cfg.Pin.Check("2468") {
		t.Fatal("a second setup changed the PIN")
	}
	if p := other.send("GET", "/parent", nil, "").Body.String(); !strings.Contains(p, `action="/parent/login"`) || strings.Contains(p, "Log out") {
		t.Error("the second window must see the PIN question")
	}
	// From here the PIN guards the page as always.
	fresh := newBrowser(t, r)
	if w := fresh.login("1357"); !strings.Contains(w.Header().Get("Location"), "err=wrong_pin") {
		t.Errorf("wrong PIN after setup: %s", w.Header().Get("Location"))
	}
	fresh.login("2468")
	if !strings.Contains(fresh.send("GET", "/parent", nil, "").Body.String(), "Log out") {
		t.Error("the new PIN does not open the page")
	}
}

func TestSetupKeepsTheFamilyFilterWhenTicked(t *testing.T) {
	r := waitingRig(t)
	b := newBrowser(t, r)
	b.send("POST", "/parent/setup", setupForm(b), own)
	if r.cfg.SetupPending || r.cfg.Accounts[0].FamilyDNSOff {
		t.Fatalf("pending=%v dnsOff=%v", r.cfg.SetupPending, r.cfg.Accounts[0].FamilyDNSOff)
	}
}

// The form's session opens nothing else: every other form of the page still
// needs a parent who knows the PIN, and there is no PIN to guess yet.
func TestSetupSessionIsNotAParentSession(t *testing.T) {
	r := waitingRig(t)
	b := newBrowser(t, r)
	csrf := b.csrf()
	before := r.cfg.Clone()
	for path, f := range map[string]url.Values{
		"/parent/save":     {"csrf": {csrf}, "a": {"0"}, "band": {"16-17"}, "browseMin": {"1440"}, "videoMin": {"1440"}, "bedStart": {"00:00"}, "bedEnd": {"00:00"}},
		"/parent/pin":      {"csrf": {csrf}, "pinOld": {""}, "pinNew": {"1111"}, "pinAgain": {"1111"}},
		"/parent/timezone": {"csrf": {csrf}, "utcOffset": {"240"}},
		"/parent/logout":   {"csrf": {csrf}},
	} {
		w := b.send("POST", path, f, own)
		if w.Code == 200 || strings.Contains(w.Header().Get("Location"), "ok=") {
			t.Errorf("%s with the form's session: %d %s", path, w.Code, w.Header().Get("Location"))
		}
	}
	if r.saves != 0 || r.cfg.SettingsVersion != before.SettingsVersion || r.cfg.Pin.Check("1111") || r.cfg.UTCOffsetMin != before.UTCOffsetMin {
		t.Fatal("the form's session changed the settings")
	}

	// No PIN exists: the PIN question answers "no" and counts nothing, so
	// nobody is locked out before the parent has chosen a PIN.
	for i := 0; i < core.MaxTries+3; i++ {
		w := b.send("POST", "/parent/login", url.Values{"pin": {"0000"}, "lang": {"en"}}, own)
		if w.Code != http.StatusSeeOther || strings.Contains(w.Header().Get("Location"), "locked") {
			t.Fatalf("login while waiting: %d %s", w.Code, w.Header().Get("Location"))
		}
		rw := r.do(req{method: "POST", path: "/v1/pin", token: tokWait, body: `{"pin":"0000"}`})
		var m map[string]any
		_ = json.Unmarshal(rw.Body.Bytes(), &m)
		if rw.Code != 200 || m["ok"] != false || m["locked"] == true || m["triesLeft"] != float64(core.MaxTries) {
			t.Fatalf("extension PIN check while waiting: %d %v", rw.Code, m)
		}
	}
	if !strings.Contains(b.send("GET", "/parent", nil, "").Body.String(), `action="/parent/setup"`) {
		t.Fatal("the setup page is gone")
	}
	// After the setup the first wrong try is the first one counted.
	b.send("POST", "/parent/setup", setupForm(b), own)
	rw := r.do(req{method: "POST", path: "/v1/pin", token: tokWait, body: `{"pin":"0000"}`})
	var m map[string]any
	_ = json.Unmarshal(rw.Body.Bytes(), &m)
	if m["ok"] != false || m["triesLeft"] != float64(core.MaxTries-1) {
		t.Errorf("first wrong PIN after the setup: %v", m)
	}
}

// While the setup waits the extension gets a normal state with no limit, so
// the store's version works unchanged.
func TestStateWhileWaitingHasNoLimits(t *testing.T) {
	r := waitingRig(t)
	body := r.do(req{path: "/v1/state", token: tokWait}).Body.Bytes()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	lim, _ := m["limits"].(map[string]any)
	if lim["browseMin"] != float64(1440) || lim["videoMin"] != float64(1440) {
		t.Errorf("limits before the setup = %v, want a whole day", m["limits"])
	}
	if m["bedtimeActive"] != false || m["paused"] != false || m["homework"] != false {
		t.Errorf("before the setup: bedtimeActive %v, paused %v, homework %v", m["bedtimeActive"], m["paused"], m["homework"])
	}
	// No bedtime is JSON null, at the top and in the schedule. The extension
	// reads two equal times as a missing value and would then show, and warn
	// about, the age group's own bedtime (P2-194 review, must-fix 1).
	if v, has := m["bedtime"]; !has || v != nil {
		t.Errorf("bedtime before the setup = %v, want null", v)
	}
	school, _ := m["schedule"].(map[string]any)["school"].(map[string]any)
	if v, has := school["bedtime"]; !has || v != nil {
		t.Errorf("schedule.school.bedtime before the setup = %v, want null", v)
	}
	if m["setup"] != true {
		t.Errorf("setup = %v, want true while the setup waits", m["setup"])
	}

	// After the setup the chosen age's bedtime is sent as two times, and the
	// setup mark is gone.
	b := newBrowser(t, r)
	b.send("POST", "/parent/setup", setupForm(b), own)
	m = map[string]any{}
	if err := json.Unmarshal(r.do(req{path: "/v1/state", token: tokWait}).Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	bt, _ := m["bedtime"].(map[string]any)
	if bt["start"] == nil || bt["start"] == bt["end"] {
		t.Errorf("bedtime after the setup = %v", m["bedtime"])
	}
	if v, has := m["setup"]; has {
		t.Errorf("setup after the setup = %v, want the field gone", v)
	}
}

// Yandex Browser is Chromium: the store's extension reports "chrome". The
// agent tells them apart by the browser's own name in the request.
func TestYandexBrowserGetsItsOwnLine(t *testing.T) {
	r := newRig(t)
	const ya = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 YaBrowser/24.10.0.0 Safari/537.36"
	const cr = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	post := func(tok, browser, ua string) int {
		return r.do(req{method: "POST", path: "/v1/usage", token: tok, body: `{"browser":"` + browser + `","browseSec":1}`, hdr: map[string]string{"User-Agent": ua}}).Code
	}
	if c := post(tokA, "chrome", ya); c != 200 {
		t.Fatalf("usage from Yandex Browser: %d", c)
	}
	if c := post(tokB, "chrome", cr); c != 200 {
		t.Fatalf("usage from Chrome: %d", c)
	}
	seen := func(sid, id string) bool { _, ok := r.s.Store.LastSeen(sid, id); return ok }
	if !seen(kidA, "yandex") || seen(kidA, "chrome") {
		t.Error("Yandex Browser was counted as Chrome")
	}
	if !seen(kidB, "chrome") || seen(kidB, "yandex") {
		t.Error("Chrome was counted as Yandex Browser")
	}
	// A newer extension says "yandex" itself; Firefox is never renamed.
	if c := post(tokB, "yandex", ya); c != 200 || !seen(kidB, "yandex") {
		t.Errorf("an extension that reports yandex: %d", c)
	}
	if c := post(tokA, "firefox", ya); c != 200 || !seen(kidA, "firefox") {
		t.Errorf("firefox with an odd name: %d", c)
	}
}
