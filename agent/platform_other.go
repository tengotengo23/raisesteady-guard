//go:build !windows && !linux

package main

import (
	"bufio"
	"context"
	"errors"
	"io"

	"raisesteady-guard/agent/internal/agent"
	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/core"
)

// The agent runs on Windows and Linux; on other systems the commands say so.
// This file lets `go vet` and `go test` run on the build Mac.

type accountLine = api.LocalAccount

func supported() bool { return false }

func platform(*core.Data) (agent.System, agent.Services) { return nil, nil }

func openForDesktopUsers(agent.System, string) int { return 0 }

func openPage(agent.System, bool, bool, io.Writer) int { return exitError }

func isAdmin() bool { return false }

func isService() bool { return false }

func runService(func(ctx context.Context, logon <-chan string) error) error {
	return errors.New("windows only")
}

func dataDir() string { return "" }

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
