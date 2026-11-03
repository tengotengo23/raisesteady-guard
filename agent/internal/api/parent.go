package api

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"raisesteady-guard/agent/internal/core"
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
	sessionTTL  = 10 * time.Minute
	maxSessions = 16
	maxForm     = 64 << 10
)

type session struct {
	csrf string
	exp  time.Time
}

func (s *Server) parentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/parent", s.handleParent)
	mux.HandleFunc("/parent/app.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(appCSS)
	})
	mux.HandleFunc("/parent/login", s.post(false, s.handleLogin))
	mux.HandleFunc("/parent/logout", s.post(true, s.handleLogout))
	mux.HandleFunc("/parent/save", s.post(true, s.handleSave))
	mux.HandleFunc("/parent/defaults", s.post(true, s.handleDefaults))
	mux.HandleFunc("/parent/accounts", s.post(true, s.handleAccounts))
	mux.HandleFunc("/parent/pin", s.post(true, s.handleChangePin))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/parent", http.StatusSeeOther)
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

// session returns the parent's session, sliding its expiry.
func (s *Server) session(r *http.Request) (string, *session) {
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

func (s *Server) newSession() (string, *session) {
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
	ss := &session{csrf: core.NewToken(), exp: s.now().Add(sessionTTL)}
	s.sessions[tok] = ss
	return tok, ss
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
	Report               []store.DayReport
	TopSites             []store.DomainTime
	ByReason             []reasonCount
	Browsers             []browserView
	TZ                   []tzOpt
	PinToday             string
	LocalAccounts        []acctOpt
	Everyone             bool
	WholeComputer        bool
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
		case "stale", "bad_input", "no_accounts", "pin_mismatch":
			d.Msg = T(lang, k)
		default:
			d.MsgKind = ""
		}
	} else if k := q.Get("ok"); k == "saved" || k == "pin_changed" {
		d.MsgKind, d.Msg = "ok", T(lang, k)
	}
	switch sec := q.Get("sec"); sec {
	case "limits", "accounts", "pin":
		d.MsgSec = sec
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
	d.Report, d.TopSites = s.Store.Report(acct.SID, 7)
	tot := map[string]int{}
	for _, day := range d.Report {
		for r, c := range day.ByReason {
			tot[r] += c
		}
	}
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
