//go:build !windows

package main

import (
	"context"
	"errors"

	"raisesteady-guard/agent/internal/agent"
	"raisesteady-guard/agent/internal/api"
)

// The agent runs on Windows only; on other systems the commands say so.
// This file lets `go vet` and `go test` run on the build Mac.

type accountLine = api.LocalAccount

func supported() bool { return false }

func platform() (agent.System, agent.Services) { return nil, nil }

func isAdmin() bool { return false }

func isService() bool { return false }

func runService(func(ctx context.Context, logon <-chan string) error) error {
	return errors.New("windows only")
}
