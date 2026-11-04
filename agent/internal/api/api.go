// Package api serves the extension's localhost API (/v1/...) and the parent
// page (/parent) on 127.0.0.1 only.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/store"
)

// BrowserStatus is one line of the parent page's browser list.
type BrowserStatus struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Account string `json:"account,omitempty"` // shown when more than one account is protected
	// State: "active" (the extension reported in the last 15 minutes),
	// "set" (policies written, the extension has not reported),
	// "waiting" (policies follow at the child's next sign-in),
	// "closed" (not supported: closed in the child's session).
	State  string `json:"state"`
	AgoMin int    `json:"agoMin,omitempty"`
}

// LocalAccount is a Windows account the parent may protect.
type LocalAccount struct {
	SID   string `json:"sid"`
	Name  string `json:"name"`
	Admin bool   `json:"admin"`
}

// Server holds what the handlers need. The agent fills it in.
type Server struct {
	Port   int
	Bands  *core.Bands
	Store  *store.Store
	Lock   *core.Lockout
	Now    func() time.Time
	Config func() core.Config
	// Save validates, stores and applies a new configuration.
	Save          func(core.Config) error
	Browsers      func() []BrowserStatus
	LocalAccounts func() []LocalAccount
	// WholeComputer (Linux): browser settings are for every account, so the
	// page has no account choice and refuses one.
	WholeComputer bool
	// Apps are the PC games and chat apps the guard knows (P2-160).
	Apps []core.PCApp

	mu       sync.Mutex // serializes configuration changes
	sessMu   sync.Mutex
	sessions map[string]*session
	rateMu   sync.Mutex
	rate     map[string]*bucket
}

const (
	maxBody       = 16 << 10
	ratePerMinute = 300
)

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handler returns the HTTP handler with the host check and security headers.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/state", s.api(http.MethodGet, s.handleState))
	mux.HandleFunc("/v1/usage", s.api(http.MethodPost, s.handleUsage))
	mux.HandleFunc("/v1/pin", s.api(http.MethodPost, s.handlePin))
	mux.HandleFunc("/v1/grant", s.api(http.MethodPost, s.handleGrant))
	mux.HandleFunc("/v1/event", s.api(http.MethodPost, s.handleEvent))
	mux.HandleFunc("/v1/request", s.api(http.MethodPost, s.handleRequest))
	s.parentRoutes(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// same-origin, not no-referrer: under no-referrer a browser sends "Origin: null" with the
		// parent page's own forms, and the origin check then refuses the right PIN (P2-157).
		// Other sites still get no referrer.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if !s.hostOK(r.Host) {
			// Stops DNS rebinding: only our own host names are served.
			http.Error(w, "wrong host", http.StatusMisdirectedRequest)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) hostOK(host string) bool {
	p := strconv.Itoa(s.Port)
	return host == "127.0.0.1:"+p || host == "localhost:"+p
}

