package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type seen struct {
	method, path, query string
	header              http.Header
	body                int
}

type rig struct {
	t     *testing.T
	srv   *httptest.Server
	priv  ed25519.PrivateKey
	u     *Updater
	clock time.Time

	mu         sync.Mutex
	manifest   []byte
	sig        []byte
	files      map[string][]byte
	reqs       []seen
	installs   []string
	installErr error
	logs       []string
}

func newRig(t *testing.T, version string) *rig {
	t.Helper()
	r := &rig{t: t, files: map[string][]byte{}, clock: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	r.priv = priv
	r.srv = httptest.NewTLSServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.srv.Close)

	oldM, oldP, oldK, oldH := manifestURL, urlPrefix, publicKey, allowHost
	t.Cleanup(func() { manifestURL, urlPrefix, publicKey, allowHost = oldM, oldP, oldK, oldH })
	manifestURL = r.srv.URL + "/latest/update.json"
	urlPrefix = r.srv.URL + "/releases/download/"
	publicKey = pub
	allowHost = func(h string) bool { return h == "127.0.0.1" }

	r.u = r.updater(t.TempDir(), version)
	return r
}

func (r *rig) updater(dir, version string) *Updater {
	u := New(dir, version, func(f string, a ...any) {
		r.mu.Lock()
		r.logs = append(r.logs, f)
		r.mu.Unlock()
		r.t.Logf(f, a...)
	})
	u.client = newClient(r.srv.Client().Transport)
	u.now = func() time.Time { return r.clock }
	u.jitter = func() time.Duration { return 0 }
	u.target = func() (string, string, error) { return "windows-amd64", "exe", nil }
	u.install = func(_ context.Context, kind, file string) error {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.installs = append(r.installs, kind+":"+string(b))
		return r.installErr
	}
	return u
}

func (r *rig) serve(w http.ResponseWriter, q *http.Request) {
	body, _ := io.ReadAll(q.Body)
	r.mu.Lock()
	r.reqs = append(r.reqs, seen{q.Method, q.URL.Path, q.URL.RawQuery, q.Header.Clone(), len(body)})
	man, sig, files := r.manifest, r.sig, r.files
	r.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "track", Value: "me"})
	switch {
	case q.URL.Path == "/latest/update.json" && man != nil:
		w.Write(man)
	case q.URL.Path == "/latest/update.json.sig" && sig != nil:
		w.Write(sig)
	case q.URL.Path == "/releases/download/v9/redirect":
		http.Redirect(w, q, strings.Replace(r.srv.URL, "127.0.0.1", "localhost", 1)+"/releases/download/v9/file", http.StatusFound)
	case files[q.URL.Path] != nil:
		w.Write(files[q.URL.Path])
	default:
		http.NotFound(w, q)
	}
}

// publish signs a manifest offering content as version v; it returns the file entry.
func (r *rig) publish(v string, content []byte, edit func(*File)) File {
	sum := sha256.Sum256(content)
	f := File{Name: "setup.exe", URL: urlPrefix + "v" + v + "/setup.exe", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content))}
	if edit != nil {
		edit(&f)
	}
	m := Manifest{V: 1, Version: v, Date: "2026-11-19", Files: map[string]File{"windows-amd64": f}}
	b, _ := json.MarshalIndent(m, "", "  ")
	b = append(b, '\n')
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manifest = b
	r.sig = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, b)) + "\n")
	r.files["/releases/download/v"+v+"/setup.exe"] = content
	return f
}

func (r *rig) downloads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, q := range r.reqs {
		if strings.HasPrefix(q.path, "/releases/") {
			n++
		}
	}
	return n
}

func (r *rig) nInstalls() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.installs) }

