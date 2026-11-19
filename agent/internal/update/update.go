// Package update keeps RaiseSteady Guard up to date (P2-260). It reads a
// small signed file, update.json, from the project's GitHub releases, checks
// its Ed25519 signature with the key built into the program, and installs a
// newer version silently: no window and no question on the child's screen.
//
// Rules: the signature must be valid over the exact bytes of update.json; the
// version must be strictly newer; the download must come from the project's
// release folder over https; its size must be within 1.5 times the declared
// size and under 64 MB; its SHA-256 must match. Anything else is ignored, the
// running version stays, and one line goes to the log.
//
// Privacy: every request is a plain GET with the User-Agent
// "RaiseSteadyGuard": no cookies, no query string, no identifiers.
//
// The addresses and the key are unexported variables; only tests in this
// package change them. There is no environment variable or flag for them,
// and no off switch.
package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"raisesteady-guard/agent/internal/core"
)

// The only addresses read and the only key trusted. Not configurable; tests
// in this package change them to a local test server and a test key.
var (
	manifestURL = "https://github.com/tengotengo23/raisesteady-guard/releases/latest/download/update.json"
	urlPrefix   = "https://github.com/tengotengo23/raisesteady-guard/releases/download/"
	publicKey   = mustKey("HRfw+iOYKPkWrl92DL2ytpXVasidCrMbesThZ99acKQ=")
	allowHost   = func(host string) bool {
		host = strings.ToLower(host)
		return host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")
	}
)

const (
	UserAgent    = "RaiseSteadyGuard"
	MaxDownload  = 64 << 20 // largest installer accepted
	maxManifest  = 64 << 10
	maxSig       = 1 << 10
	maxRedirects = 5
	maxUnpacked  = 256 << 20 // whole tar contents

	dialTimeout     = 10 * time.Second
	smallTimeout    = 60 * time.Second
	downloadTimeout = 20 * time.Minute

	Every      = 6 * time.Hour    // between checks
	Jitter     = 30 * time.Minute // random extra wait
	PendingFor = 30 * time.Minute // time an install has to finish
	startDelay = 2 * time.Minute  // first check after the service starts
)

// backoff is the wait after the 1st, 2nd and 3rd (or later) failure in a row.
var backoff = []time.Duration{time.Hour, 6 * time.Hour, 24 * time.Hour}

func mustKey(b64 string) ed25519.PublicKey {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("update: bad built-in key")
	}
	return ed25519.PublicKey(b)
}

// File is one download in the manifest.
type File struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest is update.json.
type Manifest struct {
	V       int             `json:"v"`
	Version string          `json:"version"`
	Date    string          `json:"date"`
	Files   map[string]File `json:"files"`
}

// State is kept in <data>/update.json between runs.
type State struct {
	LastCheck  time.Time `json:"lastCheck,omitempty"`  // last check that finished (newest or installed)
	LastUpdate time.Time `json:"lastUpdate,omitempty"` // last update that finished
	Pending    string    `json:"pending,omitempty"`    // version being installed
	PendingAt  time.Time `json:"pendingAt,omitempty"`
	Failures   int       `json:"failures,omitempty"` // in a row
	NextTry    time.Time `json:"nextTry,omitempty"`
	LastErr    string    `json:"lastErr,omitempty"`
}

// Status is what the parent page shows.
type Status struct {
	Version    string
	LastCheck  time.Time
	LastUpdate time.Time
	Failing    bool // three or more failures in a row
}

// Updater checks for and installs updates.
type Updater struct {
	dir     string // data folder; downloads go to dir/updates
	version string // running version
	logf    func(string, ...any)

	now     func() time.Time
	jitter  func() time.Duration
	sleep   func(context.Context, time.Duration) bool
	target  func() (key, kind string, err error)
	install func(ctx context.Context, kind, file string) error
	client  *http.Client
	dlLimit time.Duration

	mu sync.Mutex
	st State
}

// New returns an updater for the running version, keeping its state in dir.
func New(dir, version string, logf func(string, ...any)) *Updater {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	u := &Updater{
		dir: dir, version: version, logf: logf,
		now:     time.Now,
		jitter:  func() time.Duration { return time.Duration(rand.Int64N(int64(Jitter))) },
		sleep:   sleepCtx,
		target:  platformTarget,
		install: platformInstall,
		client:  newClient(nil),
		dlLimit: downloadTimeout,
	}
	_ = core.LoadJSON(u.statePath(), &u.st)
	return u
}

