//go:build !windows

package linuxsys

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/guard"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, root, rel, target string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Fatal(err)
	}
}

func data(t *testing.T) *core.Data {
	t.Helper()
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

const passwd = `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
mum:x:1000:1000:Mum:/home/mum:/bin/bash
kid:x:1001:1001:Kid:/home/kid:/bin/bash
svc:x:1002:1002::/var/lib/svc:/usr/sbin/nologin
nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin
`

func TestAccountsAndAdmins(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/passwd", passwd)
	write(t, root, "etc/group", "root:x:0:\nsudo:x:27:mum\nkid:x:1001:\nwheel:x:10:\n")
	f := FS{Root: root}
	accts := f.LocalAccounts()
	if len(accts) != 2 || accts[0].Name != "mum" || !accts[0].Admin || accts[1].Name != "kid" || accts[1].Admin || accts[1].SID != "1001" {
		t.Fatalf("accounts = %+v", accts)
	}
	for uid, want := range map[int]bool{0: false, 999: false, 1000: true, 1001: true, 65534: false} {
		if got := f.IsPersonUID(uid); got != want {
			t.Errorf("IsPersonUID(%d) = %v", uid, got)
		}
	}
	write(t, root, "etc/login.defs", "UID_MIN 500\nUID_MAX 60000\n")
	if !f.IsPersonUID(500) {
		t.Error("UID_MIN from login.defs ignored")
	}
}

func TestLanguages(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/passwd", passwd)
	write(t, root, "etc/default/locale", "LANG=\"ka_GE.UTF-8\"\n")
	write(t, root, "var/lib/AccountsService/users/kid", "[User]\nLanguage=ru_RU.UTF-8\n")
	f := FS{Root: root}
	if f.SystemLang() != "ka" {
		t.Errorf("SystemLang = %q", f.SystemLang())
	}
	if f.UserLang("1001") != "ru" || f.UserLang("1000") != "" {
		t.Errorf("UserLang = %q %q", f.UserLang("1001"), f.UserLang("1000"))
	}
	env := map[string]string{"LANG": "ru_RU.UTF-8"}
	if LangFromEnv(func(k string) string { return env[k] }) != "ru" {
		t.Error("LangFromEnv ru")
	}
	if LangFromEnv(func(string) string { return "" }) != "en" {
		t.Error("LangFromEnv default")
	}
}

// proc adds a fake process.
func proc(t *testing.T, root string, pid, ppid, uid int, exe, ns string) {
	t.Helper()
	dir := filepath.Join("proc", strconv.Itoa(pid))
	write(t, root, filepath.Join(dir, "status"), "Name:\tx\nPPid:\t"+strconv.Itoa(ppid)+"\nUid:\t"+strconv.Itoa(uid)+"\t"+strconv.Itoa(uid)+"\t0\t0\n")
	link(t, root, filepath.Join(dir, "exe"), exe)
	link(t, root, filepath.Join(dir, "ns", "mnt"), ns)
	if err := os.MkdirAll(filepath.Join(root, dir, "root"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestProcessesVerifyThatBrowsersSeeOurPolicies(t *testing.T) {
	root := t.TempDir()
	const host = "mnt:[4026531841]"
	ours := `{"IncognitoModeAvailability": 1}`
	exp := Expect{"chrome": {"etc/opt/chrome/policies/managed/raisesteady-guard.json": sum([]byte(ours))},
		"chromium": {"etc/chromium/policies/managed/raisesteady-guard.json": sum([]byte(ours)), "etc/chromium-browser/policies/managed/raisesteady-guard.json": sum([]byte(ours))},
		"firefox":  {"etc/firefox/policies/policies.json": sum([]byte(`{"policies":{}}`))}}
	proc(t, root, 1, 0, 0, "/usr/lib/systemd/systemd", host)
	proc(t, root, 100, 1, 1001, "/usr/bin/bash", host)
	proc(t, root, 101, 100, 1001, "/opt/google/chrome/chrome", host)               // main, host namespace
	proc(t, root, 102, 101, 1001, "/opt/google/chrome/chrome", "mnt:[4026532000]") // its sandboxed child
	proc(t, root, 200, 100, 1001, "/opt/google/chrome/chrome", "mnt:[4026532100]") // hidden policies
	proc(t, root, 201, 200, 1001, "/opt/google/chrome/chrome", "mnt:[4026532101]") // child of the hidden one
	proc(t, root, 300, 100, 1001, "/snap/firefox/1234/usr/lib/firefox/firefox", "mnt:[4026532200]")
	write(t, root, "proc/300/root/etc/firefox/policies/policies.json", `{"policies":{}}`)
	proc(t, root, 400, 100, 1001, "/home/kid/Downloads/chrome-linux/chrome (deleted)", host)
	proc(t, root, 500, 100, 1001, "/usr/bin/bwrap", host)
	write(t, root, "proc/500/root/.flatpak-info", "[Application]\nname=com.opera.Opera\n[Instance]\napp-path=/var/lib/flatpak/app/com.opera.Opera/x86_64/stable/abc/files\n")
	proc(t, root, 501, 100, 1001, "/app/lib/firefox/firefox", host)
	write(t, root, "proc/501/root/.flatpak-info", "[Application]\nname=org.mozilla.firefox\n[Instance]\napp-path=/home/kid/.local/share/flatpak/app/org.mozilla.firefox/x86_64/stable/abc/files\n")
	// Claims to be the system Firefox Flatpak but runs another program.
	proc(t, root, 502, 100, 1001, "/tmp/fake/chromium", host)
	write(t, root, "proc/502/root/.flatpak-info", "[Application]\nname=org.mozilla.firefox\n[Instance]\napp-path=/var/lib/flatpak/app/org.mozilla.firefox/x86_64/stable/abc/files\n")
	// Claims to be a Flatpak that is not a browser, but runs Chromium (security review, 3 Nov).
	proc(t, root, 503, 100, 1001, "/home/kid/c/chrome", host)
	write(t, root, "proc/503/root/.flatpak-info", "[Application]\nname=com.spotify.Client\n[Instance]\napp-path=/var/lib/flatpak/app/com.spotify.Client/x86_64/stable/abc/files\n")
	// A real non-browser Flatpak stays open.
	proc(t, root, 504, 100, 1001, "/app/extra/share/spotify/spotify", host)
	write(t, root, "proc/504/root/.flatpak-info", "[Application]\nname=com.spotify.Client\n[Instance]\napp-path=/var/lib/flatpak/app/com.spotify.Client/x86_64/stable/abc/files\n")
	// Snap Chromium in its own namespace sees only one of its two files.
	proc(t, root, 600, 100, 1001, "/snap/chromium/3000/usr/lib/chromium-browser/chrome", "mnt:[4026532300]")
	write(t, root, "proc/600/root/etc/chromium-browser/policies/managed/raisesteady-guard.json", ours)
	// Firefox whose policy file cannot be read (a private mount refusing root).
	proc(t, root, 700, 100, 1001, "/snap/firefox/1234/usr/lib/firefox/firefox", "mnt:[4026532400]")
	write(t, root, "proc/700/root/etc/firefox/policies/policies.json", `{"policies":{}}`)
	if err := os.Chmod(filepath.Join(root, "proc/700/root/etc/firefox/policies/policies.json"), 0); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "proc/9/task"), 0o755); err != nil { // a kernel thread: no exe
		t.Fatal(err)
	}
	write(t, root, "proc/9/status", "Name:\tkthreadd\nPPid:\t0\nUid:\t0\t0\t0\t0\n")

	procs, err := FS{Root: root}.Processes(exp)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uint32]string{}
	for _, p := range procs {
		got[p.PID] = p.Path
		if p.SID != strconv.Itoa(int(p.Session)) {
			t.Errorf("pid %d: SID %q, session %d", p.PID, p.SID, p.Session)
		}
	}
	want := map[uint32]string{
		101: "/opt/google/chrome/chrome",
		102: "/opt/google/chrome/chrome",
		200: "unverified:/opt/google/chrome/chrome",
		300: "/snap/firefox/1234/usr/lib/firefox/firefox",
		400: "/home/kid/Downloads/chrome-linux/chrome",
		500: "flatpak-claim:com.opera.Opera:/usr/bin/bwrap",
		501: "flatpak-claim:org.mozilla.firefox:/app/lib/firefox/firefox",
		502: "flatpak-claim:org.mozilla.firefox:/tmp/fake/chromium",
		503: "flatpak-claim:com.spotify.Client:/home/kid/c/chrome",
		504: "flatpak-claim:com.spotify.Client:/app/extra/share/spotify/spotify",
		600: "unverified:/snap/chromium/3000/usr/lib/chromium-browser/chrome",
	}
	if os.Geteuid() != 0 { // root reads a mode-0 file
		want[700] = "unverified:/snap/firefox/1234/usr/lib/firefox/firefox"
	}
	for pid, w := range want {
		if got[pid] != w {
			t.Errorf("pid %d path = %q, want %q", pid, got[pid], w)
		}
	}
	if _, ok := got[9]; ok {
		t.Error("kernel thread listed")
	}
	// The guard then closes exactly the hidden one and the copies.
	list := data(t).Unsupported
	closed := map[uint32]string{}
	for _, p := range procs {
		if m := guard.ClassifyLinux(list, p.Path, ""); m.Unsupported != nil {
			closed[p.PID] = m.Unsupported.ID
		}
	}
	wantClosed := map[uint32]string{200: "chrome-other", 201: "chrome-other", 400: "chromium", 500: "opera", 501: "firefox-other", 502: "firefox-other", 503: "chromium", 600: "chromium"}
	if os.Geteuid() != 0 {
		wantClosed[700] = "firefox-other"
	}
	for pid, id := range wantClosed {
		if closed[pid] != id {
			t.Errorf("pid %d closed as %q, want %q", pid, closed[pid], id)
		}
	}
	for pid, id := range closed {
		if _, ok := wantClosed[pid]; !ok {
			t.Errorf("pid %d closed as %s, should stay open", pid, id)
		}
	}
}

func TestDetectBrowsers(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/passwd", passwd)
	write(t, root, "opt/google/chrome/chrome", "")
	write(t, root, "usr/lib64/firefox/firefox", "")
	write(t, root, "var/lib/flatpak/app/org.mozilla.firefox/current", "")
	write(t, root, "var/lib/flatpak/app/com.google.Chrome/current", "")
	write(t, root, "home/kid/.local/share/flatpak/app/org.mozilla.firefox/current", "")
	write(t, root, "snap/brave/current/x", "")
	write(t, root, "usr/lib/x86_64-linux-gnu/opera/opera", "")
	link(t, root, "usr/bin/opera", "../lib/x86_64-linux-gnu/opera/opera")
	write(t, root, "usr/bin/firefox", "#!/bin/sh\nexec /usr/lib64/firefox/firefox \"$@\"\n") // a wrapper: not scanned
	found := FS{Root: root}.DetectBrowsers(data(t).Unsupported)
	var sup, uns []string
	for _, f := range found {
		if f.Supported {
			sup = append(sup, f.Name)
		} else {
			uns = append(uns, f.ID+"@"+f.Path+"@"+f.ProfileSID)
		}
	}
	if strings.Join(sup, ",") != "Google Chrome,Firefox,Firefox (Flatpak)" {
		t.Errorf("supported = %v", sup)
	}
	wantU := []string{
		"chrome-other@/var/lib/flatpak/app/com.google.Chrome@",
		"brave-other@/snap/brave@",
		"firefox-other@/home/kid/.local/share/flatpak/app/org.mozilla.firefox@1001",
		"opera@/usr/lib/x86_64-linux-gnu/opera/opera@",
	}
	for _, w := range wantU {
		ok := false
		for _, u := range uns {
			ok = ok || u == w
		}
		if !ok {
			t.Errorf("missing unsupported %s in %v", w, uns)
		}
	}
	if len(uns) != len(wantU) {
		t.Errorf("unsupported = %v", uns)
	}
}

func TestExpectFrom(t *testing.T) {
	exp := ExpectFrom(map[string]string{
		"etc/opt/chrome/policies/managed/raisesteady-guard.json":           "a",
		"etc/chromium-browser/policies/managed/raisesteady-guard.json":     "b",
		"etc/firefox/policies/policies.json":                               "c",
		"var/lib/flatpak/extension/x/x86_64/stable/policies/policies.json": "d",
	})
	if exp["chrome"]["etc/opt/chrome/policies/managed/raisesteady-guard.json"] != "a" || exp["chromium"] == nil || exp["firefox"] == nil || len(exp) != 3 {
		t.Errorf("expect = %v", exp)
	}
}