func payload(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func (r *rig) noFinalFile() {
	r.t.Helper()
	ents, _ := os.ReadDir(r.u.updatesDir())
	if len(ents) != 0 {
		r.t.Fatalf("updates folder not empty: %v", ents)
	}
}

func TestGoodUpdate(t *testing.T) {
	r := newRig(t, "0.4.4")
	content := payload(5000)
	r.publish("0.4.5", content, nil)
	if err := r.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.nInstalls() != 1 || r.installs[0] != "exe:"+string(content) {
		t.Fatalf("installs = %d", r.nInstalls())
	}
	st := r.u.st
	if st.Pending != "0.4.5" || st.Failures != 0 {
		t.Fatalf("state = %+v", st)
	}
	checkAdminOnly(t, r.u.updatesDir())
	// The installer restarted the service as 0.4.5: the new run settles it.
	r.clock = r.clock.Add(3 * time.Minute)
	u2 := r.updater(r.u.dir, "0.4.5")
	u2.settle()
	s := u2.Status()
	if s.Version != "0.4.5" || !s.LastUpdate.Equal(r.clock) || !s.LastCheck.Equal(r.clock) || s.Failing || u2.st.Pending != "" {
		t.Fatalf("after restart: %+v %+v", s, u2.st)
	}
	if _, err := os.Stat(u2.updatesDir()); !os.IsNotExist(err) {
		t.Fatalf("download not cleaned up: %v", err)
	}
}

func TestBadSignature(t *testing.T) {
	r := newRig(t, "0.4.4")
	r.publish("0.4.5", payload(100), nil)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	r.sig = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(other, r.manifest)))
	if err := r.u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "signature does not match") {
		t.Fatalf("err = %v", err)
	}
	for _, bad := range []string{"", "not base64!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		r.sig = []byte(bad)
		if err := r.u.Check(context.Background()); err == nil {
			t.Fatalf("signature %q accepted", bad)
		}
	}
	if r.downloads() != 0 || r.nInstalls() != 0 || r.u.st.Failures != 4 {
		t.Fatalf("downloads=%d installs=%d failures=%d", r.downloads(), r.nInstalls(), r.u.st.Failures)
	}
}

func TestTamperedManifest(t *testing.T) {
	r := newRig(t, "0.4.4")
	r.publish("0.4.5", payload(100), nil)
	for _, tamper := range []func([]byte) []byte{
		func(b []byte) []byte { return bytes.Replace(b, []byte("0.4.5"), []byte("0.4.6"), 1) },
		func(b []byte) []byte { return bytes.Replace(b, []byte("127.0.0.1"), []byte("127.0.0.2"), 1) },
		func(b []byte) []byte { return append(b, ' ') },
	} {
		good := append([]byte(nil), r.manifest...)
		r.manifest = tamper(append([]byte(nil), good...))
		if err := r.u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "signature") {
			t.Fatalf("tampered manifest: err = %v", err)
		}
		r.manifest = good
	}
	if r.downloads() != 0 || r.nInstalls() != 0 {
		t.Fatalf("downloads=%d installs=%d", r.downloads(), r.nInstalls())
	}
}

func TestSignedButWrongFormat(t *testing.T) {
	r := newRig(t, "0.4.4")
	for _, body := range []string{`{"v":2,"version":"9.0.0","files":{}}`, `not json`} {
		r.manifest = []byte(body)
		r.sig = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, r.manifest)))
		if err := r.u.Check(context.Background()); err == nil {
			t.Fatalf("%s accepted", body)
		}
	}
}

