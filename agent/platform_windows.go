//go:build windows

package main

import (
	"bufio"
	"context"
	"os"
	"path/filepath"

	"raisesteady-guard/agent/internal/agent"
	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/winsys"
)

type accountLine = api.LocalAccount

func supported() bool { return true }

func platform() (agent.System, agent.Services) { return &winsys.Sys{}, winsys.Services{} }

func isAdmin() bool { return winsys.IsAdmin() }

func isService() bool { return winsys.IsService() }

func runService(body func(ctx context.Context, logon <-chan string) error) error {
	return winsys.RunService(body)
}

func dataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "RaiseSteadyGuard")
}

const adminHint = "Please run this as an administrator."

func checkAccounts(s string) (string, error) { return s, nil }

const defaultXPIPath = ""

func stdinIsTerminal() bool { return false }

func readSecret(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

func langFromEnv() string { return "en" }