func newClient(tr http.RoundTripper) *http.Client {
	if tr == nil {
		tr = &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
			TLSHandshakeTimeout: dialTimeout,
			ForceAttemptHTTP2:   true,
			DisableCompression:  true,
		}
	}
	// No cookie jar. Redirects only over https to GitHub, a few hops at most.
	return &http.Client{Transport: tr, CheckRedirect: checkRedirect}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("too many redirects")
	}
	return checkURL(req.URL)
}

func checkURL(u *url.URL) error {
	if u.Scheme != "https" || u.User != nil || !allowHost(u.Hostname()) {
		return fmt.Errorf("address not allowed: %s", u.Redacted())
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (u *Updater) statePath() string  { return filepath.Join(u.dir, "update.json") }
func (u *Updater) updatesDir() string { return filepath.Join(u.dir, "updates") }

// Status returns the line for the parent page.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return Status{Version: u.version, LastCheck: u.st.LastCheck, LastUpdate: u.st.LastUpdate, Failing: u.st.Failures >= 3}
}

func (u *Updater) save() {
	if err := core.SaveJSON(u.statePath(), u.st); err != nil {
		u.logf("update: cannot save state: %v", err)
	}
}

// settle records the result of an install started by an earlier run: done when
// this program is the pending version, failed when the time ran out.
func (u *Updater) settle() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.st.Pending == "" {
		return
	}
	now := u.now()
	switch {
	case sameVersion(u.st.Pending, u.version):
		u.logf("update: now running version %s", u.version)
		u.st.Pending, u.st.PendingAt = "", time.Time{}
		u.st.LastUpdate, u.st.LastCheck = now, now
		u.st.Failures, u.st.LastErr = 0, ""
		u.st.NextTry = now.Add(Every + u.jitter())
		u.save()
		u.cleanup()
	case !now.Before(u.st.PendingAt.Add(PendingFor)):
		v := u.st.Pending
		u.st.Pending, u.st.PendingAt = "", time.Time{}
		u.failLocked(fmt.Errorf("install of %s did not finish; still running %s", v, u.version))
	}
}

func (u *Updater) failLocked(err error) {
	u.st.Failures++
	i := min(u.st.Failures, len(backoff)) - 1
	u.st.NextTry = u.now().Add(backoff[i])
	u.st.LastErr = err.Error()
	u.logf("update: %v (keeping version %s; next try in %s)", err, u.version, backoff[i])
	u.save()
}

// wait returns how long to wait before the next check.
func (u *Updater) wait(first bool) time.Duration {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := u.now()
	if u.st.Pending != "" {
		return u.st.PendingAt.Add(PendingFor).Sub(now)
	}
	if first && u.st.Failures == 0 {
		return startDelay
	}
	return u.st.NextTry.Sub(now)
}

// Run checks at start and then every 6 hours plus up to 30 minutes, waiting
// 1, 6, then 24 hours after failures, until ctx ends.
func (u *Updater) Run(ctx context.Context) {
	if !releaseVersion(u.version) {
		u.logf("update: version %q is not a release; updates are off for this build", u.version)
		return
	}
	first := true
	for {
		u.settle()
		if !u.sleep(ctx, u.wait(first)) {
			return
		}
		first = false
		u.settle()
		if !u.due() {
			continue
		}
		_ = u.Check(ctx)
	}
}

// due reports whether a check may run now: no install is pending and, after
// a failure (a pending install that timed out counts), the back-off is over.
func (u *Updater) due() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.st.Pending != "" {
		return false
	}
	return u.st.Failures == 0 || !u.now().Before(u.st.NextTry)
}

// sameVersion compares parsed versions, so "0.4.5+abc1234" is 0.4.5.
func sameVersion(a, b string) bool {
	if a == b {
		return true
	}
	pa, ea := parseVersion(a)
	pb, eb := parseVersion(b)
	return ea == nil && eb == nil && compare(pa, pb) == 0
}

// Check runs one round: read, verify, download and start the install of a
// newer version. Errors are logged and counted for the back-off.
func (u *Updater) Check(ctx context.Context) error {
	installed, err := u.check(ctx)
	u.mu.Lock()
	defer u.mu.Unlock()
	if err != nil {
		u.cleanup()
		u.failLocked(err)
		return err
	}
	if !installed {
		u.st.LastCheck = u.now()
		u.st.Failures, u.st.LastErr = 0, ""
		u.st.NextTry = u.now().Add(Every + u.jitter())
		u.save()
	}
	return nil
}

