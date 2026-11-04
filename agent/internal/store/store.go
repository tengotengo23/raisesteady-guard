// Package store keeps each protected account's daily usage, grants and
// blocked-page counts for 30 days. No full URLs are stored.
package store

import (
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"raisesteady-guard/agent/internal/core"
)

// KeepDays is how long daily records are kept.
const KeepDays = 30

// MaxPerCall is the most seconds one usage report may add to each field.
const MaxPerCall = 120

// FirstReportCap is the most seconds a browser's first report may add.
const FirstReportCap = 60

const (
	maxDomainsPerDay  = 200
	maxDomainsPerCall = 50
	maxBlockedDomains = 100
	maxExtraMinPerDay = 240
	maxEventsPerDay   = 5000
	// MaxSearchesPerDay is how many different searches a day keeps.
	MaxSearchesPerDay  = 50
	maxSearchesPerCall = 20
	// MaxSearchLen is the longest search kept, in characters.
	MaxSearchLen = 100
	// MaxGamePerCall caps one game-time step (the guard runs every 3 seconds).
	MaxGamePerCall = 10
	// MaxRequests is how many open requests an account keeps; RequestDays how long.
	MaxRequests = 20
	RequestDays = 7
	// MaxNoteLen is the longest note on a request, in characters.
	MaxNoteLen = 140
)

// Day is one account's record for one local date.
type Day struct {
	BrowseSec      int            `json:"browseSec"`
	VideoSec       int            `json:"videoSec"`
	LearningSec    int            `json:"learningSec"`
	ExtraMin       int            `json:"extraMin"`
	Domains        map[string]int `json:"domains,omitempty"`
	Blocked        map[string]int `json:"blocked,omitempty"`        // reason -> count
	BlockedDomains map[string]int `json:"blockedDomains,omitempty"` // domain -> count
	// 0.3.0 (P2-160)
	Searches   map[string]int `json:"searches,omitempty"`   // search words -> count
	GameSec    int            `json:"gameSec,omitempty"`    // PC game time
	GameWarned bool           `json:"gameWarned,omitempty"` // the 5-minute warning was shown
	AppsClosed map[string]int `json:"appsClosed,omitempty"` // app id -> times closed
}

type file struct {
	Accounts map[string]map[string]*Day `json:"accounts"` // SID -> "2026-11-03" -> day
	// Requests are the child's open requests per account (P2-160).
	Requests map[string][]Request `json:"requests,omitempty"`
}

// Store is safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	Path  string
	Now   func() time.Time
	data  file
	last  map[string]time.Time // SID|browser -> last report
	dirty bool
}

// Open loads the store from path (missing file: empty store).
func Open(path string, now func() time.Time) (*Store, error) {
	s := &Store{Path: path, Now: now, last: map[string]time.Time{}}
	if s.Now == nil {
		s.Now = time.Now
	}
	s.data.Accounts = map[string]map[string]*Day{}
	if path != "" {
		if err := core.LoadJSON(path, &s.data); err != nil && !errors.Is(err, os.ErrNotExist) {
			// A damaged file starts a fresh store rather than stopping the agent.
			s.data.Accounts = map[string]map[string]*Day{}
		}
		if s.data.Accounts == nil {
			s.data.Accounts = map[string]map[string]*Day{}
		}
	}
	return s, nil
}

// DayKey is the local date of t.
func DayKey(t time.Time) string { return t.Format("2006-01-02") }

func (s *Store) day(sid string, t time.Time) *Day {
	days := s.data.Accounts[sid]
	if days == nil {
		days = map[string]*Day{}
		s.data.Accounts[sid] = days
	}
	k := DayKey(t)
	d := days[k]
	if d == nil {
		d = &Day{}
		days[k] = d
	}
	return d
}

