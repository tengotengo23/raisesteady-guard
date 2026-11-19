//go:build linux

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// platformTarget finds how Guard was installed: the tar install lives in
// /opt/raisesteady-guard; otherwise dpkg or rpm owns the package.
func platformTarget() (key, kind string, err error) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return "", "", fmt.Errorf("no updates for %s", runtime.GOARCH)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	switch {
	case strings.HasPrefix(exe, "/opt/raisesteady-guard/"):
		kind = "tar"
	case exists("/var/lib/dpkg/info/raisesteady-guard.list"):
		kind = "deb"
	case rpmOwns():
		kind = "rpm"
	default:
		return "", "", errors.New("cannot tell how Guard was installed")
	}
	return "linux-" + runtime.GOARCH + "-" + kind, kind, nil
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func rpmOwns() bool {
	rpm := firstExisting("/usr/bin/rpm", "/bin/rpm")
	if rpm == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, rpm, "-q", "--quiet", "raisesteady-guard").Run() == nil
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if exists(p) {
			return p
		}
	}
	return ""
}

// platformInstall runs the package manager in its own transient systemd unit,
// so it is not stopped with this service when the package restarts it.
func platformInstall(ctx context.Context, kind, file string) error {
	run := firstExisting("/usr/bin/systemd-run", "/bin/systemd-run")
	if run == "" {
		return errors.New("systemd-run not found")
	}
	args := []string{"--unit=raisesteady-guard-update-" + fmt.Sprint(time.Now().Unix()), "--collect", "--quiet",
		"--property=KillMode=process"}
	switch kind {
	case "deb":
		dpkg := firstExisting("/usr/bin/dpkg", "/bin/dpkg")
		if dpkg == "" {
			return errors.New("dpkg not found")
		}
		args = append(args, "--setenv=DEBIAN_FRONTEND=noninteractive", dpkg, "--force-confold", "-i", file)
	case "rpm":
		rpm := firstExisting("/usr/bin/rpm", "/bin/rpm")
		if rpm == "" {
			return errors.New("rpm not found")
		}
		args = append(args, rpm, "-U", file)
	case "tar":
		args = append(args, "/bin/sh", file)
	default:
		return fmt.Errorf("unknown install kind %q", kind)
	}
	ctx, cancel := context.WithTimeout(ctx, smallTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, run, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
