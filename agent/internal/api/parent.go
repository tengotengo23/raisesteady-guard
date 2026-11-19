package api

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/news"
	"raisesteady-guard/agent/internal/store"
)

//go:embed web/*
var webFS embed.FS

var pageTmpl = template.Must(template.New("parent.html").Funcs(template.FuncMap{
	"t":  T,
	"tf": func(lang, key string, a ...any) string { return fmt.Sprintf(T(lang, key), a...) },
}).ParseFS(webFS, "web/parent.html"))

var appCSS = func() []byte {
	tok, _ := webFS.ReadFile("web/tokens.css")
	css, _ := webFS.ReadFile("web/parent.css")
	return append(append(tok, '\n'), css...)
}()

const (
	sessionCookie = "kg_parent"
	// sessionTTL is absolute: the parent signs in again after 10 minutes.
	sessionTTL = 10 * time.Minute
	// setupTTL: a first-run form stays valid while the parent reads the terms.
	setupTTL    = 2 * time.Hour
	maxSessions = 16
	maxForm     = 64 << 10
)

type session struct {
	csrf string
	exp  time.Time
	// setup marks the session of the first-run form (P2-194). It carries the
	// form's CSRF token and nothing else: it never counts as signed in.
	setup bool
}

func (s *Server) parentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/parent", s.handleParent)
	mux.HandleFunc("/parent/app.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(appCSS)
	})
	mux.HandleFunc("/parent/state", s.handleSetupState)
	mux.HandleFunc("/parent/setup", s.post(false, s.handleSetup))
	mux.HandleFunc("/parent/login", s.post(false, s.handleLogin))
	mux.HandleFunc("/parent/logout", s.post(true, s.handleLogout))
	mux.HandleFunc("/parent/save", s.post(true, s.handleSave))
	mux.HandleFunc("/parent/defaults", s.post(true, s.handleDefaults))
	mux.HandleFunc("/parent/accounts", s.post(true, s.handleAccounts))
	mux.HandleFunc("/parent/pin", s.post(true, s.handleChangePin))
	mux.HandleFunc("/parent/pause", s.post(true, s.handlePause))
	mux.HandleFunc("/parent/news", s.post(true, s.handleNews))
	mux.HandleFunc("/parent/request", s.post(true, s.handleRequestAnswer))
	mux.HandleFunc("/parent/apps", s.post(true, s.handleApps))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/parent", http.StatusSeeOther)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			// No icon: an empty answer, so the browser logs no error on the parent page.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
}

func (s *Server) htmlHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	h.Set("X-Frame-Options", "DENY")
}

func (s *Server) selfOrigin(o string) bool {
	p := strconv.Itoa(s.Port)
	return o == "http://127.0.0.1:"+p || o == "http://localhost:"+p
}

// session returns the signed-in parent's session. A first-run form's
// session is never one.
func (s *Server) session(r *http.Request) (string, *session) {
	if tok, ss := s.anySession(r); ss != nil && !ss.setup {
		return tok, ss
	}
	return "", nil
}

// setupSession returns the session of the first-run form.
func (s *Server) setupSession(r *http.Request) (string, *session) {
	if tok, ss := s.anySession(r); ss != nil && ss.setup {
		return tok, ss
	}
	return "", nil
}

func (s *Server) anySession(r *http.Request) (string, *session) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || !core.ValidToken(c.Value) {
		return "", nil
	}
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	now := s.now()
	for k, v := range s.sessions {
		if now.After(v.exp) {
			delete(s.sessions, k)
		}
	}
	for k, v := range s.sessions {
		if core.ConstantTimeEqual(k, c.Value) {
			return k, v
		}
	}
	return "", nil
}

func (s *Server) newSession() (string, *session) { return s.addSession(false) }

func (s *Server) addSession(setup bool) (string, *session) {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]*session{}
	}
	if len(s.sessions) >= maxSessions {
		var oldest string
		for k, v := range s.sessions {
			if oldest == "" || v.exp.Before(s.sessions[oldest].exp) {
				oldest = k
			}
		}
		delete(s.sessions, oldest)
	}
	tok := core.NewToken()
	ss := &session{csrf: core.NewToken(), exp: s.now().Add(sessionTTL), setup: setup}
	if setup {
		ss.exp = s.now().Add(setupTTL)
	}
	s.sessions[tok] = ss
	return tok, ss
}

// First-run setup on the parent page (P2-194) --------------------------------
//
// A package install starts the protection at once and leaves the config
// "pending": no PIN, no age. Until the parent finishes the form, /parent
// shows only that form. Anyone at this computer can open it (there is no PIN
// yet); a web page cannot post it (same-origin checks and the form's token).

var errSetupDone = errors.New("the setup is already finished")

// handleSetupState answers the launcher: is this RaiseSteady Guard, and does
// the first-run setup still wait? It says nothing else.
func (s *Server) handleSetupState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"app": core.AppName, "setup": s.Config().SetupPending})
}