func TestOldOrEqualVersion(t *testing.T) {
	for _, v := range []string{"0.4.4", "0.4.3", "0.3.99", "0.4.4-rc.1", "v0.4.4"} {
		r := newRig(t, "0.4.4")
		r.publish(v, payload(100), nil)
		if err := r.u.Check(context.Background()); err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		if r.downloads() != 0 || r.nInstalls() != 0 {
			t.Fatalf("%s: downloads=%d installs=%d", v, r.downloads(), r.nInstalls())
		}
		if !r.u.st.LastCheck.Equal(r.clock) || r.u.st.Failures != 0 || !r.u.st.NextTry.Equal(r.clock.Add(Every)) {
			t.Fatalf("%s: state %+v", v, r.u.st)
		}
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.4.5", "0.4.4", true}, {"0.5.0", "0.4.9", true}, {"1.0.0", "0.99.99", true}, {"0.4.10", "0.4.9", true},
		{"0.4.4", "0.4.4", false}, {"0.4.3", "0.4.4", false}, {"0.4.5-rc.1", "0.4.5", false}, {"0.4.5", "0.4.5-rc.1", true},
		{"0.4.5-rc.2", "0.4.5-rc.1", true}, {"0.4.5-rc.10", "0.4.5-rc.9", true}, {"v0.4.5", "0.4.4", true},
	}
	for _, c := range cases {
		got, err := Newer(c.a, c.b)
		if err != nil || got != c.want {
			t.Errorf("Newer(%s, %s) = %v, %v", c.a, c.b, got, err)
		}
	}
	for _, bad := range []string{"dev", "", "1.2", "1.2.3.4", "01.2.3", "1.x.3", "1.2.3-"} {
		if _, err := Newer(bad, "0.4.4"); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestDevBuildNeverUpdates(t *testing.T) {
	r := newRig(t, "dev")
	r.publish("9.9.9", payload(100), nil)
	if err := r.u.Check(context.Background()); !errors.Is(err, errNotRelease) {
		t.Fatalf("err = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.u.Run(ctx) // returns at once
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) != 0 {
		t.Fatalf("dev build sent %d requests", len(r.reqs))
	}
}

func TestWrongHash(t *testing.T) {
	r := newRig(t, "0.4.4")
	r.publish("0.4.5", payload(1000), func(f *File) { f.SHA256 = strings.Repeat("ab", 32) })
	if err := r.u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "SHA-256 does not match") {
		t.Fatalf("err = %v", err)
	}
	if r.downloads() != 1 || r.nInstalls() != 0 {
		t.Fatalf("downloads=%d installs=%d", r.downloads(), r.nInstalls())
	}
	r.noFinalFile()
	r.publish("0.4.5", payload(1000), func(f *File) { f.SHA256 = "nothex" })
	if err := r.u.Check(context.Background()); err == nil || r.downloads() != 1 {
		t.Fatalf("unreadable hash: err=%v downloads=%d", err, r.downloads())
	}
}

func TestWrongHost(t *testing.T) {
	r := newRig(t, "0.4.4")
	content := payload(100)
	for _, u := range []string{
		"https://evil.example/releases/download/v0.4.5/setup.exe",
		strings.Replace(urlPrefix, "https://", "http://", 1) + "v0.4.5/setup.exe",
		strings.Replace(urlPrefix, "/releases/download/", "/other/", 1) + "setup.exe",
		urlPrefix + "v0.4.5/setup.exe?x=1",
		urlPrefix + "../../evil/setup.exe",
	} {
		r.publish("0.4.5", content, func(f *File) { f.URL = u })
		if err := r.u.Check(context.Background()); err == nil {
			t.Fatalf("%s accepted", u)
		}
	}
	if r.downloads() != 0 {
		t.Fatalf("downloads = %d", r.downloads())
	}
	// A redirect to a host that is not GitHub is not followed.
	r.publish("0.4.5", content, func(f *File) { f.URL = urlPrefix + "v9/redirect" })
	r.files["/releases/download/v9/file"] = content
	err := r.u.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("redirect: err = %v", err)
	}
	if r.nInstalls() != 0 || r.downloads() != 1 {
		t.Fatalf("installs=%d downloads=%d", r.nInstalls(), r.downloads())
	}
	// The manifest itself is read only from the allowed host.
	manifestURL = "https://evil.example/update.json"
	if err := r.u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("manifest host: err = %v", err)
	}
}

func TestOversizedFile(t *testing.T) {
	r := newRig(t, "0.4.4")
	big := payload(1000)
	sum := sha256.Sum256(big)
	// Served 1000 bytes, declared 100: more than 1.5 times.
	r.publish("0.4.5", big, func(f *File) { f.Size = 100; f.SHA256 = hex.EncodeToString(sum[:]) })
	if err := r.u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("err = %v", err)
	}
	// Served far less than declared.
	r.publish("0.4.5", big, func(f *File) { f.Size = 5000 })
	if err := r.u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("small: err = %v", err)
	}
	n := r.downloads()
	// Declared over 64 MB: not even downloaded.
	r.publish("0.4.5", big, func(f *File) { f.Size = MaxDownload + 1 })
	if err := r.u.Check(context.Background()); err == nil || r.downloads() != n {
		t.Fatalf("over 64 MB: err=%v downloads=%d", err, r.downloads())
	}
	if r.nInstalls() != 0 {
		t.Fatalf("installs = %d", r.nInstalls())
	}
	r.noFinalFile()
}

func TestFailedInstallKeepsOldVersion(t *testing.T) {
	r := newRig(t, "0.4.4")
	r.publish("0.4.5", payload(200), nil)
	r.installErr = errors.New("setup refused")
	if err := r.u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "setup refused") {
		t.Fatalf("err = %v", err)
	}
	if s := r.u.Status(); s.Version != "0.4.4" || !s.LastUpdate.IsZero() || r.u.st.Pending != "" || r.u.st.Failures != 1 {
		t.Fatalf("status %+v state %+v", s, r.u.st)
	}
	r.noFinalFile()

	// The setup started but the new version never came up: after PendingFor
	// the old version records a failure and backs off.
	r.installErr = nil
	if err := r.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.clock = r.clock.Add(PendingFor - time.Second)
	r.u.settle()
	if r.u.st.Pending != "0.4.5" {
		t.Fatalf("settled too early: %+v", r.u.st)
	}
	r.clock = r.clock.Add(time.Second)
	u2 := r.updater(r.u.dir, "0.4.4") // the service restarted with the old agent
	u2.settle()
	if u2.st.Pending != "" || u2.st.Failures != 2 || !strings.Contains(u2.st.LastErr, "did not finish") || !u2.st.NextTry.Equal(r.clock.Add(6*time.Hour)) {
		t.Fatalf("state %+v", u2.st)
	}
	if !u2.Status().LastUpdate.IsZero() {
		t.Fatal("LastUpdate set after a failed install")
	}
}

