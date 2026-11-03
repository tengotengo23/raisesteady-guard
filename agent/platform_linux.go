//go:build linux

package main

import (
	"bufio"
	"context"
	"errors"
	"os"

	"raisesteady-guard/agent/internal/agent"
	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/linuxsys"

	"golang.org/x/sys/unix"
)

type accountLine = api.LocalAccount

func supported() bool { return true }

func platform() (agent.System, agent.Services) {
	return linuxsys.New(dataDir()), linuxsys.Services{}
}

func isAdmin() bool { return os.Geteuid() == 0 }

func isService() bool { return linuxsys.IsService() }

func runService(body func(ctx context.Context, logon <-chan string) error) error {
	return linuxsys.RunService(body)
}

func dataDir() string { return "/var/lib/raisesteady-guard" }

const adminHint = "Please run this with sudo."

// Linux browsers read policies for the whole computer, so every person's
// account is protected.
func checkAccounts(s string) (string, error) {
	if s == "" || s == "all" {
		return "all", nil
	}
	return "", errors.New("on Linux the browser settings apply to every account: use --accounts all")
}

// The package puts the signed Firefox add-on next to the program.
const defaultXPIPath = "/usr/lib/raisesteady-guard/raisesteady-guard.xpi"

func stdinIsTerminal() bool {
	_, err := unix.IoctlGetTermios(0, unix.TCGETS)
	return err == nil
}

// readSecret reads a line from the terminal with echo off.
func readSecret(in *bufio.Reader) (string, error) {
	old, err := unix.IoctlGetTermios(0, unix.TCGETS)
	if err == nil {
		quiet := *old
		quiet.Lflag &^= unix.ECHO
		if unix.IoctlSetTermios(0, unix.TCSETS, &quiet) == nil {
			defer unix.IoctlSetTermios(0, unix.TCSETS, old)
		}
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

func langFromEnv() string { return linuxsys.LangFromEnv(os.Getenv) }