var errNotRelease = errors.New("not a release build")

func (u *Updater) check(ctx context.Context) (installed bool, err error) {
	if !releaseVersion(u.version) {
		return false, errNotRelease
	}
	key, kind, err := u.target()
	if err != nil {
		return false, err
	}
	body, err := u.get(ctx, manifestURL, maxManifest, smallTimeout)
	if err != nil {
		return false, fmt.Errorf("manifest: %w", err)
	}
	sig, err := u.get(ctx, manifestURL+".sig", maxSig, smallTimeout)
	if err != nil {
		return false, fmt.Errorf("signature: %w", err)
	}
	m, err := Verify(body, sig, publicKey)
	if err != nil {
		return false, err
	}
	newer, err := Newer(m.Version, u.version)
	if err != nil {
		return false, err
	}
	if !newer {
		return false, nil
	}
	f, ok := m.Files[key]
	if !ok {
		return false, fmt.Errorf("version %s has no file for %s", m.Version, key)
	}
	if err := checkFile(f); err != nil {
		return false, fmt.Errorf("version %s: %w", m.Version, err)
	}
	file, err := u.download(ctx, f, kind)
	if err != nil {
		return false, fmt.Errorf("version %s: %w", m.Version, err)
	}
	if kind == "tar" {
		if file, err = unpack(file, filepath.Join(u.updatesDir(), "tar")); err != nil {
			return false, fmt.Errorf("version %s: %w", m.Version, err)
		}
	}
	// Record the pending version before starting: the installer restarts
	// the service, and the next run settles it.
	u.mu.Lock()
	u.st.Pending, u.st.PendingAt = m.Version, u.now()
	u.save()
	u.mu.Unlock()
	u.logf("update: installing version %s (running %s)", m.Version, u.version)
	if err := u.install(ctx, kind, file); err != nil {
		u.mu.Lock()
		u.st.Pending, u.st.PendingAt = "", time.Time{}
		u.mu.Unlock()
		return false, fmt.Errorf("install %s: %w", m.Version, err)
	}
	return true, nil
}

// Verify checks the base64 Ed25519 signature over the exact bytes of body and
// parses the manifest.
func Verify(body, sig []byte, key ed25519.PublicKey) (*Manifest, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return nil, errors.New("signature is not readable")
	}
	if !ed25519.Verify(key, body, raw) {
		return nil, errors.New("signature does not match")
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if m.V != 1 {
		return nil, fmt.Errorf("manifest: unknown format %d", m.V)
	}
	return &m, nil
}

var hexRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func checkFile(f File) error {
	if !strings.HasPrefix(f.URL, urlPrefix) {
		return errors.New("download address is not the project's release folder")
	}
	pu, err := url.Parse(f.URL)
	if err != nil || pu.RawQuery != "" || pu.Fragment != "" || strings.Contains(pu.Path, "..") {
		return errors.New("download address is not plain")
	}
	if err := checkURL(pu); err != nil {
		return err
	}
	if f.Size <= 0 || f.Size > MaxDownload {
		return fmt.Errorf("declared size %d is out of range", f.Size)
	}
	if !hexRe.MatchString(strings.ToLower(f.SHA256)) {
		return errors.New("declared SHA-256 is not readable")
	}
	return nil
}

// get reads a small file with a plain GET.
func (u *Updater) get(ctx context.Context, raw string, limit int64, timeout time.Duration) ([]byte, error) {
	resp, cancel, err := u.open(ctx, raw, timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("too large")
	}
	return b, nil
}

func (u *Updater) open(ctx context.Context, raw string, timeout time.Duration) (*http.Response, context.CancelFunc, error) {
	pu, err := url.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	if err := checkURL(pu); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := u.client.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return nil, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, cancel, nil
}

var fileNames = map[string]string{"exe": "raisesteady-guard-setup.exe", "deb": "raisesteady-guard.deb", "rpm": "raisesteady-guard.rpm", "tar": "raisesteady-guard.tar.gz"}

