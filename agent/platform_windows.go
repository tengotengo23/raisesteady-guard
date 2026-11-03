//go:build windows

package main

import (
	"context"

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
