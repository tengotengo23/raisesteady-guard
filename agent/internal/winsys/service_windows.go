//go:build windows

package winsys

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"raisesteady-guard/agent/internal/core"
)

// Services implements agent.Services with the service control manager.
type Services struct{}

func open() (*mgr.Mgr, *mgr.Service, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, nil, err
	}
	s, err := m.OpenService(core.ServiceName)
	if err != nil {
		m.Disconnect()
		return nil, nil, err
	}
	return m, s, nil
}

// Install creates (or updates) the service: LocalSystem, automatic start,
// restart on every failure.
func (Services) Install(exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	conf := mgr.Config{
		DisplayName:  core.AppName,
		Description:  core.AppName + ": keeps the browsers safe for the protected Windows accounts.",
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
	}
	s, err := m.OpenService(core.ServiceName)
	if err == nil {
		cur, err := s.Config()
		if err == nil {
			cur.BinaryPathName = fmt.Sprintf(`"%s" service`, exePath)
			cur.DisplayName, cur.Description, cur.StartType = conf.DisplayName, conf.Description, conf.StartType
			err = s.UpdateConfig(cur)
		}
		if err != nil {
			s.Close()
			return err
		}
	} else {
		s, err = m.CreateService(core.ServiceName, exePath, conf, "service")
		if err != nil {
			return err
		}
	}
	defer s.Close()
	acts := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}
	if err := s.SetRecoveryActions(acts, 86400); err != nil {
		return err
	}
	return s.SetRecoveryActionsOnNonCrashFailures(true)
}

// Start starts the service.
func (Services) Start() error {
	m, s, err := open()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	defer s.Close()
	if st, err := s.Query(); err == nil && (st.State == svc.Running || st.State == svc.StartPending) {
		return nil
	}
	return s.Start()
}

// Stop stops the service and waits up to 30 seconds.
func (Services) Stop() error {
	m, s, err := open()
	if err != nil {
		return nil // not installed
	}
	defer m.Disconnect()
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return err
	}
	if st.State == svc.Stopped {
		return nil
	}
	if st.State != svc.StopPending {
		if st, err = s.Control(svc.Stop); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != svc.Stopped {
		if time.Now().After(deadline) {
			return errors.New("the service did not stop in 30 seconds")
		}
		time.Sleep(300 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the service.
func (Services) Delete() error {
	m, s, err := open()
	if err != nil {
		return nil
	}
	defer m.Disconnect()
	defer s.Close()
	return s.Delete()
}

// Running reports whether the service runs.
func (Services) Running() bool {
	m, s, err := open()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	defer s.Close()
	st, err := s.Query()
	return err == nil && st.State == svc.Running
}

// IsService reports whether the process was started by the service manager.
func IsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

// RunService runs body as the RaiseSteadyGuard service. body gets a context that
// ends at stop or shutdown, and a channel of SIDs that signed in.
func RunService(body func(ctx context.Context, logon <-chan string) error) error {
	return svc.Run(core.ServiceName, &handler{body: body})
}

type handler struct {
	body func(ctx context.Context, logon <-chan string) error
}

const wtsSessionLogon = 0x5

func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logon := make(chan string, 8)
	done := make(chan error, 1)
	go func() { done <- h.body(ctx, logon) }()
	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptSessionChange}
	for {
		select {
		case err := <-done:
			if err != nil {
				// A non-zero exit code makes the service manager restart us.
				return true, 1
			}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(15 * time.Second):
				}
				return false, 0
			case svc.SessionChange:
				if c.EventType == wtsSessionLogon && c.EventData != 0 {
					// Copy the notification without turning a uintptr into a pointer.
					var n windows.WTSSESSION_NOTIFICATION
					size := unsafe.Sizeof(n)
					if windows.ReadProcessMemory(windows.CurrentProcess(), c.EventData, (*byte)(unsafe.Pointer(&n)), size, nil) != nil {
						continue
					}
					if sid, err := SessionSID(n.SessionID); err == nil {
						select {
						case logon <- sid:
						default:
						}
					}
				}
			}
		}
	}
}