// AskSetupState asks the service on this computer whether the first-run
// setup still waits. An error means the service does not answer.
func AskSetupState(port int) (bool, error) {
	c := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/parent/state")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var out struct {
		App   string `json:"app"`
		Setup bool   `json:"setup"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&out) != nil || out.App != core.AppName {
		return false, errors.New("another program answers on the port")
	}
	return out.Setup, nil
}

// setupRedirect goes back to the form with a message (shown under the
// button, so the address ends at the message) and keeps what was chosen.
func (s *Server) setupRedirect(w http.ResponseWriter, r *http.Request, lang, msg string) {
	q := url.Values{}
	q.Set("lang", lang)
	q.Set("err", msg)
	f := r.PostForm
	if b := f.Get("band"); b != "" {
		q.Set("band", b)
	}
	if f.Get("agree") == "yes" {
		q.Set("agree", "1")
	}
	if f.Get("dns") != "on" {
		q.Set("dns", "off")
	}
	if f.Get("news") != "on" {
		q.Set("news", "off")
	}
	http.Redirect(w, r, "/parent?"+q.Encode()+"#msg", http.StatusSeeOther)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	if !s.Config().SetupPending {
		s.redirect(w, r, lang, "", "")
		return
	}
	f := r.PostForm
	_, sess := s.setupSession(r)
	if sess == nil || !core.ConstantTimeEqual(f.Get("csrf"), sess.csrf) {
		s.setupRedirect(w, r, lang, "stale")
		return
	}
	if f.Get("agree") != "yes" {
		s.setupRedirect(w, r, lang, "setup_agree")
		return
	}
	var band *core.Band
	for i := range s.Bands.List {
		if s.Bands.List[i].ID == f.Get("band") {
			band = &s.Bands.List[i]
		}
	}
	if band == nil {
		s.setupRedirect(w, r, lang, "setup_age")
		return
	}
	p1, p2 := f.Get("pinNew"), f.Get("pinAgain")
	if p1 != p2 || !core.ValidPin(p1) {
		s.setupRedirect(w, r, lang, "setup_pin")
		return
	}
	h, err := core.HashPin(p1)
	if err != nil {
		s.setupRedirect(w, r, lang, "setup_pin")
		return
	}
	st := core.SettingsForBand(*band)
	st.FamilyDNSOff = f.Get("dns") != "on"
	newsChoice := core.NewsOff
	if f.Get("news") == "on" {
		newsChoice = core.NewsOn
	}
	err = s.update(func(c *core.Config) error {
		if !c.SetupPending {
			return errSetupDone
		}
		c.Pin, c.SetupPending = h, false
		c.News = newsChoice
		for i := range c.Accounts {
			c.Accounts[i].Settings = st
		}
		return nil
	})
	if errors.Is(err, errSetupDone) {
		s.redirect(w, r, lang, "", "")
		return
	}
	if err != nil {
		s.setupRedirect(w, r, lang, "bad_input")
		return
	}
	// The form's sessions end here; the parent who finished is signed in.
	s.sessMu.Lock()
	for k, v := range s.sessions {
		if v.setup {
			delete(s.sessions, k)
		}
	}
	s.sessMu.Unlock()
	tok, _ := s.newSession()
	setCookie(w, sessionCookie, tok, 0)
	s.redirect(w, r, lang, "ok", "setup_done", "sec", "setup")
}

type termBlock struct {
	Head bool
	Text string
}

// termBlocks turns the terms text (lines; "# " starts a heading) into blocks.
func termBlocks(text string) []termBlock {
	var out []termBlock
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
		case strings.HasPrefix(l, "# "):
			out = append(out, termBlock{Head: true, Text: strings.TrimPrefix(l, "# ")})
		default:
			out = append(out, termBlock{Text: l})
		}
	}
	return out
}

// setupPage shows the first-run form and nothing else.
func (s *Server) setupPage(w http.ResponseWriter, r *http.Request, d pageData, lang string) {
	q := r.URL.Query()
	d.Setup, d.Title = true, T(lang, "setup_title")
	d.MsgKind, d.Msg = "", ""
	switch k := q.Get("err"); k {
	case "stale", "bad_input", "setup_age", "setup_pin":
		d.MsgKind, d.Msg = "err", T(lang, k)
	case "setup_agree":
		d.MsgKind, d.Msg = "err", T(lang, "setup_agree_err")
	}
	tok, sess := s.setupSession(r)
	if sess == nil {
		tok, sess = s.addSession(true)
		setCookie(w, sessionCookie, tok, 0)
	}
	d.CSRF = sess.csrf
	d.Terms = termBlocks(core.Terms(lang))
	for _, b := range s.Bands.List {
		d.Bands = append(d.Bands, bandOpt{ID: b.ID, Label: fmt.Sprintf(T(lang, "years"), strings.ReplaceAll(b.ID, "-", "–")), Selected: b.ID == q.Get("band")})
	}
	d.SetupAgree = q.Get("agree") == "1"
	d.SetupDNS = q.Get("dns") != "off"
	d.SetupNews = q.Get("news") != "off"
	d.WholeComputer = s.WholeComputer
	if s.LocalAccounts != nil {
		var names []string
		for _, la := range s.LocalAccounts() {
			if la.Admin {
				names = append(names, la.Name)
			}
		}
		d.SetupAdmins = strings.Join(names, ", ")
	}
	s.htmlHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	_ = pageTmpl.Execute(w, d)
}

type postFunc func(w http.ResponseWriter, r *http.Request, lang string, sess *session, sessTok string)

// post wraps a parent-page form: POST only, same-origin only, form size cap,
// and (when needAuth) a valid session plus its CSRF token.
func (s *Server) post(needAuth bool, fn postFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !s.selfOrigin(o) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if sf := r.Header.Get("Sec-Fetch-Site"); sf != "" && sf != "same-origin" && sf != "none" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxForm)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		lang := r.PostForm.Get("lang")
		if !validLang(lang) {
			lang = pickLang(r)
		}
		var tok string
		var sess *session
		if needAuth {
			tok, sess = s.session(r)
			if sess == nil {
				s.redirect(w, r, lang, "", "")
				return
			}
			if !core.ConstantTimeEqual(r.PostForm.Get("csrf"), sess.csrf) {
				s.redirect(w, r, lang, "err", "stale")
				return
			}
		}
		fn(w, r, lang, sess, tok)
	}
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, lang, kind, msg string, extra ...string) {
	q := url.Values{}
	q.Set("lang", lang)
	if kind != "" {
		q.Set(kind, msg)
	}
	for i := 0; i+1 < len(extra); i += 2 {
		q.Set(extra[i], extra[i+1])
	}
	http.Redirect(w, r, "/parent?"+q.Encode(), http.StatusSeeOther)
}

func setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/parent", MaxAge: maxAge, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	cfg := s.Config()
	if cfg.SetupPending {
		// No PIN exists yet: the page shows the first-run form.
		s.redirect(w, r, lang, "", "")
		return
	}
	ok, left, retry, err := s.Lock.VerifyKey("parent", cfg.Pin, r.PostForm.Get("pin"))
	switch {
	case ok:
		tok, _ := s.newSession()
		// A session cookie (no Max-Age): it ends with the browser, and the
		// server forgets the session after 10 minutes anyway.
		setCookie(w, sessionCookie, tok, 0)
		s.redirect(w, r, lang, "", "")
	case errors.Is(err, core.ErrLocked):
		s.redirect(w, r, lang, "err", "locked", "wait", strconv.Itoa(int(retry.Minutes()+0.999)))
	default:
		s.redirect(w, r, lang, "err", "wrong_pin", "left", strconv.Itoa(left))
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, lang string, _ *session, tok string) {
	s.sessMu.Lock()
	delete(s.sessions, tok)
	s.sessMu.Unlock()
	setCookie(w, sessionCookie, "", -1)
	s.redirect(w, r, lang, "", "")
}

func formAccount(r *http.Request, cfg core.Config) int {
	i, err := strconv.Atoi(r.PostForm.Get("a"))
	if err != nil || i < 0 || i >= len(cfg.Accounts) {
		return -1
	}
	return i
}

func atoiRange(s string, lo, hi int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil && n >= lo && n <= hi
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	f := r.PostForm
	cfg := s.Config()
	i := formAccount(r, cfg)
	a := strconv.Itoa(i)
	if i < 0 {
		s.redirect(w, r, lang, "err", "bad_input")
		return
	}
	browse, ok1 := atoiRange(f.Get("browseMin"), 5, 720)
	video, ok2 := atoiRange(f.Get("videoMin"), 5, 720)
	allow, err1 := parseDomains(f.Get("allow"))
	block, err2 := parseDomains(f.Get("block"))
	bs, be := f.Get("bedStart"), f.Get("bedEnd")
	tz, ok3 := atoiRange(f.Get("tz"), -720, 840)
	if f.Get("tz") == "" {
		tz, ok3 = cfg.UTCOffsetMin, true
	}
	if _, ok := s.Bands.Get(f.Get("band")); !ok || !ok1 || !ok2 || !ok3 || !core.ValidOffset(tz) || err1 != nil || err2 != nil || !core.ValidHHMM(bs) || !core.ValidHHMM(be) {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "limits")
		return
	}
	// Weekend and homework time (P2-160): checked only when switched on; when
	// off, good values are kept for next time.
	old := cfg.Accounts[i].Settings
	wk := old.Weekend
	wk.On = f.Get("weekendOn") == "on"
	wb, okb := atoiRange(f.Get("wBrowseMin"), 5, 720)
	wv, okv := atoiRange(f.Get("wVideoMin"), 5, 720)
	wbs, wbe := f.Get("wBedStart"), f.Get("wBedEnd")
	wOK := okb && okv && core.ValidHHMM(wbs) && core.ValidHHMM(wbe)
	if wOK {
		wk.BrowseMin, wk.VideoMin, wk.Bedtime = wb, wv, core.Bedtime{Start: wbs, End: wbe}
	}
	hw := old.Homework
	hw.On = f.Get("homeworkOn") == "on"
	hs, he := f.Get("hwStart"), f.Get("hwEnd")
	hOK := core.ValidHHMM(hs) && core.ValidHHMM(he) && hs < he
	if hOK {
		hw.Start, hw.End = hs, he
	}
	if (wk.On && !wOK) || (hw.On && !hOK) {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "limits")
		return
	}
	sid := cfg.Accounts[i].SID
	err := s.update(func(c *core.Config) error {
		j := indexOfSID(c, sid)
		if j < 0 {
			return errors.New("account gone")
		}
		c.UTCOffsetMin = tz
		st := &c.Accounts[j].Settings
		st.Band, st.BrowseMin, st.VideoMin = f.Get("band"), browse, video
		st.Bedtime = core.Bedtime{Start: bs, End: be}
		st.Allow, st.Block = allow, block
		for _, d := range allow {
			st.Block = removeStr(st.Block, d)
		}
		st.FocusMode = f.Get("focusMode") == "on"
		st.Adblock = f.Get("adblock") == "on"
		st.FamilyDNSOff = f.Get("familyDns") != "on"
		st.Weekend, st.Homework = wk, hw
		st.SearchesOff = f.Get("searches") != "on"
		st.TopicsOff = f.Get("topics") != "on"
		return nil
	})
	if err != nil {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "limits")
		return
	}
	s.redirect(w, r, lang, "ok", "saved", "a", a, "sec", "limits")
}

func (s *Server) handleDefaults(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	cfg := s.Config()
	i := formAccount(r, cfg)
	if i < 0 {
		s.redirect(w, r, lang, "err", "bad_input")
		return
	}
	sid := cfg.Accounts[i].SID
	err := s.update(func(c *core.Config) error {
		j := indexOfSID(c, sid)
		if j < 0 {
			return errors.New("account gone")
		}
		st := &c.Accounts[j].Settings
		b, ok := s.Bands.Get(st.Band)
		if !ok {
			return errors.New("band")
		}
		st.BrowseMin, st.VideoMin = b.BrowseMin, b.VideoMin
		st.Bedtime = core.Bedtime{Start: b.Bedtime.Start, End: b.Bedtime.End}
		return nil
	})
	a := strconv.Itoa(i)
	if err != nil {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "limits")
		return
	}
	s.redirect(w, r, lang, "ok", "saved", "a", a, "sec", "limits")
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	f := r.PostForm
	if s.WholeComputer {
		s.redirect(w, r, lang, "err", "bad_input", "sec", "accounts")
		return
	}
	everyone := f.Get("everyone") == "on"
	known := map[string]LocalAccount{}
	if s.LocalAccounts != nil {
		for _, la := range s.LocalAccounts() {
			known[la.SID] = la
		}
	}
	var picked []string
	for _, sid := range f["sid"] {
		if _, ok := known[sid]; ok && core.ValidSID(sid) && !containsStr(picked, sid) {
			picked = append(picked, sid)
		}
	}
	if !everyone && len(picked) == 0 {
		s.redirect(w, r, lang, "err", "no_accounts", "sec", "accounts")
		return
	}
	err := s.update(func(c *core.Config) error {
		base := c.Accounts[0].Settings
		old := map[string]core.Account{}
		for _, a := range c.Accounts {
			old[a.SID] = a
		}
		var next []core.Account
		if everyone {
			c.Mode = core.ModeAll
			a, ok := old[core.AllSID]
			if !ok {
				a = core.Account{SID: core.AllSID, Name: "*", Token: core.NewToken(), Settings: base}
			}
			next = []core.Account{a}
		} else {
			c.Mode = core.ModeUsers
			for _, sid := range picked {
				a, ok := old[sid]
				if !ok {
					a = core.Account{SID: sid, Name: known[sid].Name, Token: core.NewToken(), Settings: base}
				}
				next = append(next, a)
			}
		}
		c.Accounts = next
		return nil
	})
	if err != nil {
		s.redirect(w, r, lang, "err", "bad_input", "sec", "accounts")
		return
	}
	s.redirect(w, r, lang, "ok", "saved", "sec", "accounts")
}

func (s *Server) handleChangePin(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	f := r.PostForm
	cfg := s.Config()
	ok, left, retry, err := s.Lock.VerifyKey("parent", cfg.Pin, f.Get("pinNow"))
	if errors.Is(err, core.ErrLocked) {
		s.redirect(w, r, lang, "err", "locked", "wait", strconv.Itoa(int(retry.Minutes()+0.999)), "sec", "pin")
		return
	}
	if !ok {
		s.redirect(w, r, lang, "err", "wrong_pin", "left", strconv.Itoa(left), "sec", "pin")
		return
	}
	p1, p2 := f.Get("pinNew"), f.Get("pinAgain")
	if p1 != p2 || !core.ValidPin(p1) {
		s.redirect(w, r, lang, "err", "pin_mismatch", "sec", "pin")
		return
	}
	h, err := core.HashPin(p1)
	if err != nil {
		s.redirect(w, r, lang, "err", "pin_mismatch", "sec", "pin")
		return
	}
	if err := s.update(func(c *core.Config) error { c.Pin = h; return nil }); err != nil {
		s.redirect(w, r, lang, "err", "bad_input", "sec", "pin")
		return
	}
	s.redirect(w, r, lang, "ok", "pin_changed", "sec", "pin")
}

// handlePause starts or ends "Pause the internet" (P2-160, F2).
func (s *Server) handlePause(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	cfg := s.Config()
	i := formAccount(r, cfg)
	if i < 0 {
		s.redirect(w, r, lang, "err", "bad_input")
		return
	}
	a := strconv.Itoa(i)
	now := s.now()
	until := ""
	switch d := r.PostForm.Get("dur"); d {
	case "30", "60", "120":
		n, _ := strconv.Atoi(d)
		until = now.Add(time.Duration(n) * time.Minute).Format(time.RFC3339)
	case "tomorrow":
		until = core.NextMorning(now).Format(time.RFC3339)
	case "resume":
	default:
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "pause")
		return
	}
	sid := cfg.Accounts[i].SID
	err := s.update(func(c *core.Config) error {
		j := indexOfSID(c, sid)
		if j < 0 {
			return errors.New("account gone")
		}
		c.Accounts[j].PauseUntil = until
		return nil
	})
	if err != nil {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "pause")
		return
	}
	s.redirect(w, r, lang, "ok", "pause_saved", "a", a, "sec", "pause")
}

// handleNews turns news from RaiseSteady on or off, or hides one note
// (P2-252). Turning on reads nothing here; the page reads on its next open.
func (s *Server) handleNews(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	f := r.PostForm
	cfg := s.Config()
	a := "0"
	if i := formAccount(r, cfg); i >= 0 {
		a = strconv.Itoa(i)
	}
	act, id := f.Get("action"), f.Get("id")
	if act != "on" && act != "off" && !(act == "hide" && core.ValidNewsID(id)) {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "news")
		return
	}
	err := s.update(func(c *core.Config) error {
		switch act {
		case "on":
			c.News = core.NewsOn
		case "off":
			c.News = core.NewsOff
		case "hide":
			for _, h := range c.NewsHidden {
				if h == id {
					return nil
				}
			}
			c.NewsHidden = append(c.NewsHidden, id)
			if n := len(c.NewsHidden) - core.MaxNewsHidden; n > 0 {
				c.NewsHidden = append([]string(nil), c.NewsHidden[n:]...)
			}
		}
		return nil
	})
	if err != nil {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "news")
		return
	}
	s.redirect(w, r, lang, "ok", "saved", "a", a, "sec", "news")
}

// newsView is one note as the parent page shows it.
type newsView struct {
	ID, Title, Link string
	Lines           []string
}

// fillUpdate writes the version line: version, last check, last update.
func (s *Server) fillUpdate(d *pageData, lang string) {
	if s.Update == nil {
		return
	}
	st := s.Update()
	when := func(t time.Time) string {
		if t.IsZero() {
			return T(lang, "upd_never")
		}
		return t.In(s.now().Location()).Format("2006-01-02 15:04")
	}
	d.UpdateLine = fmt.Sprintf(T(lang, "upd_line"), st.Version, when(st.LastCheck), when(st.LastUpdate))
	d.UpdateFailing = st.Failing
}

// fillNews fills the news box. Nothing is read unless the parent chose "on".
func (s *Server) fillNews(d *pageData, cfg core.Config, lang string) {
	if s.News == nil {
		return
	}
	d.NewsBox = true
	if cfg.News != core.NewsOn {
		d.NewsAsk = true
		return
	}
	for _, it := range news.Visible(s.News.Items(), s.now().Format("2006-01-02"), cfg.NewsHidden) {
		d.News = append(d.News, newsView{ID: it.ID, Title: it.TitleIn(lang), Lines: it.BodyLines(lang), Link: it.Link})
	}
}

// handleRequestAnswer answers one of the child's requests (P2-160, F6).
func (s *Server) handleRequestAnswer(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	f := r.PostForm
	cfg := s.Config()
	i := formAccount(r, cfg)
	if i < 0 {
		s.redirect(w, r, lang, "err", "bad_input")
		return
	}
	a := strconv.Itoa(i)
	sid := cfg.Accounts[i].SID
	var req *store.Request
	for _, q := range s.Store.Requests(sid) {
		if q.ID == f.Get("id") {
			q := q
			req = &q
		}
	}
	ans := f.Get("answer")
	if req == nil {
		s.redirect(w, r, lang, "err", "req_gone", "a", a, "sec", "requests")
		return
	}
	switch {
	case ans == "no":
	case req.Kind == "site" && ans == "allow":
	case req.Kind == "time" && (ans == "15" || ans == "30"):
	default:
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "requests")
		return
	}
	if _, ok := s.Store.TakeRequest(sid, req.ID); !ok {
		s.redirect(w, r, lang, "err", "req_gone", "a", a, "sec", "requests")
		return
	}
	switch ans {
	case "allow":
		err := s.update(func(c *core.Config) error {
			j := indexOfSID(c, sid)
			if j < 0 {
				return errors.New("account gone")
			}
			st := &c.Accounts[j]
			st.Block = removeStr(st.Block, req.Domain)
			if !containsStr(st.Allow, req.Domain) {
				st.Allow = append(st.Allow, req.Domain)
			}
			return nil
		})
		if err != nil {
			s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "requests")
			return
		}
	case "15", "30":
		n, _ := strconv.Atoi(ans)
		s.Store.AddExtra(sid, n)
	}
	_ = s.Store.Save()
	s.redirect(w, r, lang, "ok", "req_done", "a", a, "sec", "requests")
}

// handleApps saves the game time and each PC app's choice (P2-160, F5). Only
// choices that differ from the default are kept, so a new age band still
// moves the defaults.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request, lang string, _ *session, _ string) {
	f := r.PostForm
	cfg := s.Config()
	i := formAccount(r, cfg)
	if i < 0 {
		s.redirect(w, r, lang, "err", "bad_input")
		return
	}
	a := strconv.Itoa(i)
	gm, ok := atoiRange(f.Get("gameMin"), 0, 720)
	if !ok {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "apps")
		return
	}
	acct := cfg.Accounts[i]
	choices := map[string]string{}
	for _, app := range s.Apps {
		v := f.Get("app_" + app.ID)
		switch v {
		case core.AppAllowed, core.AppGame, core.AppBlocked:
		case "":
			v = acct.AppMode(app, s.Bands)
		default:
			s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "apps")
			return
		}
		if v != core.DefaultAppMode(app, acct.Band, s.Bands) {
			choices[app.ID] = v
		}
	}
	sid := acct.SID
	if gm > acct.GameMinutes(s.Bands) {
		s.Store.ResetGameWarned(sid)
	}
	err := s.update(func(c *core.Config) error {
		j := indexOfSID(c, sid)
		if j < 0 {
			return errors.New("account gone")
		}
		st := &c.Accounts[j].Settings
		st.GameMin = nil
		if b, ok := s.Bands.Get(st.Band); !ok || b.VideoMin != gm {
			st.GameMin = &gm
		}
		st.Apps = nil
		if len(choices) > 0 {
			st.Apps = choices
		}
		return nil
	})
	if err != nil {
		s.redirect(w, r, lang, "err", "bad_input", "a", a, "sec", "apps")
		return
	}
	s.redirect(w, r, lang, "ok", "saved", "a", a, "sec", "apps")
}

// view types for the template

type bandOpt struct {
	ID, Label string
	Selected  bool
}

type acctTab struct {
	Index   int
	Name    string
	Current bool
}

type acctOpt struct {
	SID, Name      string
	Admin, Checked bool
}

type reasonCount struct {
	Key   string
	Count int
}

type tzOpt struct {
	Value    int
	Label    string
	Selected bool
}

type browserView struct {
	Name, Account, State, Text string
}

type reqView struct {
	Account          int
	ID, Kind, Domain string
	Text, Note, When string
}

type appOpt struct {
	Value, Label string
	Selected     bool
}

type appView struct {
	ID, Name, AgeNote string
	Opts              []appOpt
}

type dayRow struct {
	store.DayReport
	Picked bool
}

type countView struct {
	Name  string
	Count int
}

type dayRuleView struct {
	On                  bool
	BrowseMin, VideoMin int
	Start, End          string
}

// tzOffsets are the UTC offsets in use (minutes).
var tzOffsets = []int{-720, -660, -600, -570, -540, -480, -420, -360, -300, -240, -210, -180, -120, -60, 0, 60, 120, 180, 210, 240, 270, 300, 330, 345, 360, 390, 420, 480, 525, 540, 570, 600, 630, 660, 720, 765, 780, 840}

type pageData struct {
	Lang, AppName, Title string
	Langs                []struct{ Code, Name string }
	LoggedIn             bool
	CSRF                 string
	Msg, MsgKind, MsgSec string
	Account              int
	AccountName          string
	Tabs                 []acctTab
	Settings             core.Settings
	Bands                []bandOpt
	AllowText, BlockText string
	Report               []dayRow
	TopSites             []store.DomainTime
	ByReason             []reasonCount
	Browsers             []browserView
	TZ                   []tzOpt
	PinToday             string
	LocalAccounts        []acctOpt
	Everyone             bool
	WholeComputer        bool
	// 0.3.0 (P2-160)
	Requests      []reqView
	MultiAccounts bool
	Paused        bool
	PausedText    string
	Weekend       dayRuleView
	Homework      dayRuleView
	SearchesOn    bool
	GameMin       int
	Apps          []appView
	Day           string
	DaySearches   []store.Count
	DayApps       []countView
	SearchesDay   string
	AppsClosedDay string
	// The first-run form (P2-194)
	Setup       bool
	Terms       []termBlock
	SetupAgree  bool
	SetupDNS    bool
	SetupAdmins string
	SetupNews   bool
	// News from RaiseSteady (P2-252)
	NewsBox bool // the box is on the page
	NewsAsk bool // not chosen yet or off: explanation and "Turn on"
	News    []newsView
	// The self-updater (P2-260)
	UpdateLine    string
	UpdateFailing bool
}

func (s *Server) handleParent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	lang := pickLang(r)
	if l := q.Get("lang"); validLang(l) {
		setCookie(w, "kg_lang", l, 365*24*3600)
	}
	cfg := s.Config()
	d := pageData{Lang: lang, AppName: core.AppName, Title: T(lang, "title"), Langs: Langs}
	// Messages come only from our fixed keys, never from the query text itself.
	if k := q.Get("err"); k != "" {
		d.MsgKind = "err"
		switch k {
		case "wrong_pin":
			n, _ := atoiRange(q.Get("left"), 0, core.MaxTries)
			d.Msg = fmt.Sprintf(T(lang, "wrong_pin"), n)
		case "locked":
			n, _ := atoiRange(q.Get("wait"), 0, 60)
			d.Msg = fmt.Sprintf(T(lang, "locked"), n)
		case "stale", "bad_input", "no_accounts", "pin_mismatch", "req_gone":
			d.Msg = T(lang, k)
		default:
			d.MsgKind = ""
		}
	} else if k := q.Get("ok"); k == "saved" || k == "pin_changed" || k == "req_done" || k == "pause_saved" || k == "setup_done" {
		d.MsgKind, d.Msg = "ok", T(lang, k)
	}
	switch sec := q.Get("sec"); sec {
	case "limits", "accounts", "pin", "pause", "requests", "apps", "setup", "news":
		d.MsgSec = sec
	}
	if cfg.SetupPending {
		s.setupPage(w, r, d, lang)
		return
	}
	_, sess := s.session(r)
	s.htmlHeaders(w)
	if sess == nil {
		d.MsgSec = "login"
		_ = pageTmpl.Execute(w, d)
		return
	}
	d.LoggedIn, d.CSRF = true, sess.csrf
	i, ok := atoiRange(q.Get("a"), 0, len(cfg.Accounts)-1)
	if !ok {
		i = 0
	}
	acct := cfg.Accounts[i]
	d.Account, d.Settings = i, acct.Settings
	d.AccountName = acct.Name
	if acct.SID == core.AllSID {
		d.AccountName = T(lang, "everyone")
		d.Everyone = true
	}
	d.WholeComputer = s.WholeComputer
	for j, a := range cfg.Accounts {
		name := a.Name
		if a.SID == core.AllSID {
			name = T(lang, "everyone")
		}
		d.Tabs = append(d.Tabs, acctTab{Index: j, Name: name, Current: j == i})
	}
	if len(d.Tabs) < 2 {
		d.Tabs = nil
	}
	for _, b := range s.Bands.List {
		d.Bands = append(d.Bands, bandOpt{ID: b.ID, Label: fmt.Sprintf(T(lang, "years"), strings.ReplaceAll(b.ID, "-", "–")), Selected: b.ID == acct.Band})
	}
	d.AllowText = strings.Join(acct.Allow, "\n")
	d.BlockText = strings.Join(acct.Block, "\n")
	rep, top := s.Store.Report(acct.SID, 7)
	d.TopSites = top
	d.Day = rep[len(rep)-1].Day
	for _, day := range rep {
		if day.Day == q.Get("day") {
			d.Day = day.Day
		}
	}
	tot := map[string]int{}
	for _, day := range rep {
		d.Report = append(d.Report, dayRow{DayReport: day, Picked: day.Day == d.Day})
		for r, c := range day.ByReason {
			tot[r] += c
		}
	}
	s.fillExtras(&d, cfg, i, lang)
	s.fillNews(&d, cfg, lang)
	s.fillUpdate(&d, lang)
	for _, r := range Reasons {
		if tot[r] > 0 {
			d.ByReason = append(d.ByReason, reasonCount{Key: "reason_" + r, Count: tot[r]})
		}
	}
	if s.Browsers != nil {
		for _, b := range s.Browsers() {
			v := browserView{Name: b.Name, Account: b.Account, State: b.State}
			switch b.State {
			case "active":
				ago := T(lang, "ago_now")
				if b.AgoMin >= 1 {
					ago = fmt.Sprintf(T(lang, "ago_min"), b.AgoMin)
				}
				v.Text = fmt.Sprintf(T(lang, "st_active"), ago)
			case "set", "waiting", "closed":
				v.Text = T(lang, "st_"+b.State)
			default:
				continue
			}
			if v.Account == core.AllSID {
				v.Account = ""
			}
			d.Browsers = append(d.Browsers, v)
		}
	}
	found := false
	for _, o := range tzOffsets {
		d.TZ = append(d.TZ, tzOpt{Value: o, Label: core.FormatOffset(o), Selected: o == cfg.UTCOffsetMin})
		found = found || o == cfg.UTCOffsetMin
	}
	if !found {
		d.TZ = append(d.TZ, tzOpt{Value: cfg.UTCOffsetMin, Label: core.FormatOffset(cfg.UTCOffsetMin), Selected: true})
	}
	if n, last := s.Lock.Today(); n > 0 {
		d.PinToday = fmt.Sprintf(T(lang, "pin_today"), n, last.In(core.Zone(cfg.UTCOffsetMin)).Format("15:04"))
	}
	protected := map[string]bool{}
	for _, a := range cfg.Accounts {
		protected[a.SID] = true
	}
	if s.LocalAccounts != nil {
		for _, la := range s.LocalAccounts() {
			d.LocalAccounts = append(d.LocalAccounts, acctOpt{SID: la.SID, Name: la.Name, Admin: la.Admin, Checked: protected[la.SID]})
		}
	}
	_ = pageTmpl.Execute(w, d)
}

// fillExtras adds the 0.3.0 parts of the page (P2-160): requests, pause,
// weekend, homework, searches, PC apps and the picked day's detail.
func (s *Server) fillExtras(d *pageData, cfg core.Config, i int, lang string) {
	now := s.now()
	acct := cfg.Accounts[i]
	d.MultiAccounts = len(cfg.Accounts) > 1
	for j, a := range cfg.Accounts {
		name := a.Name
		if a.SID == core.AllSID || name == "" {
			name = T(lang, "your_child")
		}
		for _, q := range s.Store.Requests(a.SID) {
			v := reqView{Account: j, ID: q.ID, Kind: q.Kind, Domain: q.Domain, When: q.At.In(now.Location()).Format("2006-01-02 15:04")}
			if q.Kind == "site" {
				v.Text = fmt.Sprintf(T(lang, "req_site"), name, q.Domain)
			} else {
				v.Text = fmt.Sprintf(T(lang, "req_time"), name)
			}
			if q.Note != "" {
				v.Note = fmt.Sprintf(T(lang, "req_note"), q.Note)
			}
			d.Requests = append(d.Requests, v)
		}
	}
	st := acct.Settings
	if p, until := st.PausedAt(now); p {
		d.Paused = true
		when := until.Format("15:04")
		if store.DayKey(until) != store.DayKey(now) {
			when = until.Format("2006-01-02 15:04")
		}
		d.PausedText = fmt.Sprintf(T(lang, "pause_on"), when)
	}
	d.Weekend = dayRuleView{On: st.Weekend.On, BrowseMin: st.Weekend.BrowseMin, VideoMin: st.Weekend.VideoMin, Start: st.Weekend.Bedtime.Start, End: st.Weekend.Bedtime.End}
	if d.Weekend.BrowseMin == 0 || !core.ValidHHMM(d.Weekend.Start) || !core.ValidHHMM(d.Weekend.End) {
		// Not set yet: start from the school-day values.
		d.Weekend = dayRuleView{On: st.Weekend.On, BrowseMin: st.BrowseMin, VideoMin: st.VideoMin, Start: st.Bedtime.Start, End: st.Bedtime.End}
	}
	d.Homework = dayRuleView{On: st.Homework.On, Start: st.Homework.Start, End: st.Homework.End}
	if !core.ValidHHMM(d.Homework.Start) || !core.ValidHHMM(d.Homework.End) {
		d.Homework.Start, d.Homework.End = "16:00", "18:00"
	}
	d.SearchesOn = !st.SearchesOff
	d.GameMin = st.GameMinutes(s.Bands)
	for _, app := range s.Apps {
		mode := st.AppMode(app, s.Bands)
		v := appView{ID: app.ID, Name: app.Name}
		if app.Kind == "chat" && app.MinAge > 0 {
			v.AgeNote = fmt.Sprintf(T(lang, "app_min_age"), app.MinAge)
		}
		for _, m := range []string{core.AppAllowed, core.AppGame, core.AppBlocked} {
			v.Opts = append(v.Opts, appOpt{Value: m, Label: T(lang, "app_"+m), Selected: m == mode})
		}
		d.Apps = append(d.Apps, v)
	}
	det := s.Store.Detail(acct.SID, d.Day)
	d.DaySearches = det.Searches
	for _, c := range det.AppsClosed {
		name := c.Key
		for _, app := range s.Apps {
			if app.ID == c.Key {
				name = app.Name
			}
		}
		d.DayApps = append(d.DayApps, countView{Name: name, Count: c.Count})
	}
	d.SearchesDay = fmt.Sprintf(T(lang, "searches_day"), d.Day)
	d.AppsClosedDay = fmt.Sprintf(T(lang, "apps_closed_day"), d.Day)
}
