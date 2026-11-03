//go:build linux

package linuxsys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/detect"
	"raisesteady-guard/agent/internal/guard"
	"raisesteady-guard/agent/internal/policy"
)

// Sys is the agent's view of a Linux computer.
type Sys struct {
	FS
	DataDir string

	once sync.Once
	reg  *policy.FileRegistry
}

// New returns the system for the data folder.
func New(dataDir string) *Sys { return &Sys{FS: FS{Root: "/"}, DataDir: dataDir} }

// Reg returns the policy files as a registry. A damaged image is set aside
// and the next apply writes everything again.
func (s *Sys) Reg() policy.Registry {
	s.once.Do(func() {
		image := filepath.Join(s.DataDir, "policy-image.json")
		r, err := policy.OpenFileRegistry(s.Root, image)
		if err != nil {
			_ = os.Rename(image, image+".damaged")
			r, _ = policy.OpenFileRegistry(s.Root, image)
		}
		s.reg = r
	})
	return s.reg
}

// MountHive is not used on Linux (all accounts mode only).
func (s *Sys) MountHive(string) (string, func() error, error) {
	return "", nil, errors.New("not used on Linux")
}

// SignedIn reports whether the account has a running session.
func (s *Sys) SignedIn(sid string) bool { return s.exists(filepath.Join("run/user", sid)) }

// LocalAccounts implements agent.System.
func (s *Sys) LocalAccounts() ([]api.LocalAccount, error) { return s.FS.LocalAccounts(), nil }

// IsPerson is the "every person's account" test of the all accounts mode.
func (s *Sys) IsPerson(sid string) bool {
	uid, err := strconv.Atoi(sid)
	return err == nil && s.IsPersonUID(uid)
}

// WholeComputerOnly: Linux browsers read policies for the whole computer.
func (s *Sys) WholeComputerOnly() bool { return true }

// DetectEnv is not used on Linux (DetectBrowsers is).
func (s *Sys) DetectEnv() detect.Env { return nil }

// GuardSys implements agent.System.
func (s *Sys) GuardSys() guard.System { return s }

// Classify implements the guard's Linux rules.
func (s *Sys) Classify(list core.UnsupportedList, path, product string) guard.Match {
	return guard.ClassifyLinux(list, path, product)
}

// Processes implements guard.System.
func (s *Sys) Processes() ([]guard.Proc, error) {
	var exp Expect
	if r, ok := s.Reg().(*policy.FileRegistry); ok && r != nil {
		exp = ExpectFrom(r.Written())
	}
	return s.FS.Processes(exp)
}

// ProductName is not used on Linux.
func (s *Sys) ProductName(string) string { return "" }

// Kill implements guard.System.
func (s *Sys) Kill(pid uint32) error { return syscall.Kill(int(pid), syscall.SIGKILL) }

// Message shows a desktop notice to the person with this uid (the session
// number is the uid). It runs notify-send as that person, in the background.
func (s *Sys) Message(session uint32, title, text string) error {
	u, ok := s.UserByUID(int(session))
	if !ok {
		return fmt.Errorf("no account %d", session)
	}
	run := fmt.Sprintf("/run/user/%d", u.UID)
	var argv []string
	switch {
	case s.exists("usr/bin/notify-send"):
		argv = []string{"/usr/bin/notify-send", "--app-name=" + core.AppName, "--icon=raisesteady-guard", title, text}
	case s.exists("usr/bin/gdbus"):
		argv = []string{"/usr/bin/gdbus", "call", "--session", "--dest", "org.freedesktop.Notifications",
			"--object-path", "/org/freedesktop/Notifications", "--method", "org.freedesktop.Notifications.Notify",
			core.AppName, "0", "raisesteady-guard", title, text, "[]", "{}", "8000"}
	default:
		return errors.New("no notify-send or gdbus")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = []string{"HOME=" + u.Home, "USER=" + u.Name, "XDG_RUNTIME_DIR=" + run,
			"DBUS_SESSION_BUS_ADDRESS=unix:path=" + run + "/bus", "PATH=/usr/bin:/bin"}
		cmd.Dir = "/"
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(u.UID), Gid: uint32(u.GID), Groups: []uint32{}}}
		_ = cmd.Run()
	}()
	return nil
}

// SecureDir makes the folder root's only.
func (s *Sys) SecureDir(path string) error {
	if err := os.Chown(path, 0, 0); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

// IsService reports whether systemd started this process.
func IsService() bool { return os.Getenv("INVOCATION_ID") != "" }

// RunService runs body until SIGTERM or SIGINT. A new folder in /run/user
// (a person signing in) is sent on logon.
func RunService(body func(ctx context.Context, logon <-chan string) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	logon := make(chan string, 8)
	go func() {
		seen := map[string]bool{}
		first := true
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			ents, _ := os.ReadDir("/run/user")
			now := map[string]bool{}
			for _, e := range ents {
				now[e.Name()] = true
				if !seen[e.Name()] && !first {
					select {
					case logon <- e.Name():
					default:
					}
				}
			}
			seen, first = now, false
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return body(ctx, logon)
}

// unitPaths are where packages and install.sh put the unit.
var unitPaths = []string{"/usr/lib/systemd/system/" + Unit, "/lib/systemd/system/" + Unit, "/etc/systemd/system/" + Unit}

const ownUnit = "/etc/systemd/system/" + Unit

// Services controls the systemd service.
type Services struct{}

func systemctl(args ...string) (int, error) {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return 0, err
}

func unitInstalled() bool {
	for _, p := range unitPaths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// Install writes the unit if no package did, then enables it.
func (Services) Install(exePath string) error {
	if !unitInstalled() {
		if err := os.WriteFile(ownUnit, []byte(UnitText(exePath)), 0o644); err != nil {
			return err
		}
	}
	if _, err := systemctl("daemon-reload"); err != nil {
		return err
	}
	_, err := systemctl("enable", Unit)
	return err
}

// Start starts the service.
func (Services) Start() error {
	_, err := systemctl("restart", Unit)
	return err
}

// Stop stops the service; nil if it is not installed.
func (Services) Stop() error {
	if !unitInstalled() {
		return nil
	}
	code, err := systemctl("stop", Unit)
	if code == 5 { // unit not loaded
		return nil
	}
	return err
}

// Delete disables the service and removes a unit Install wrote.
func (Services) Delete() error {
	if !unitInstalled() {
		return nil
	}
	if code, err := systemctl("disable", Unit); err != nil && code != 5 {
		return err
	}
	if b, err := os.ReadFile(ownUnit); err == nil && strings.HasPrefix(string(b), "# RaiseSteady Guard") {
		if err := os.Remove(ownUnit); err != nil {
			return err
		}
		_, _ = systemctl("daemon-reload")
	}
	return nil
}

// Running reports whether the service is active.
func (Services) Running() bool {
	code, _ := systemctl("is-active", "--quiet", Unit)
	return code == 0
}
