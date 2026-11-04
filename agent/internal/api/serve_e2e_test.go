package api

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/core"
)

// TestServeForE2E serves the extension API and the parent page for the managed-mode browser test
// (tests/e2e/c-managed.mjs, P2-160) on 127.0.0.1:KG_E2E_PORT (default 47616) until KG_E2E_SECONDS
// pass or /__stop is asked. Account Ana (token "a1" x 32, PIN 2468, age 9-12). The clock is the real
// time of day moved to the next Wednesday, so homework time (school days) always applies; bedtime is
// set six hours away and the limits are long, so neither gets in the way. Runs only with KG_E2E_SECONDS.
func TestServeForE2E(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("KG_E2E_SECONDS"))
	if n <= 0 {
		t.Skip("KG_E2E_SECONDS not set")
	}
	port := 47616
	if p, err := strconv.Atoi(os.Getenv("KG_E2E_PORT")); err == nil && p > 1024 {
		port = p
	}
	r := newRig(t)
	shift := (int(time.Wednesday) - int(time.Now().Weekday()) + 7) % 7
	r.s.Now = func() time.Time { return time.Now().AddDate(0, 0, shift) }
	r.s.Store.Now = r.s.Now
	r.s.Lock.Now = r.s.Now
	now := r.s.Now()
	hm := func(t time.Time) string { return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute()) }
	b, _ := r.s.Bands.Get("9-12")
	r.setSettings(0, func(s *core.Settings) {
		s.Band = b.ID
		s.BrowseMin, s.VideoMin = 720, 720
		s.Bedtime.Start, s.Bedtime.End = hm(now.Add(6*time.Hour)), hm(now.Add(7*time.Hour))
		s.Block = []string{"blocked.kgtest.example"}
	})
	r.cfg.Port = port
	r.s.Port = port
	srv := &http.Server{Addr: "127.0.0.1:" + strconv.Itoa(port)}
	h := r.s.Handler()
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.URL.Path == "/__stop" {
			go srv.Close()
			return
		}
		h.ServeHTTP(w, q)
	})
	go func() { time.Sleep(time.Duration(n) * time.Second); srv.Close() }()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}