// download saves f into the updates folder (administrators only) under a
// fixed name. A half-finished or wrong file never takes that name.
func (u *Updater) download(ctx context.Context, f File, kind string) (string, error) {
	name, ok := fileNames[kind]
	if !ok {
		return "", fmt.Errorf("unknown install kind %q", kind)
	}
	dir := u.updatesDir()
	u.cleanup()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := secureDir(dir); err != nil {
		return "", fmt.Errorf("protect the updates folder: %w", err)
	}
	resp, cancel, err := u.open(ctx, f.URL, u.dlLimit)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer cancel()
	defer resp.Body.Close()
	limit := min(f.Size+f.Size/2, MaxDownload)
	if resp.ContentLength > limit {
		return "", fmt.Errorf("download is %d bytes, declared %d", resp.ContentLength, f.Size)
	}
	tmp, err := os.CreateTemp(dir, ".part-*")
	if err != nil {
		return "", err
	}
	ok = false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	if n > limit || n*3 < f.Size*2 {
		return "", fmt.Errorf("download is %d bytes, declared %d", n, f.Size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != strings.ToLower(f.SHA256) {
		return "", errors.New("download SHA-256 does not match")
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	final := filepath.Join(dir, name)
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", err
	}
	ok = true
	return final, nil
}

// cleanup removes earlier downloads.
func (u *Updater) cleanup() {
	if u.st.Pending != "" {
		return
	}
	_ = os.RemoveAll(u.updatesDir())
}

// unpack extracts a .tar.gz into dir and returns the path of its install.sh.
// Only plain files and folders are taken; links and paths that leave dir
// are refused.
func unpack(file, dir string) (string, error) {
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(bufio.NewReader(f))
	if err != nil {
		return "", fmt.Errorf("unpack: %w", err)
	}
	tr := tar.NewReader(gz)
	var total int64
	script := ""
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("unpack: %w", err)
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." {
			continue
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, `\`) {
			return "", fmt.Errorf("unpack: unsafe path %q", h.Name)
		}
		dst := filepath.Join(dir, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			total += h.Size
			if h.Size < 0 || total > maxUnpacked {
				return "", errors.New("unpack: too large")
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return "", err
			}
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o755|0o600)
			if err != nil {
				return "", err
			}
			_, err = io.Copy(out, io.LimitReader(tr, h.Size))
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return "", err
			}
			if parts := strings.Split(name, "/"); len(parts) <= 2 && parts[len(parts)-1] == "install.sh" {
				script = dst
			}
		default:
			// links and devices are skipped
		}
	}
	if script == "" {
		return "", errors.New("unpack: no install.sh")
	}
	return script, nil
}

// releaseVersion says whether v is a release version such as 0.4.4.
func releaseVersion(v string) bool { _, err := parseVersion(v); return err == nil }

type semver struct {
	n   [3]int
	pre []string
}

func parseVersion(v string) (semver, error) {
	var s semver
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	base, pre, hasPre := strings.Cut(v, "-")
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return s, fmt.Errorf("version %q is not readable", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (len(p) > 1 && p[0] == '0') {
			return s, fmt.Errorf("version %q is not readable", v)
		}
		s.n[i] = n
	}
	if hasPre {
		if pre == "" {
			return s, fmt.Errorf("version %q is not readable", v)
		}
		s.pre = strings.Split(pre, ".")
	}
	return s, nil
}

// Newer says whether candidate is strictly newer than running (semver).
func Newer(candidate, running string) (bool, error) {
	a, err := parseVersion(candidate)
	if err != nil {
		return false, err
	}
	b, err := parseVersion(running)
	if err != nil {
		return false, err
	}
	return compare(a, b) > 0, nil
}

func compare(a, b semver) int {
	for i := range a.n {
		if a.n[i] != b.n[i] {
			if a.n[i] > b.n[i] {
				return 1
			}
			return -1
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		x, y := a.pre[i], b.pre[i]
		xn, xe := strconv.Atoi(x)
		yn, ye := strconv.Atoi(y)
		switch {
		case xe == nil && ye == nil:
			if xn != yn {
				if xn > yn {
					return 1
				}
				return -1
			}
		case xe == nil:
			return -1
		case ye == nil:
			return 1
		case x != y:
			if x > y {
				return 1
			}
			return -1
		}
	}
	switch {
	case len(a.pre) > len(b.pre):
		return 1
	case len(a.pre) < len(b.pre):
		return -1
	}
	return 0
}
