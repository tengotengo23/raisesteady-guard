package store

import (
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"raisesteady-guard/agent/internal/core"
)

// ---- PC game time and closed apps (P2-160, F5) ----

// AddGame adds game seconds for today (at most MaxGamePerCall per call) and
// returns today's total.
func (s *Store) AddGame(sid string, sec int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(sid, s.Now())
	d.GameSec += clamp(sec, 0, MaxGamePerCall)
	s.dirty = true
	return d.GameSec
}

// GameToday returns today's game seconds and whether the 5-minute warning was shown.
func (s *Store) GameToday(sid string) (sec int, warned bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(sid, s.Now())
	return d.GameSec, d.GameWarned
}

// MarkGameWarned records the 5-minute warning; it returns false when it was already shown today.
func (s *Store) MarkGameWarned(sid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(sid, s.Now())
	if d.GameWarned {
		return false
	}
	d.GameWarned = true
	s.dirty = true
	return true
}

// ResetGameWarned lets the game-time warning show again today (the parent gave more game time).
func (s *Store) ResetGameWarned(sid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.day(sid, s.Now()).GameWarned = false
	s.dirty = true
}

// AddAppClosed counts one closed app for today.
func (s *Store) AddAppClosed(sid, appID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(sid, s.Now())
	if d.AppsClosed == nil {
		d.AppsClosed = map[string]int{}
	}
	if _, ok := d.AppsClosed[appID]; ok || len(d.AppsClosed) < 100 {
		d.AppsClosed[appID]++
	}
	s.dirty = true
}

// Count is a word or an app with a count.
type Count struct {
	Key   string
	Count int
}

func sorted(m map[string]int, max int) []Count {
	var out []Count
	for k, c := range m {
		out = append(out, Count{k, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

// DayDetail is the part of the report for one picked day.
type DayDetail struct {
	Day        string
	Searches   []Count // most first
	AppsClosed []Count // app id -> times closed
	GameMin    int
}

// Detail returns one day's searches and closed apps.
func (s *Store) Detail(sid, day string) DayDetail {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := DayDetail{Day: day}
	if d := s.data.Accounts[sid][day]; d != nil {
		out.Searches = sorted(d.Searches, MaxSearchesPerDay)
		out.AppsClosed = sorted(d.AppsClosed, 0)
		out.GameMin = d.GameSec / 60
	}
	return out
}

// ---- requests from the child (P2-160, F6) ----

// Request is one open request: a site to allow, or more time.
type Request struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"` // "site" or "time"
	Domain string    `json:"domain,omitempty"`
	Note   string    `json:"note,omitempty"`
	At     time.Time `json:"at"`
}

// CleanNote trims a note, drops control characters and cuts it to MaxNoteLen characters.
func CleanNote(n string) string {
	if !utf8.ValidString(n) {
		return ""
	}
	n = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, n)
	n = strings.Join(strings.Fields(n), " ")
	if utf8.RuneCountInString(n) > MaxNoteLen {
		n = strings.TrimSpace(string([]rune(n)[:MaxNoteLen]))
	}
	return n
}

// pruneRequests drops requests older than RequestDays; it must run under mu.
func (s *Store) pruneRequests() {
	cut := s.Now().AddDate(0, 0, -RequestDays)
	for sid, l := range s.data.Requests {
		keep := l[:0:0]
		for _, r := range l {
			if r.At.After(cut) {
				keep = append(keep, r)
			}
		}
		if len(keep) != len(l) {
			s.dirty = true
		}
		if len(keep) == 0 {
			delete(s.data.Requests, sid)
		} else {
			s.data.Requests[sid] = keep
		}
	}
}

// AddRequest records a request. The same request as the newest open one is
// not added twice (its time and note are refreshed; dup is true). Only the
// newest MaxRequests are kept.
func (s *Store) AddRequest(sid, kind, domain, note string) (dup bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Requests == nil {
		s.data.Requests = map[string][]Request{}
	}
	s.pruneRequests()
	now := s.Now()
	note = CleanNote(note)
	l := s.data.Requests[sid]
	if n := len(l); n > 0 && l[n-1].Kind == kind && l[n-1].Domain == domain {
		l[n-1].At = now
		if note != "" {
			l[n-1].Note = note
		}
		s.dirty = true
		return true
	}
	l = append(l, Request{ID: core.RandomHex(8), Kind: kind, Domain: domain, Note: note, At: now})
	if len(l) > MaxRequests {
		l = l[len(l)-MaxRequests:]
	}
	s.data.Requests[sid] = l
	s.dirty = true
	return false
}

// Requests returns an account's open requests, newest first.
func (s *Store) Requests(sid string) []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneRequests()
	l := s.data.Requests[sid]
	out := make([]Request, 0, len(l))
	for i := len(l) - 1; i >= 0; i-- {
		out = append(out, l[i])
	}
	return out
}

// TakeRequest removes an open request and returns it.
func (s *Store) TakeRequest(sid, id string) (Request, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.data.Requests[sid]
	for i, r := range l {
		if r.ID == id {
			s.data.Requests[sid] = append(l[:i:i], l[i+1:]...)
			if len(s.data.Requests[sid]) == 0 {
				delete(s.data.Requests, sid)
			}
			s.dirty = true
			return r, true
		}
	}
	return Request{}, false
}
