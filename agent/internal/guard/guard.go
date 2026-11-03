// Package guard finds browsers the agent cannot protect and closes them in
// protected accounts' sessions, with a short calm message.
package guard

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"raisesteady-guard/agent/internal/core"
)

// Supported exe names and the product names that confirm them.
var supportedExe = map[string]string{
	"chrome.exe":  "chrome",
	"msedge.exe":  "edge",
	"brave.exe":   "brave",
	"firefox.exe": "firefox",
}

// Match is what Classify found.
type Match struct {
	SupportedID string            // chrome, edge, brave, firefox
	Unsupported *core.Unsupported // a browser we close
}

func normPath(p string) string {
	return strings.ToLower(strings.ReplaceAll(p, "/", `\`))
}

func baseName(p string) string {
	p = normPath(p)
	if i := strings.LastIndex(p, `\`); i >= 0 {
		return p[i+1:]
	}
	return filepath.Base(p)
}

// Classify decides what a program is. A path match always means "unsupported"
// (a Tor Browser firefox.exe is not Firefox); an exe-name match does not
// count when the version info names a supported product.
func Classify(list core.UnsupportedList, path, product string) Match {
	np := normPath(path)
	exe := baseName(path)
	prod := strings.TrimSpace(product)
	for i := range list.Browsers {
		u := &list.Browsers[i]
		for _, pc := range u.PathContains {
			if pc != "" && strings.Contains(np, strings.ToLower(pc)) {
				return Match{Unsupported: u}
			}
		}
	}
	for i := range list.Browsers {
		u := &list.Browsers[i]
		for _, p := range u.Product {
			if prod != "" && strings.EqualFold(prod, p) {
				return Match{Unsupported: u}
			}
		}
	}
	supportedProduct := false
	for _, p := range list.SupportedProducts {
		if prod != "" && strings.EqualFold(prod, p) {
			supportedProduct = true
		}
	}
	if !supportedProduct {
		for i := range list.Browsers {
			u := &list.Browsers[i]
			for _, e := range u.Exe {
				if strings.EqualFold(exe, e) {
					return Match{Unsupported: u}
				}
			}
		}
	}
	if id, ok := supportedExe[exe]; ok {
		return Match{SupportedID: id}
	}
	return Match{}
}

// Proc is one running process.
type Proc struct {
	PID     uint32
	Session uint32
	SID     string
	Path    string
}

// System is what the guard needs from Windows.
type System interface {
	Processes() ([]Proc, error)
	ProductName(path string) string
	Kill(pid uint32) error
	Message(session uint32, title, text string) error
	UserLang(sid string) string
}

// Closed is one browser the guard closed (for the parent page).
type Closed struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Path  string    `json:"path"`
	Count int       `json:"count"`
	Last  time.Time `json:"last"`
}

// Guard is the 3-second process guard.
type Guard struct {
	Sys       System
	List      core.UnsupportedList
	Protected func(sid string) bool
	Now       func() time.Time

	mu       sync.Mutex
	products map[string]string // path -> product name
	lastMsg  map[string]time.Time
	closed   map[string]*Closed
}

// MessageEvery is the shortest time between two messages for the same browser
// in the same session.
const MessageEvery = time.Minute

func (g *Guard) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *Guard) product(path string) string {
	if p, ok := g.products[path]; ok {
		return p
	}
	if len(g.products) > 2000 {
		g.products = map[string]string{}
	}
	p := g.Sys.ProductName(path)
	g.products[path] = p
	return p
}

// Tick checks every process once and closes unsupported browsers in protected
// sessions. It returns how many processes it closed.
func (g *Guard) Tick() (int, error) {
	procs, err := g.Sys.Processes()
	if err != nil {
		return 0, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.products == nil {
		g.products = map[string]string{}
		g.lastMsg = map[string]time.Time{}
		g.closed = map[string]*Closed{}
	}
	n := 0
	for _, p := range procs {
		if p.Session == 0 || p.Path == "" || p.SID == "" || !g.Protected(p.SID) {
			continue
		}
		m := Classify(g.List, p.Path, g.product(p.Path))
		if m.Unsupported == nil {
			continue
		}
		if err := g.Sys.Kill(p.PID); err != nil {
			continue
		}
		n++
		c := g.closed[m.Unsupported.ID]
		if c == nil {
			c = &Closed{ID: m.Unsupported.ID, Name: m.Unsupported.Name}
			g.closed[m.Unsupported.ID] = c
		}
		c.Count++
		c.Last = g.now()
		c.Path = p.Path
		key := m.Unsupported.ID + "|" + strconv.FormatUint(uint64(p.Session), 10)
		if t, ok := g.lastMsg[key]; ok && g.now().Sub(t) < MessageEvery {
			continue
		}
		g.lastMsg[key] = g.now()
		lang := g.Sys.UserLang(p.SID)
		_ = g.Sys.Message(p.Session, core.AppName, ClosedMessage(lang, m.Unsupported.Name))
	}
	return n, nil
}

// ClosedList returns the browsers closed since the service started.
func (g *Guard) ClosedList() []Closed {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []Closed
	for _, c := range g.closed {
		out = append(out, *c)
	}
	return out
}

var closedText = map[string]string{
	"en": "%s isn't set up for children on this computer, so it was closed. You can use Chrome, Edge or Firefox instead.",
	"ka": "%s ამ კომპიუტერზე ბავშვებისთვის არ არის მორგებული, ამიტომ დაიხურა. სანაცვლოდ გამოიყენე Chrome, Edge ან Firefox.",
	"ru": "%s на этом компьютере не настроен для детей, поэтому он закрыт. Вместо него можно открыть Chrome, Edge или Firefox.",
}

// ClosedMessage is the short message shown in the child's session.
func ClosedMessage(lang, browser string) string {
	t, ok := closedText[lang]
	if !ok {
		t = closedText["en"]
	}
	return strings.Replace(t, "%s", browser, 1)
}

// LangFromLocale maps a Windows language tag ("ka-GE") to en, ka or ru.
func LangFromLocale(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	switch {
	case strings.HasPrefix(tag, "ka"):
		return "ka"
	case strings.HasPrefix(tag, "ru"):
		return "ru"
	default:
		return "en"
	}
}