func TestBackoff(t *testing.T) {
	r := newRig(t, "0.4.4") // no manifest published: every check fails
	for i, want := range []time.Duration{time.Hour, 6 * time.Hour, 24 * time.Hour, 24 * time.Hour} {
		if err := r.u.Check(context.Background()); err == nil {
			t.Fatal("check passed")
		}
		if got := r.u.st.NextTry.Sub(r.clock); got != want {
			t.Fatalf("failure %d: wait %s, want %s", i+1, got, want)
		}
		if f := r.u.Status().Failing; f != (i >= 2) {
			t.Fatalf("failure %d: failing = %v", i+1, f)
		}
	}
	// State survives a restart.
	u2 := r.updater(r.u.dir, "0.4.4")
	if u2.st.Failures != 4 || u2.wait(true) != 24*time.Hour {
		t.Fatalf("after restart: %+v wait %s", u2.st, u2.wait(true))
	}
	r.publish("0.4.4", payload(10), nil)
	if err := r.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.u.st.Failures != 0 || r.u.st.LastErr != "" || r.u.Status().Failing || r.u.st.NextTry.Sub(r.clock) != Every {
		t.Fatalf("after success: %+v", r.u.st)
	}
}

func TestRunWaits(t *testing.T) {
	r := newRig(t, "0.4.4") // checks fail
	r.u.jitter = func() time.Duration { return 7 * time.Minute }
	var waits []time.Duration
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.u.sleep = func(_ context.Context, d time.Duration) bool {
		waits = append(waits, d)
		r.clock = r.clock.Add(d)
		switch len(waits) {
		case 4:
			r.publish("0.4.4", payload(10), nil) // the next check passes
		case 6:
			return false
		}
		return true
	}
	r.u.Run(ctx)
	want := []time.Duration{startDelay, time.Hour, 6 * time.Hour, 24 * time.Hour, Every + 7*time.Minute, Every + 7*time.Minute}
	if len(waits) != len(want) {
		t.Fatalf("waits = %v", waits)
	}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("waits = %v, want %v", waits, want)
		}
	}
}

// Review M1: a release build reports "0.4.5+abc1234"; the pending "0.4.5"
// install must count as done.
func TestPlusBuildVersionSettles(t *testing.T) {
	r := newRig(t, "0.4.4")
	r.publish("0.4.5", payload(200), nil)
	if err := r.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.u.st.Pending != "0.4.5" {
		t.Fatalf("not pending: %+v", r.u.st)
	}
	r.clock = r.clock.Add(time.Minute)
	u2 := r.updater(r.u.dir, "0.4.5+abc1234") // the new agent started
	u2.settle()
	if u2.st.Pending != "" || u2.st.Failures != 0 || u2.st.LastErr != "" || !u2.Status().LastUpdate.Equal(r.clock) {
		t.Fatalf("state %+v", u2.st)
	}
	// Still settled after the pending time would have run out.
	r.clock = r.clock.Add(PendingFor)
	u2.settle()
	if u2.st.Failures != 0 {
		t.Fatalf("counted as failed: %+v", u2.st)
	}
}

// Review M2: when a pending install times out, Run must wait the back-off
// (1 h) before the next check, not check again at once.
func TestRunHonoursBackoffAfterPendingTimeout(t *testing.T) {
	r := newRig(t, "0.4.4")
	r.publish("0.4.5", payload(200), nil)
	r.u.jitter = func() time.Duration { return 7 * time.Minute }
	type step struct {
		wait     time.Duration
		installs int
	}
	var steps []step
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.u.sleep = func(_ context.Context, d time.Duration) bool {
		steps = append(steps, step{d, r.nInstalls()})
		r.clock = r.clock.Add(d)
		return len(steps) < 4
	}
	r.u.Run(ctx)
	// startDelay, then the install is pending for PendingFor; it never comes
	// up (still 0.4.4), so the failure waits 1 h before the next try.
	want := []step{{startDelay, 0}, {PendingFor, 1}, {time.Hour, 1}, {PendingFor, 2}}
	if len(steps) != len(want) {
		t.Fatalf("steps = %+v, want %+v", steps, want)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Fatalf("steps = %+v, want %+v", steps, want)
		}
	}
	if r.u.st.Failures != 1 {
		t.Fatalf("state %+v", r.u.st)
	}
}

