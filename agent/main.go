// Command raisesteady-guard-agent is the Windows service and command-line tool of
// RaiseSteady Guard (code name RaiseSteady Guard). Run "raisesteady-guard-agent help".
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"raisesteady-guard/agent/internal/agent"
	"raisesteady-guard/agent/internal/core"
)

var version = "dev"

// Exit codes the installer reads.
const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitWrongPin = 3
	exitLocked   = 4
	exitNotAdmin = 5
)

const usage = `raisesteady-guard-agent: the Windows service of ` + core.AppName + `.

  service                         run as the RaiseSteadyGuard service (the service manager does this)
  install --accounts all|SID,SID --band ID (--pin PIN | --pin-env NAME) [--port N] [--firefox-xpi URL] [--family-dns on|off]
  uninstall (--pin PIN | --pin-env NAME)
  verify-pin (--pin PIN | --pin-env NAME)
  apply-once                      write the policies now (administrators)
  status                          show the state as JSON (no PIN, no tokens)
  list-accounts [--ini PATH]      list this PC's accounts (for the installer)
  version

Exit codes: 0 ok, 1 error, 2 usage, 3 wrong PIN, 4 PIN locked, 5 needs an administrator.
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func dataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "RaiseSteadyGuard")
}

// logger writes to agent.log in the data folder (rotated at 1 MB) and, for
// commands, to stderr. It never receives a PIN or a token.
type logger struct {
	mu   sync.Mutex
	path string
	echo io.Writer
}

func (l *logger) logf(format string, args ...any) {
	line := time.Now().Format(time.RFC3339) + " " + fmt.Sprintf(format, args...) + "\n"
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.echo != nil {
		io.WriteString(l.echo, line)
	}
	if l.path == "" {
		return
	}
	if st, err := os.Stat(l.path); err == nil && st.Size() > 1<<20 {
		_ = os.Rename(l.path, l.path+".1")
	}
	if f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.WriteString(line)
		f.Close()
	}
}

func readPin(fs *flag.FlagSet, pin, pinEnv *string) (string, error) {
	p := *pin
	if *pinEnv != "" {
		if !strings.HasPrefix(*pinEnv, "KG_") {
			return "", errors.New("--pin-env must name a KG_ variable")
		}
		p = os.Getenv(*pinEnv)
	}
	if !core.ValidPin(p) {
		return "", errors.New("the PIN must be 4 to 8 digits")
	}
	return p, nil
}

func codeFor(err error, stderr io.Writer) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, core.ErrLocked):
		fmt.Fprintln(stderr, "PIN checks are locked for a while after five wrong tries.")
		return exitLocked
	case errors.Is(err, agent.ErrWrongPin):
		fmt.Fprintln(stderr, "The PIN is not right.")
		return exitWrongPin
	case errors.Is(err, agent.ErrUsage):
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	fmt.Fprintln(stderr, "error:", err)
	return exitError
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	cmd, rest := args[0], args[1:]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	accounts := fs.String("accounts", "", "all or SIDs")
	band := fs.String("band", "", "age band id")
	pin := fs.String("pin", "", "parent PIN")
	pinEnv := fs.String("pin-env", "", "environment variable holding the PIN")
	port := fs.Int("port", core.DefaultPort, "localhost port")
	xpi := fs.String("firefox-xpi", "", "Firefox install_url")
	familyDNS := fs.String("family-dns", "on", "on or off: the AdGuard DNS Family resolver for new accounts")
	ini := fs.String("ini", "", "write an INI file")
	if err := fs.Parse(rest); err != nil {
		return exitUsage
	}
	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	case "version":
		fmt.Fprintln(stdout, core.AppName, version)
		return exitOK
	}
	if !supported() {
		fmt.Fprintln(stderr, "This command works on Windows only.")
		return exitError
	}
	data, err := core.LoadData()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	paths := agent.PathsIn(dataDir())
	sys, svcs := platform()
	needAdmin := func() bool {
		if isAdmin() {
			return true
		}
		fmt.Fprintln(stderr, "Please run this as an administrator.")
		return false
	}
	switch cmd {
	case "service":
		if !isService() {
			fmt.Fprintln(stderr, "The service manager starts this command.")
			return exitUsage
		}
		lg := &logger{path: paths.Log}
		err := runService(func(ctx context.Context, logon <-chan string) error {
			return serve(ctx, logon, sys, data, paths, lg)
		})
		if err != nil {
			lg.logf("service ended: %v", err)
			return exitError
		}
		return exitOK
	case "install":
		if !needAdmin() {
			return exitNotAdmin
		}
		p, err := readPin(fs, pin, pinEnv)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsage
		}
		if *familyDNS != "on" && *familyDNS != "off" {
			fmt.Fprintln(stderr, "--family-dns must be on or off")
			return exitUsage
		}
		exe, err := os.Executable()
		if err != nil {
			return codeFor(err, stderr)
		}
		if *xpi == "" {
			if x := filepath.Join(filepath.Dir(exe), "raisesteady-guard.xpi"); fileExists(x) {
				*xpi = fileURL(x)
			}
		}
		res, err := agent.Install(sys, svcs, data, paths, agent.InstallOptions{
			Accounts: *accounts, Band: *band, Pin: p, Port: *port, ExePath: exe, FirefoxXPI: *xpi,
			FamilyDNSOff: *familyDNS == "off",
		}, nil)
		lg := &logger{path: paths.Log}
		if err == nil {
			lg.logf("installed %s: %d policy values written, %d waiting for a sign-in, %d errors", version, res.Written, res.Pending, len(res.Errors))
			fmt.Fprintf(stdout, "Installed. %d policy values written, %d waiting for the child's first sign-in.\n", res.Written, res.Pending)
			if len(data.Missing) > 0 {
				fmt.Fprintf(stdout, "Built without: %s\n", strings.Join(data.Missing, ", "))
			}
		}
		return codeFor(err, stderr)
	case "uninstall":
		if !needAdmin() {
			return exitNotAdmin
		}
		p, err := readPin(fs, pin, pinEnv)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsage
		}
		res, err := agent.Uninstall(sys, svcs, data, paths, p, nil)
		if err == nil {
			fmt.Fprintf(stdout, "Removed. %d policy values removed or put back.\n", res.Removed)
		}
		return codeFor(err, stderr)
	case "verify-pin":
		if !needAdmin() {
			return exitNotAdmin
		}
		p, err := readPin(fs, pin, pinEnv)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitWrongPin
		}
		return codeFor(agent.VerifyPin(paths, p, nil), stderr)
	case "apply-once":
		if !needAdmin() {
			return exitNotAdmin
		}
		a, err := agent.Load(sys, data, paths, nil)
		if err != nil {
			return codeFor(err, stderr)
		}
		r := a.Apply(true)
		fmt.Fprintf(stdout, "%d written, %d unchanged, %d removed, %d waiting for a sign-in, %d errors\n", r.Written, r.Unchanged, r.Removed, r.Pending, len(r.Errors))
		if len(r.Errors) > 0 {
			return exitError
		}
		return exitOK
	case "status":
		if !needAdmin() {
			return exitNotAdmin
		}
		b, _ := json.MarshalIndent(agent.GetStatus(sys, svcs, data, paths, nil), "", "  ")
		fmt.Fprintln(stdout, string(b))
		return exitOK
	case "list-accounts":
		list, err := sys.LocalAccounts()
		if err != nil {
			return codeFor(err, stderr)
		}
		if *ini != "" {
			// On an update the installer ticks the accounts already protected.
			var cfg core.Config
			protected, mode := map[string]bool{}, ""
			if core.LoadJSON(paths.Config, &cfg) == nil {
				mode = cfg.Mode
				for _, a := range cfg.Accounts {
					protected[strings.ToUpper(a.SID)] = true
				}
			}
			if err := writeAccountsINI(*ini, list, protected, mode); err != nil {
				return codeFor(err, stderr)
			}
			return exitOK
		}
		for _, a := range list {
			kind := "standard"
			if a.Admin {
				kind = "administrator"
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", a.SID, kind, a.Name)
		}
		return exitOK
	}
	fmt.Fprint(stderr, usage)
	return exitUsage
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// fileURL turns C:\Program Files\RaiseSteadyGuard\raisesteady-guard.xpi into a file:/// URL.
func fileURL(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	u := url.URL{Scheme: "file", Path: "/" + p}
	return u.String()
}

// writeAccountsINI writes the accounts as a UTF-16 INI file for NSIS:
// [accounts] count=N installed=0|1 mode=users|all, then [a1] sid= name=
// admin=0|1 protected=0|1 (protected: in the current installation).
func writeAccountsINI(path string, list []accountLine, protected map[string]bool, mode string) error {
	var b strings.Builder
	installed := 0
	if mode != "" {
		installed = 1
	}
	fmt.Fprintf(&b, "[accounts]\r\ncount=%d\r\ninstalled=%d\r\nmode=%s\r\n", len(list), installed, mode)
	for i, a := range list {
		admin, prot := 0, 0
		if a.Admin {
			admin = 1
		}
		if protected[strings.ToUpper(a.SID)] {
			prot = 1
		}
		name := strings.NewReplacer("\r", " ", "\n", " ", "[", "(", "]", ")").Replace(a.Name)
		fmt.Fprintf(&b, "[a%d]\r\nsid=%s\r\nname=%s\r\nadmin=%d\r\nprotected=%d\r\n", i+1, a.SID, name, admin, prot)
	}
	u := utf16.Encode([]rune(b.String()))
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xFE})
	_ = binary.Write(&buf, binary.LittleEndian, u)
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

// serve is the service body: the loop and the localhost server.
func serve(ctx context.Context, logon <-chan string, sys agent.System, data *core.Data, paths agent.Paths, lg *logger) error {
	a, err := agent.Load(sys, data, paths, nil)
	if err != nil {
		lg.logf("cannot start: %v", err)
		return err
	}
	a.Log = lg.logf
	cfg := a.Config()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Port))
	if err != nil {
		lg.logf("cannot listen on 127.0.0.1:%d: %v", cfg.Port, err)
		return err
	}
	srv := &http.Server{
		Handler:           a.Server().Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			lg.logf("server: %v", err)
		}
	}()
	lg.logf("%s %s listening on 127.0.0.1:%d", core.AppName, version, cfg.Port)
	a.Run(ctx, logon)
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	lg.logf("stopped")
	return nil
}
