package api

import (
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestServeForBrowser serves the real parent page on 127.0.0.1:47615 (PIN 2468) for a real-browser test
// (tests/e2e/parent-login.mjs, P2-157) until KG_SERVE_SECONDS pass or /__stop is asked (this test's own
// address, not the agent's). Runs only with KG_SERVE_SECONDS set.
func TestServeForBrowser(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("KG_SERVE_SECONDS"))
	if n <= 0 {
		t.Skip("KG_SERVE_SECONDS not set")
	}
	r := newRig(t)
	r.now = time.Now()
	h := r.s.Handler()
	srv := &http.Server{Addr: "127.0.0.1:47615"}
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