var mozOrigin = regexp.MustCompile(`^moz-extension://[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var chromeOrigin = regexp.MustCompile(`^chrome-extension://([a-p]{32})$`)

// extOrigin reports whether an Origin header is one of our extensions.
// Firefox gives each install a random UUID, so any moz-extension origin passes;
// Chromium ids are checked against ours (any id while ours is a placeholder).
func (s *Server) extOrigin(origin string) bool {
	if mozOrigin.MatchString(origin) {
		return true
	}
	m := chromeOrigin.FindStringSubmatch(origin)
	if m == nil {
		return false
	}
	ids := s.Config().ExtIDs
	if ids.Placeholder {
		return true
	}
	for _, id := range []string{ids.ChromiumDevID, ids.ChromeStoreID, ids.EdgeStoreID} {
		if id != "" && id == m[1] {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

type bucket struct {
	start time.Time
	n     int
}

func (s *Server) allow(key string) bool { return s.allowMax(key, ratePerMinute) }

// allowMax counts one call for key and reports whether it is within max a minute.
func (s *Server) allowMax(key string, max int) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	if s.rate == nil || len(s.rate) > 1000 {
		s.rate = map[string]*bucket{}
	}
	now := s.now()
	b := s.rate[key]
	if b == nil || now.Sub(b.start) >= time.Minute {
		b = &bucket{start: now}
		s.rate[key] = b
	}
	b.n++
	return b.n <= max
}

// requestsPerMinute caps the child's requests per account token.
const requestsPerMinute = 10

type apiFunc func(w http.ResponseWriter, r *http.Request, cfg core.Config, acct int)

// api wraps a /v1 handler: Origin check, preflight, token, rate limit, method,
// content type and body size.
func (s *Server) api(method string, fn apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !s.extOrigin(origin) {
				jsonErr(w, http.StatusForbidden, "origin")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			if origin == "" {
				jsonErr(w, http.StatusForbidden, "origin")
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "X-KG-Token, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != method {
			jsonErr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		cfg := s.Config()
		acct, ok := cfg.AccountByToken(r.Header.Get("X-KG-Token"))
		if !ok {
			jsonErr(w, http.StatusUnauthorized, "token")
			return
		}
		if !s.allow(cfg.Accounts[acct].Token) {
			jsonErr(w, http.StatusTooManyRequests, "rate")
			return
		}
		if method == http.MethodPost {
			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mt != "application/json" {
				jsonErr(w, http.StatusUnsupportedMediaType, "content-type")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		fn(w, r, cfg, acct)
	}
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func minutesOf(hhmm string) int {
	h, _ := strconv.Atoi(hhmm[:2])
	m, _ := strconv.Atoi(hhmm[3:])
	return h*60 + m
}

// BedtimeActive reports whether t falls in the bedtime window (which may cross midnight).
func BedtimeActive(t time.Time, b core.Bedtime) bool {
	if !core.ValidHHMM(b.Start) || !core.ValidHHMM(b.End) {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	st, en := minutesOf(b.Start), minutesOf(b.End)
	switch {
	case st == en:
		return false
	case st < en:
		return m >= st && m < en
	default:
		return m >= st || m < en
	}
}

// State is the /v1/state answer.
type State struct {
	Day             string       `json:"day"`
	Band            string       `json:"band"`
	Limits          Limits       `json:"limits"`
	Used            Used         `json:"used"`
	ExtraMin        int          `json:"extraMin"`
	Bedtime         core.Bedtime `json:"bedtime"`
	BedtimeActive   bool         `json:"bedtimeActive"`
	Allow           []string     `json:"allow"`
	Block           []string     `json:"block"`
	FocusMode       bool         `json:"focusMode"`
	Adblock         bool         `json:"adblock"`
	SettingsVersion int          `json:"settingsVersion"`
	Now             string       `json:"now"`
	// 0.3.0 (P2-160). Limits and Bedtime above are today's (weekend aware);
	// Bedtime is the night that matters now (last night's in its morning part).
	Paused        bool   `json:"paused"`
	PausedUntil   string `json:"pausedUntil,omitempty"`   // RFC 3339
	PausedUntilHM string `json:"pausedUntilHM,omitempty"` // "HH:MM" on the agent's clock
	Homework      bool   `json:"homework"`
	HomeworkEnd   string `json:"homeworkEnd,omitempty"`
	Searches      bool   `json:"searches"` // search words are recorded
	// 0.3.1 (P2-161): the topic filter is on (the parent can switch it off).
	Topics   bool     `json:"topics"`
	Schedule Schedule `json:"schedule"`
}

// Schedule lets the extension work out the day and bedtime itself when the
// agent has not answered for a while.
type Schedule struct {
	School   DayLimits  `json:"school"`
	Weekend  *DayLimits `json:"weekend,omitempty"`
	Homework *HomeWin   `json:"homework,omitempty"`
}

// DayLimits are one kind of day's minutes and bedtime.
type DayLimits struct {
	BrowseMin int          `json:"browseMin"`
	VideoMin  int          `json:"videoMin"`
	Bedtime   core.Bedtime `json:"bedtime"`
}

// HomeWin is the homework window on school days.
type HomeWin struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Limits are the daily limits in minutes.
type Limits struct {
	BrowseMin int `json:"browseMin"`
	VideoMin  int `json:"videoMin"`
}

// Used is today's counted time.
type Used struct {
	BrowseSec   int `json:"browseSec"`
	VideoSec    int `json:"videoSec"`
	LearningSec int `json:"learningSec"`
}

func (s *Server) state(cfg core.Config, acct int) State {
	a := cfg.Accounts[acct]
	now := s.now()
	d := s.Store.Today(a.SID)
	nz := func(l []string) []string {
		if l == nil {
			return []string{}
		}
		return l
	}
	browse, video := a.LimitsOn(now)
	st := State{
		Day: store.DayKey(now), Band: a.Band,
		Limits:        Limits{BrowseMin: browse, VideoMin: video},
		Used:          Used{BrowseSec: d.BrowseSec, VideoSec: d.VideoSec, LearningSec: d.LearningSec},
		ExtraMin:      d.ExtraMin,
		Bedtime:       a.BedtimeNow(now),
		BedtimeActive: a.BedtimeActiveAt(now),
		Allow:         nz(a.Allow), Block: nz(a.Block),
		FocusMode: a.FocusMode, Adblock: a.Adblock,
		SettingsVersion: cfg.SettingsVersion,
		Now:             now.Format(time.RFC3339),
		Homework:        a.HomeworkActive(now),
		Searches:        !a.SearchesOff,
		Topics:          !a.TopicsOff,
		Schedule:        Schedule{School: DayLimits{BrowseMin: a.BrowseMin, VideoMin: a.VideoMin, Bedtime: a.Bedtime}},
	}
	if p, until := a.PausedAt(now); p {
		st.Paused, st.PausedUntil, st.PausedUntilHM = true, until.Format(time.RFC3339), until.Format("15:04")
	}
	if st.Homework {
		st.HomeworkEnd = a.Homework.End
	}
	if a.Weekend.On {
		st.Schedule.Weekend = &DayLimits{BrowseMin: a.Weekend.BrowseMin, VideoMin: a.Weekend.VideoMin, Bedtime: a.Weekend.Bedtime}
	}
	if a.Homework.On {
		st.Schedule.Homework = &HomeWin{Start: a.Homework.Start, End: a.Homework.End}
	}
	return st
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request, cfg core.Config, acct int) {
	writeJSON(w, http.StatusOK, s.state(cfg, acct))
}

var browserRe = regexp.MustCompile(`^[a-z]{1,16}$`)

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request, cfg core.Config, acct int) {
	var in struct {
		Browser     string         `json:"browser"`
		BrowseSec   int            `json:"browseSec"`
		VideoSec    int            `json:"videoSec"`
		LearningSec int            `json:"learningSec"`
		Domains     map[string]int `json:"domains"`
		Searches    map[string]int `json:"searches"`
	}
	if err := decode(r, &in); err != nil || !browserRe.MatchString(in.Browser) {
		jsonErr(w, http.StatusBadRequest, "body")
		return
	}
	searches := in.Searches
	if cfg.Accounts[acct].SearchesOff {
		searches = nil // switched off: nothing is recorded
	}
	s.Store.AddUsage(cfg.Accounts[acct].SID, store.Usage{
		Browser: in.Browser, BrowseSec: in.BrowseSec, VideoSec: in.VideoSec, LearningSec: in.LearningSec, Domains: in.Domains, Searches: searches,
	})
	writeJSON(w, http.StatusOK, s.state(cfg, acct))
}

// checkPin runs a PIN check through the shared lockout and writes the error
// answer itself when the PIN is wrong or locked.
func (s *Server) checkPin(w http.ResponseWriter, cfg core.Config, acct int, pin string, wrongCode int) bool {
	ok, left, retry, err := s.Lock.VerifyKey("api:"+cfg.Accounts[acct].SID, cfg.Pin, pin)
	if ok {
		return true
	}
	if errors.Is(err, core.ErrLocked) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "error": "locked", "locked": true, "retryAfterSec": int(retry.Seconds() + 0.999)})
		return false
	}
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "lockout")
		return false
	}
	writeJSON(w, wrongCode, map[string]any{"ok": false, "error": "pin", "triesLeft": left})
	return false
}

func (s *Server) handlePin(w http.ResponseWriter, r *http.Request, cfg core.Config, acct int) {
	var in struct {
		Pin string `json:"pin"`
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, http.StatusBadRequest, "body")
		return
	}
	if s.checkPin(w, cfg, acct, in.Pin, http.StatusOK) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (s *Server) handleGrant(w http.ResponseWriter, r *http.Request, cfg core.Config, acct int) {
	var in struct {
		Pin     string `json:"pin"`
		Kind    string `json:"kind"`
		Minutes int    `json:"minutes"`
		Domain  string `json:"domain"`
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, http.StatusBadRequest, "body")
		return
	}
	dom := core.NormalizeDomain(in.Domain)
	switch {
	case in.Kind == "extra" && in.Minutes >= 1 && in.Minutes <= 120:
	case in.Kind == "allow" && dom != "":
	case in.Kind == "resume":
	default:
		jsonErr(w, http.StatusBadRequest, "grant")
		return
	}
	if !s.checkPin(w, cfg, acct, in.Pin, http.StatusForbidden) {
		return
	}
	sid := cfg.Accounts[acct].SID
	if in.Kind == "extra" {
		s.Store.AddExtra(sid, in.Minutes)
		_ = s.Store.Save()
	} else if in.Kind == "resume" {
		err := s.update(func(c *core.Config) error {
			i := indexOfSID(c, sid)
			if i < 0 {
				return errors.New("account gone")
			}
			c.Accounts[i].PauseUntil = ""
			return nil
		})
		if err != nil {
			jsonErr(w, http.StatusInternalServerError, "save")
			return
		}
	} else {
		err := s.update(func(c *core.Config) error {
			i := indexOfSID(c, sid)
			if i < 0 {
				return errors.New("account gone")
			}
			a := &c.Accounts[i]
			a.Block = removeStr(a.Block, dom)
			if !containsStr(a.Allow, dom) {
				a.Allow = append(a.Allow, dom)
			}
			return nil
		})
		if err != nil {
			jsonErr(w, http.StatusInternalServerError, "save")
			return
		}
	}
	cfg = s.Config()
	i := indexOfSID(&cfg, sid)
	if i < 0 {
		jsonErr(w, http.StatusUnauthorized, "token")
		return
	}
	writeJSON(w, http.StatusOK, s.state(cfg, i))
}

// Reasons are the blocked-page reasons the extension reports, in report order.
var Reasons = []string{"adult", "gambling", "bypass", "social", "content", "topic", "video", "time", "bedtime", "homework", "paused", "parent"}

var reasons = func() map[string]bool {
	m := map[string]bool{}
	for _, r := range Reasons {
		m[r] = true
	}
	return m
}()

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request, cfg core.Config, acct int) {
	var in struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
		Domain string `json:"domain"`
	}
	if err := decode(r, &in); err != nil || in.Type != "blocked" || !reasons[in.Reason] {
		jsonErr(w, http.StatusBadRequest, "event")
		return
	}
	s.Store.AddEvent(cfg.Accounts[acct].SID, in.Reason, core.NormalizeDomain(in.Domain))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRequest records a request from the child (P2-160, F6): a site to
