package store

import (
	"path/filepath"
	"testing"
	"time"
)

const sid = "S-1-5-21-1-2-3-1001"

func TestUsageCaps(t *testing.T) {
	now := time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	s, _ := Open("", func() time.Time { return now })
	s.AddUsage(sid, Usage{Browser: "chrome", BrowseSec: 500, VideoSec: 30})
	if d := s.Today(sid); d.BrowseSec != FirstReportCap || d.VideoSec != 30 {
		t.Fatalf("first report: %+v", d)
	}
	now = now.Add(30 * time.Second)
	s.AddUsage(sid, Usage{Browser: "chrome", BrowseSec: 60, VideoSec: 60, LearningSec: -5})
	if d := s.Today(sid); d.BrowseSec != 90 || d.VideoSec != 60 || d.LearningSec != 0 {
		t.Fatalf("capped to 30 s since the last report: %+v", d)
	}
	now = now.Add(10 * time.Minute)
	s.AddUsage(sid, Usage{Browser: "chrome", BrowseSec: 600})
	if d := s.Today(sid); d.BrowseSec != 90+MaxPerCall {
		t.Fatalf("capped to 120 per call: %+v", d)
	}
	// A second browser counts on its own.
	s.AddUsage(sid, Usage{Browser: "firefox", BrowseSec: 20, Domains: map[string]int{"Example.org": 20, "bad domain": 5}})
	if d := s.Today(sid); d.BrowseSec != 230 {
		t.Fatalf("firefox: %+v", d)
	}
	_, top := s.Report(sid, 7)
	if len(top) != 1 || top[0].Domain != "example.org" {
		t.Fatalf("domains: %+v", top)
	}
}

func TestExtraEventsReportPrune(t *testing.T) {
	now := time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	path := filepath.Join(t.TempDir(), "usage.json")
	s, _ := Open(path, func() time.Time { return now })
	if s.AddExtra(sid, 15) != 15 || s.AddExtra(sid, 500) != maxExtraMinPerDay {
		t.Fatal("extra minutes")
	}
	s.AddEvent(sid, "adult", "x.example")
	s.AddEvent(sid, "adult", "x.example")
	s.AddEvent(sid, "time", "")
	for i := 0; i < 40; i++ {
		now = now.AddDate(0, 0, -1)
		s.AddEvent(sid, "social", "")
	}
	now = time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	rep, _ := s.Report(sid, 7)
	if len(rep) != 7 || rep[6].Day != "2026-11-03" || rep[6].Blocked != 3 || rep[6].ByReason["adult"] != 2 || rep[5].Blocked != 1 {
		t.Fatalf("report: %+v", rep[5:])
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, _ := Open(path, func() time.Time { return now })
	if s2.DayCount(sid) != 41 {
		t.Fatalf("reload: %d days", s2.DayCount(sid))
	}
	s2.Prune(map[string]bool{sid: true})
	if n := s2.DayCount(sid); n != KeepDays+1 {
		t.Fatalf("after prune: %d days, want %d", n, KeepDays+1)
	}
	s2.Prune(map[string]bool{})
	if s2.DayCount(sid) != 0 {
		t.Fatal("removed accounts must be pruned")
	}
}
