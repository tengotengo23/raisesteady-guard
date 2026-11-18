package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/news"
)

// TestServeForBrowser serves the real parent page on 127.0.0.1:47615 (PIN 2468) for a real-browser test
// (tests/e2e/parent-login.mjs, P2-157) until KG_SERVE_SECONDS pass or /__stop is asked (this test's own
// address, not the agent's). Runs only with KG_SERVE_SECONDS set. KG_SERVE_NEWS=on|ask adds the news box (P2-252):
// "on" with two sample notes (a layout check at the owner's font size), "ask" with the Turn on question.
func TestServeForBrowser(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("KG_SERVE_SECONDS"))
	if n <= 0 {
		t.Skip("KG_SERVE_SECONDS not set")
	}
	r := newRig(t)
	r.now = time.Now()
	if v := os.Getenv("KG_SERVE_NEWS"); v == "on" || v == "ask" {
		r.s.News = &fakeNews{items: []news.Item{
			{ID: "s-1", Date: "2026-11-18", Title: map[string]string{"en": "Version 0.4.4: news on this page", "ka": "ვერსია 0.4.4: სიახლეები ამ გვერდზე"},
				Body: map[string]string{"en": "You can now get short news from us here.\nTurn it off at any time.", "ka": "ახლა აქ მოკლე სიახლეებს მიიღებთ.\nნებისმიერ დროს შეგიძლიათ გამორთოთ."},
				Link: "https://raisesteady.com/guard/"},
			{ID: "s-2", Date: "2026-11-10", Title: map[string]string{"en": strings.Repeat("Long title ", 7)}, Body: map[string]string{"en": strings.Repeat("A long line of text without breaks ", 17)}},
		}}
		if v == "on" {
			r.cfg.News = core.NewsOn
		}
	}
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