// allow, or more time. It needs only the account token, never the PIN.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request, cfg core.Config, acct int) {
	var in struct {
		Kind   string `json:"kind"`
		Domain string `json:"domain"`
		Note   string `json:"note"`
	}
	if err := decode(r, &in); err != nil || len(in.Note) > 4*store.MaxNoteLen {
		jsonErr(w, http.StatusBadRequest, "body")
		return
	}
	dom := core.NormalizeDomain(in.Domain)
	switch {
	case in.Kind == "site" && dom != "":
	case in.Kind == "time":
		dom = ""
	default:
		jsonErr(w, http.StatusBadRequest, "request")
		return
	}
	if !s.allowMax("req:"+cfg.Accounts[acct].Token, requestsPerMinute) {
		jsonErr(w, http.StatusTooManyRequests, "rate")
		return
	}
	// Kept in memory (the parent page sees it at once) and written by the
	// agent's one-minute save, as usage is.
	dup := s.Store.AddRequest(cfg.Accounts[acct].SID, in.Kind, dom, in.Note)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dup": dup})
}

// update changes the configuration under the lock, bumps the settings
// version and saves (which applies the policies at once).
func (s *Server) update(fn func(*core.Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.Config().Clone()
	if err := fn(&c); err != nil {
		return err
	}
	if err := c.Validate(s.Bands); err != nil {
		return err
	}
	c.SettingsVersion++
	return s.Save(c)
}

func indexOfSID(c *core.Config, sid string) int {
	for i, a := range c.Accounts {
		if a.SID == sid {
			return i
		}
	}
	return -1
}

func containsStr(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func removeStr(l []string, s string) []string {
	out := l[:0:0]
	for _, x := range l {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

// parseDomains reads a textarea: one domain per line (commas and spaces too).
func parseDomains(text string) ([]string, error) {
	out := []string{}
	for _, f := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t' }) {
		d := core.NormalizeDomain(f)
		if d == "" {
			return nil, fmt.Errorf("bad domain")
		}
		if !containsStr(out, d) {
			out = append(out, d)
		}
	}
	return out, nil
}
