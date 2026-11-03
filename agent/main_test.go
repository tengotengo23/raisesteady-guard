package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"raisesteady-guard/agent/internal/core"
)

func TestFileURL(t *testing.T) {
	if got := fileURL(`C:\Program Files\RaiseSteadyGuard\raisesteady-guard.xpi`); got != core.DefaultFirefoxXPI {
		t.Fatalf("fileURL = %q, want %q", got, core.DefaultFirefoxXPI)
	}
}

// The add-on next to the program wins (the .tar.gz in /opt, P2-159); without one, the platform default.
func TestDefaultXPIURL(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "raisesteady-guard")
	if got := defaultXPIURL(exe); got != fileURLOrEmpty(defaultXPIPath) {
		t.Fatalf("no file next to the program: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "raisesteady-guard.xpi"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := defaultXPIURL(exe), fileURL(filepath.Join(dir, "raisesteady-guard.xpi")); got != want {
		t.Fatalf("file next to the program: %q, want %q", got, want)
	}
}

func fileURLOrEmpty(p string) string {
	if p == "" {
		return ""
	}
	return fileURL(p)
}

func TestAccountsINI(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.ini")
	err := writeAccountsINI(p, []accountLine{{SID: "S-1-5-21-1-2-3-1001", Name: "ნინო [kid]\r\n"}, {SID: "S-1-5-21-1-2-3-1000", Name: "Parent", Admin: true}}, map[string]bool{"S-1-5-21-1-2-3-1000": true}, "users")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if b[0] != 0xFF || b[1] != 0xFE {
		t.Fatal("no UTF-16 BOM")
	}
	u := make([]uint16, (len(b)-2)/2)
	for i := range u {
		u[i] = uint16(b[2+2*i]) | uint16(b[3+2*i])<<8
	}
	s := string(utf16.Decode(u))
	for _, want := range []string{"[accounts]\r\ncount=2\r\n", "installed=1\r\nmode=users", "[a1]\r\nsid=S-1-5-21-1-2-3-1001\r\nname=ნინო (kid)  \r\nadmin=0\r\nprotected=0", "[a2]", "admin=1\r\nprotected=1"} {
		if !strings.Contains(s, want) {
			t.Errorf("INI lacks %q:\n%s", want, s)
		}
	}
}

func TestRunWithoutWindows(t *testing.T) {
	var out, errb bytes.Buffer
	if run([]string{"version"}, &out, &errb) != exitOK || !strings.Contains(out.String(), core.AppName) {
		t.Fatal("version")
	}
	if run(nil, &out, &errb) != exitUsage {
		t.Fatal("no command")
	}
	if !supported() && run([]string{"status"}, &out, &errb) != exitError {
		t.Fatal("non-Windows status must fail")
	}
}
