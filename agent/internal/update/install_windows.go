//go:build windows

package update

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// platformTarget: the Windows installer carries both agents (amd64, arm64).
func platformTarget() (key, kind string, err error) {
	return "windows-amd64", "exe", nil
}

// platformInstall starts the verified setup silently in update mode
// (/S /UPDATE): it keeps the account, age, PIN and settings, stops and
// restarts the service, and restores the old agent if anything fails. The
// service runs in session 0, so nothing appears on the child's screen. The
// setup outlives this process, which the setup stops.
//
// TMP and TEMP point at <data>\updates\tmp, which inherits the admin-only
// folder's rights: run as SYSTEM, NSIS would otherwise unpack its plug-in
// DLLs into C:\Windows\Temp, where ordinary users can create files.
func platformInstall(ctx context.Context, kind, file string) error {
	tmp := filepath.Join(filepath.Dir(file), "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	env := setupEnv(os.Environ(), tmp)
	start := func(flags uint32) error {
		cmd := exec.Command(file, "/S", "/UPDATE")
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	base := uint32(windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP)
	if err := start(base | windows.CREATE_BREAKAWAY_FROM_JOB); err == nil {
		return nil
	}
	// Breakaway is refused when the service runs in a job that forbids it.
	return start(base)
}

// setupEnv returns env with TMP and TEMP replaced by dir.
func setupEnv(env []string, dir string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "TMP") || strings.EqualFold(k, "TEMP") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "TMP="+dir, "TEMP="+dir)
}
