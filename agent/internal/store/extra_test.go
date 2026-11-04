package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearches(t *testing.T) {
	now := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	s, _ := Open("", func() time.Time { return now })
	s.AddUsage(sid, Usage{Browser: "chrome", Searches: map[string]int{"  How Do  Volcanoes WORK ": 1, "": 3, "bad\x01word": 1, strings.Repeat("я", 130): 1}})
	det := s.Detail(sid, "2026-11-04")
	got := map[string]int{}
	for _, c := range det.Searches {
		got[c.Key] = c.Count
	}
	if got["how do volcanoes work"] != 1 || len(got) != 2 || got[strings.Repeat("я", 100)] != 1 {
		t.Fatalf("cleaned searches: %v", got)
	}
	// Up to 50 different searches a day; counts still grow on known ones.
	for i := 0; i < 5; i++ {
		m := map[string]int{}
		for j := 0; j < 20; j++ {
			m[fmt.Sprintf("q%d-%d", i, j)] = 1
		}
		s.AddUsage(sid, Usage{Browser: "chrome", Searches: m})
	}
	s.AddUsage(sid, Usage{Browser: "chrome", Searches: map[string]int{"how do volcanoes work": 2}})
	det = s.Detail(sid, "2026-11-04")
	if len(det.Searches) != MaxSearchesPerDay || det.Searches[0].Key != "how do volcanoes work" || det.Searches[0].Count != 3 {
		t.Fatalf("cap or count: %d %+v", len(det.Searches), det.Searches[0])
	}
	if CleanSearch("A\tB") != "a b" || CleanSearch(" \n ") != "" {
		t.Error("CleanSearch")
	}
}

func TestGameTimeAndClosedApps(t *testing.T) {
	now := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	s, _ := Open("", func() time.Time { return now })
	s.AddGame(sid, 3)
	s.AddGame(sid, 500) // capped
	if sec, warned := s.GameToday(sid); sec != 3+MaxGamePerCall || warned {
		t.Fatalf("game seconds %d %v", sec, warned)
	}
	if !s.MarkGameWarned(sid) || s.MarkGameWarned(sid) {
		t.Fatal("the warning shows once a day")
	}
	s.AddExtra(sid, 15)
	if !s.MarkGameWarned(sid) {
		t.Fatal("extra minutes let the warning show again")
	}
	s.ResetGameWarned(sid)
	if !s.MarkGameWarned(sid) {
		t.Fatal("more game time lets the warning show again")
	}
	s.AddAppClosed(sid, "roblox")
	s.AddAppClosed(sid, "roblox")
	s.AddAppClosed(sid, "discord")
	det := s.Detail(sid, "2026-11-04")
	if len(det.AppsClosed) != 2 || det.AppsClosed[0].Key != "roblox" || det.AppsClosed[0].Count != 2 {
		t.Fatalf("closed apps: %+v", det.AppsClosed)
	}
	for i := 0; i < 20; i++ {
		s.AddGame(sid, 10)
	}
	rep, _ := s.Report(sid, 7)
	if rep[6].GameMin != (3+MaxGamePerCall+200)/60 {
		t.Fatalf("report game minutes: %d", rep[6].GameMin)
	}
	now = now.Add(24 * time.Hour)
	if sec, warned := s.GameToday(sid); sec != 0 || warned {
		t.Fatal("a new day starts at zero")
	}
}

func TestRequests(t *testing.T) {
	now := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "usage.json")
	s, _ := Open(path, func() time.Time { return now })
	if s.AddRequest(sid, "site", "khanacademy.org", "  for  maths\n") {
		t.Fatal("first request is not a duplicate")
	}
	if !s.AddRequest(sid, "site", "khanacademy.org", "") {
		t.Fatal("the same request twice in a row is a duplicate")
	}
	s.AddRequest(sid, "time", "", strings.Repeat("x", 300))
	l := s.Requests(sid)
	if len(l) != 2 || l[0].Kind != "time" || len([]rune(l[0].Note)) != MaxNoteLen || l[1].Note != "for maths" {
		t.Fatalf("requests: %+v", l)
	}
	// The same site again after another request is a new one.
	if s.AddRequest(sid, "site", "khanacademy.org", "") {
		t.Fatal("not in a row: a new request")
	}
	for i := 0; i < 30; i++ {
		s.AddRequest(sid, "site", fmt.Sprintf("s%d.example", i), "")
	}
	if n := len(s.Requests(sid)); n != MaxRequests {
		t.Fatalf("kept %d, want %d", n, MaxRequests)
	}
	first := s.Requests(sid)[0]
	if r, ok := s.TakeRequest(sid, first.ID); !ok || r.Domain != "s29.example" {
		t.Fatalf("take: %+v %v", r, ok)
	}
	if _, ok := s.TakeRequest(sid, first.ID); ok {
		t.Fatal("taken twice")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, _ := Open(path, func() time.Time { return now })
	if len(s2.Requests(sid)) != MaxRequests-1 {
		t.Fatal("requests not saved")
	}
	now = now.AddDate(0, 0, RequestDays).Add(time.Minute)
	if len(s2.Requests(sid)) != 0 {
		t.Fatal("requests older than 7 days are dropped")
	}
	s2.AddRequest("S-1-5-21-9", "time", "", "")
	s2.Prune(map[string]bool{sid: true})
	if len(s2.Requests("S-1-5-21-9")) != 0 {
		t.Fatal("requests of removed accounts are dropped")
	}
}