// Today returns a copy of today's record.
func (s *Store) Today(sid string) Day {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(sid, s.Now())
	return Day{BrowseSec: d.BrowseSec, VideoSec: d.VideoSec, LearningSec: d.LearningSec, ExtraMin: d.ExtraMin}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Usage is one report from a browser.
type Usage struct {
	Browser     string
	BrowseSec   int
	VideoSec    int
	LearningSec int
	Domains     map[string]int
	// Searches are search words with a count; nil when the parent switched them off.
	Searches map[string]int
}

// AddUsage adds a report, capping each field to the real seconds since this
// browser's last report and to MaxPerCall.
func (s *Store) AddUsage(sid string, u Usage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	key := sid + "|" + u.Browser
	limit := FirstReportCap
	if t, ok := s.last[key]; ok {
		el := int(now.Sub(t).Seconds() + 0.999)
		limit = clamp(el, 0, MaxPerCall)
	}
	s.last[key] = now
	if len(s.last) > 1000 {
		s.last = map[string]time.Time{key: now}
	}
	d := s.day(sid, now)
	b := clamp(u.BrowseSec, 0, limit)
	v := clamp(u.VideoSec, 0, limit)
	l := clamp(u.LearningSec, 0, limit)
	d.BrowseSec += b
	d.VideoSec += v
	d.LearningSec += l
	if d.Domains == nil {
		d.Domains = map[string]int{}
	}
	n := 0
	for dom, sec := range u.Domains {
		if n >= maxDomainsPerCall {
			break
		}
		dom = core.NormalizeDomain(dom)
		if dom == "" {
			continue
		}
		n++
		if _, ok := d.Domains[dom]; !ok && len(d.Domains) >= maxDomainsPerDay {
			continue
		}
		d.Domains[dom] += clamp(sec, 0, limit)
	}
	n = 0
	for q, c := range u.Searches {
		if n >= maxSearchesPerCall {
			break
		}
		q = CleanSearch(q)
		if q == "" {
			continue
		}
		n++
		if d.Searches == nil {
			d.Searches = map[string]int{}
		}
		if _, ok := d.Searches[q]; !ok && len(d.Searches) >= MaxSearchesPerDay {
			continue
		}
		d.Searches[q] += clamp(c, 1, 20)
	}
	s.dirty = true
}

// CleanSearch trims, lower-cases and folds the spaces of a search, cuts it to
// MaxSearchLen characters, and returns "" for an empty one or one with
// control characters.
func CleanSearch(q string) string {
	q = strings.Join(strings.Fields(strings.ToLower(q)), " ")
	if !utf8.ValidString(q) {
		return ""
	}
	for _, r := range q {
		if unicode.IsControl(r) {
			return ""
		}
	}
	if utf8.RuneCountInString(q) > MaxSearchLen {
		q = strings.TrimSpace(string([]rune(q)[:MaxSearchLen]))
	}
	return q
}

// LastSeen returns when a browser's extension last reported usage for an
// account (since the service started).
func (s *Store) LastSeen(sid, browser string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.last[sid+"|"+browser]
	return t, ok
}

// AddExtra grants extra minutes for today and returns today's total extra.
func (s *Store) AddExtra(sid string, minutes int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(sid, s.Now())
	d.ExtraMin = clamp(d.ExtraMin+minutes, 0, maxExtraMinPerDay)
	if minutes > 0 {
		d.GameWarned = false // more time: the game-time warning may show again
	}
	s.dirty = true
	return d.ExtraMin
}

// AddEvent counts a blocked page by reason and domain.
func (s *Store) AddEvent(sid, reason, domain string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(sid, s.Now())
	if d.Blocked == nil {
		d.Blocked = map[string]int{}
	}
	total := 0
	for _, c := range d.Blocked {
		total += c
	}
	if total >= maxEventsPerDay {
		return
	}
	d.Blocked[reason]++
	if domain != "" {
		if d.BlockedDomains == nil {
			d.BlockedDomains = map[string]int{}
		}
		if _, ok := d.BlockedDomains[domain]; ok || len(d.BlockedDomains) < maxBlockedDomains {
			d.BlockedDomains[domain]++
		}
	}
	s.dirty = true
}

// DayReport is one line of the parent's report.
type DayReport struct {
	Day         string
	BrowseMin   int
	VideoMin    int
	LearningMin int
	GameMin     int
	Blocked     int
	ByReason    map[string]int
}

// Report returns the last n days (oldest first) and the top domains by time.
func (s *Store) Report(sid string, n int) ([]DayReport, []DomainTime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	var out []DayReport
	tot := map[string]int{}
	for i := n - 1; i >= 0; i-- {
		t := now.AddDate(0, 0, -i)
		k := DayKey(t)
		r := DayReport{Day: k, ByReason: map[string]int{}}
		if d := s.data.Accounts[sid][k]; d != nil {
			r.BrowseMin = d.BrowseSec / 60
			r.VideoMin = d.VideoSec / 60
			r.LearningMin = d.LearningSec / 60
			r.GameMin = d.GameSec / 60
			for reason, c := range d.Blocked {
				r.Blocked += c
				r.ByReason[reason] = c
			}
			for dom, sec := range d.Domains {
				tot[dom] += sec
			}
		}
		out = append(out, r)
	}
	var doms []DomainTime
	for d, sec := range tot {
		doms = append(doms, DomainTime{Domain: d, Min: sec / 60})
	}
	sort.Slice(doms, func(i, j int) bool {
		if doms[i].Min != doms[j].Min {
			return doms[i].Min > doms[j].Min
		}
		return doms[i].Domain < doms[j].Domain
	})
	if len(doms) > 10 {
		doms = doms[:10]
	}
	return out, doms
}

// DomainTime is minutes spent on one domain.
type DomainTime struct {
	Domain string
	Min    int
}

// Prune drops days older than KeepDays, requests older than RequestDays and
// accounts not in keep.
func (s *Store) Prune(keep map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := DayKey(s.Now().AddDate(0, 0, -KeepDays))
	s.pruneRequests()
	for sid := range s.data.Requests {
		if keep != nil && !keep[sid] {
			delete(s.data.Requests, sid)
			s.dirty = true
		}
	}
	for sid, days := range s.data.Accounts {
		if keep != nil && !keep[sid] {
			delete(s.data.Accounts, sid)
			s.dirty = true
			continue
		}
		for k := range days {
			if k < cut {
				delete(days, k)
				s.dirty = true
			}
		}
	}
}

// DayCount returns how many days are stored for an account (for tests).
func (s *Store) DayCount(sid string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.Accounts[sid])
}

// Save writes the store if it changed.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty || s.Path == "" {
		return nil
	}
	if err := core.SaveJSON(s.Path, &s.data); err != nil {
		return err
	}
	s.dirty = false
	return nil
}