func TestOnlyPlainGET(t *testing.T) {
	r := newRig(t, "0.4.4")
	r.publish("0.4.5", payload(300), nil)
	if err := r.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) != 3 {
		t.Fatalf("requests = %d", len(r.reqs))
	}
	for _, q := range r.reqs {
		if q.method != http.MethodGet || q.query != "" || q.body != 0 {
			t.Errorf("%s %s?%s body %d", q.method, q.path, q.query, q.body)
		}
		if q.header.Get("User-Agent") != "RaiseSteadyGuard" {
			t.Errorf("User-Agent = %q", q.header.Get("User-Agent"))
		}
		for k := range q.header {
			if k != "User-Agent" && k != "Accept-Encoding" {
				t.Errorf("%s sent header %s", q.path, k)
			}
		}
	}
}

func TestBuiltInAddressesAndKey(t *testing.T) {
	if len(publicKey) != ed25519.PublicKeySize {
		t.Fatal("key size")
	}
	if manifestURL != "https://github.com/tengotengo23/raisesteady-guard/releases/latest/download/update.json" ||
		urlPrefix != "https://github.com/tengotengo23/raisesteady-guard/releases/download/" {
		t.Fatal("built-in addresses changed")
	}
	for host, ok := range map[string]bool{"github.com": true, "objects.githubusercontent.com": true, "release-assets.githubusercontent.com": true,
		"evil.com": false, "github.com.evil.com": false, "githubusercontent.com.evil.com": false, "127.0.0.1": false} {
		if allowHost(host) != ok {
			t.Errorf("allowHost(%s) = %v", host, !ok)
		}
	}
	if c := newClient(nil); c.Jar != nil || c.CheckRedirect == nil {
		t.Fatal("client keeps cookies or follows any redirect")
	}
}

func tarGz(t *testing.T, entries map[string]string, link string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	if link != "" {
		tw.WriteHeader(&tar.Header{Name: link, Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink})
	}
	tw.Close()
	gz.Close()
	p := filepath.Join(t.TempDir(), "x.tar.gz")
	os.WriteFile(p, buf.Bytes(), 0o600)
	return p
}

func TestUnpack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tar")
	good := tarGz(t, map[string]string{"raisesteady-guard-0.4.5-linux-amd64/install.sh": "echo hi", "raisesteady-guard-0.4.5-linux-amd64/raisesteady-guard": "bin"}, "raisesteady-guard-0.4.5-linux-amd64/link")
	script, err := unpack(good, dir)
	if err != nil || script != filepath.Join(dir, "raisesteady-guard-0.4.5-linux-amd64", "install.sh") {
		t.Fatalf("script=%s err=%v", script, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "raisesteady-guard-0.4.5-linux-amd64", "link")); !os.IsNotExist(err) {
		t.Fatal("symlink unpacked")
	}
	for _, name := range []string{"../evil.sh", "/etc/evil", "a/../../evil"} {
		if _, err := unpack(tarGz(t, map[string]string{name: "x", "d/install.sh": "x"}, ""), dir); err == nil {
			t.Errorf("%s unpacked", name)
		}
	}
	if _, err := unpack(tarGz(t, map[string]string{"d/other.sh": "x"}, ""), dir); err == nil {
		t.Error("tar without install.sh accepted")
	}
}

func TestTarUpdateRunsInstallScript(t *testing.T) {
	r := newRig(t, "0.4.4")
	r.u.target = func() (string, string, error) { return "windows-amd64", "tar", nil }
	var got string
	r.u.install = func(_ context.Context, kind, file string) error { got = kind + ":" + file; return nil }
	b, _ := os.ReadFile(tarGz(t, map[string]string{"g/install.sh": "echo", "g/raisesteady-guard": "bin"}, ""))
	r.publish("0.4.5", b, nil)
	if err := r.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := "tar:" + filepath.Join(r.u.updatesDir(), "tar", "g", "install.sh"); got != want {
		t.Fatalf("install = %s, want %s", got, want)
	}
}
