// Package serve implements the pinscher daemon: a persistent hardware
// intelligence layer that exposes the read-only hardware API over a local
// Unix socket using newline-delimited JSON (one request object per line,
// one response object per line).
//
// Wire protocol:
//
//	→ {"id": 1, "method": "diagnose", "params": {"device": "amdgpu"}}
//	← {"id": 1, "ok": true, "result": { ... }}
//	← {"id": 2, "ok": false, "error": "unknown method: frobnicate"}
//
// The daemon is read-only by default. The mutating "set" method is only
// available when the server was started with --allow-write, and the socket
// is always created with 0600 permissions so only the owning user can talk
// to it.
package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/eonicman/pinscher/internal/config"
	"github.com/eonicman/pinscher/internal/diagnose"
	"github.com/eonicman/pinscher/internal/lintboot"
	"github.com/eonicman/pinscher/internal/profile"
	"github.com/eonicman/pinscher/internal/probe"
	"github.com/eonicman/pinscher/internal/quirk"
)

// Version is set by the caller (cmd/pinscher) so responses report the
// binary's real version.
var Version = "dev"

// maxRequestBytes caps a single request line. report JSON on very large
// machines can be big, so allow a generous 4 MiB.
const maxRequestBytes = 4 << 20

// methods is advertised in unknown-method errors so API clients (AI agents
// especially) can self-correct.
var methods = []string{
	"ping", "version", "report", "lint-boot", "diagnose", "quirks",
	"profile", "profiles", "recommend", "list-params", "get", "audit-log", "set",
}

// Request is one JSON request line.
type Request struct {
	ID     json.RawMessage `json:"id"`     // echoed back verbatim (number, string, null…)
	Method string          `json:"method"` // required
	Params map[string]any  `json:"params"` // optional
}

// Response is one JSON response line.
type Response struct {
	ID     json.RawMessage `json:"id"`
	OK     bool            `json:"ok"`
	Result any             `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Server is a listening pinscher daemon.
type Server struct {
	path       string
	allowWrite bool
	started    time.Time
	ln         net.Listener
	writeMu    sync.Mutex // serializes config mutations
}

// DefaultSocketPath returns the conventional socket location:
// /run/pinscher.sock for root, $XDG_RUNTIME_DIR/pinscher.sock for a
// logged-in user, otherwise a per-UID path in the system temp dir.
func DefaultSocketPath() string {
	if os.Geteuid() == 0 {
		return "/run/pinscher.sock"
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "pinscher.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("pinscher-%d.sock", os.Getuid()))
}

// Start binds the Unix socket and returns a ready Server. It refuses to
// start if another live instance is listening, and cleans up a stale
// socket file left behind by an unclean shutdown.
func Start(path string, allowWrite bool) (*Server, error) {
	if path == "" {
		path = DefaultSocketPath()
	}
	if fi, err := os.Stat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		// Socket file exists — is somebody live behind it?
		if conn, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
			conn.Close()
			return nil, fmt.Errorf("another pinscher serve instance is listening on %s", path)
		}
		// Stale socket from an unclean shutdown — safe to reclaim.
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("cannot remove stale socket %s: %w", path, err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	// Owner-only access, even if the process runs as root.
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod %s: %w", path, err)
	}
	return &Server{path: path, allowWrite: allowWrite, started: time.Now(), ln: ln}, nil
}

// SocketPath returns the bound socket path.
func (s *Server) SocketPath() string { return s.path }

// AllowWrite reports whether mutating methods are enabled.
func (s *Server) AllowWrite() bool { return s.allowWrite }

// Serve accepts connections until ctx is cancelled. Each connection is
// handled on its own goroutine; requests on a connection are processed in
// order.
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.ln.Close()
	}()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.handleConn(conn)
	}
}

// Close stops the listener and removes the socket file. It is safe to call
// after Serve has already closed the listener on context cancellation.
func (s *Server) Close() error {
	err := s.ln.Close()
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	if rmErr := os.Remove(s.path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) && err == nil {
		err = rmErr
	}
	return err
}

// Run is the CLI entry point: installs signal handling, optionally applies a
// duration cap, runs the accept loop, and always cleans up the socket file.
func Run(path string, allowWrite bool, duration time.Duration, quiet bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}
	srv, err := Start(path, allowWrite)
	if err != nil {
		return err
	}
	defer srv.Close()
	if !quiet {
		mode := "read-only"
		if allowWrite {
			mode = "read-write"
		}
		fmt.Fprintf(os.Stderr, "pinscher serve %s listening on %s (%s)\n", Version, srv.SocketPath(), mode)
	}
	return srv.Serve(ctx)
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), maxRequestBytes)
	w := bufio.NewWriter(conn)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		resp := s.handleRequest(line)
		out, err := json.Marshal(resp)
		if err != nil {
			out, _ = json.Marshal(Response{ID: resp.ID, OK: false, Error: "response marshal failure: " + err.Error()})
		}
		if _, err := w.Write(out); err != nil {
			return
		}
		w.WriteByte('\n')
		if err := w.Flush(); err != nil {
			return
		}
	}
}

// handleRequest parses and dispatches a single request line, converting
// panics into protocol errors so one bad request can't kill the daemon.
func (s *Server) handleRequest(line string) (resp Response) {
	var req Request
	// UseNumber keeps numeric params as their exact source text ("120000"
	// stays "120000" instead of decaying to 1.2e+05 via float64).
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("invalid request JSON: %v", err)}
	}
	resp.ID = req.ID
	defer func() {
		if r := recover(); r != nil {
			resp = Response{ID: req.ID, OK: false, Error: fmt.Sprintf("internal error: %v", r)}
		}
	}()
	if req.Method == "" {
		return Response{ID: req.ID, OK: false, Error: "missing method"}
	}
	result, err := s.dispatch(req.Method, req.Params)
	if err != nil {
		return Response{ID: req.ID, OK: false, Error: err.Error()}
	}
	return Response{ID: req.ID, OK: true, Result: result}
}

func (s *Server) dispatch(method string, params map[string]any) (any, error) {
	switch method {
	case "ping":
		return map[string]any{
			"pong":           true,
			"version":        Version,
			"pid":            os.Getpid(),
			"uptime_seconds": int(time.Since(s.started).Seconds()),
			"allow_write":    s.allowWrite,
			"socket":         s.path,
		}, nil

	case "version":
		return map[string]any{"version": Version}, nil

	case "report":
		return probe.Probe()

	case "lint-boot":
		return lintboot.LintBoot(paramString(params, "initramfs"))

	case "diagnose":
		dev, ok := paramStringOK(params, "device")
		if !ok {
			return nil, errors.New(`diagnose requires params: {"device": "<pci address|vendor:device|driver|description>"}`)
		}
		return diagnose.Diagnose(dev)

	case "quirks":
		if m, ok := paramStringOK(params, "module"); ok {
			return quirk.FindByModule(m), nil
		}
		if d, ok := paramStringOK(params, "device"); ok {
			parts := strings.SplitN(d, ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return nil, fmt.Errorf(`quirks device param must be "vendor:device" (e.g. "1002:6799"), got %q`, d)
			}
			return quirk.FindByDevice(parts[0], parts[1]), nil
		}
		return quirk.FindAll(), nil

	case "profile":
		name, ok := paramStringOK(params, "name")
		if !ok {
			return nil, errors.New(`profile requires params: {"name": "<builtin name or file path>"}`)
		}
		return profile.LoadProfile(name)

	case "profiles":
		return map[string]any{"builtins": profile.ListProfiles()}, nil

	case "recommend":
		pr, err := probe.Probe()
		if err != nil {
			return nil, fmt.Errorf("probe failed: %w", err)
		}
		return profile.RecommendProfiles(pr), nil

	case "list-params":
		module, ok := paramStringOK(params, "module")
		if !ok {
			return nil, errors.New(`list-params requires params: {"module": "<module name>"}`)
		}
		list, err := config.ListParams(module)
		if err != nil {
			return nil, err
		}
		return map[string]any{"module": module, "params": list}, nil

	case "get":
		module, ok := paramStringOK(params, "module")
		if !ok {
			return nil, errors.New(`get requires params: {"module": "<module>", "param": "<param>"}`)
		}
		param, ok := paramStringOK(params, "param")
		if !ok {
			return nil, errors.New(`get requires params: {"module": "<module>", "param": "<param>"}`)
		}
		value, err := config.GetParam(module, param)
		if err != nil {
			return nil, err
		}
		return map[string]any{"module": module, "param": param, "value": value}, nil

	case "audit-log":
		return config.GetAuditLog(), nil

	case "set":
		if !s.allowWrite {
			return nil, errors.New("write disabled: server was started without --allow-write")
		}
		module, ok := paramStringOK(params, "module")
		if !ok {
			return nil, errors.New(`set requires params: {"module": "<module>", "param": "<param>", "value": "<value>", "dry_run": false}`)
		}
		param, ok := paramStringOK(params, "param")
		if !ok {
			return nil, errors.New(`set requires params: {"module": "<module>", "param": "<param>", "value": "<value>", "dry_run": false}`)
		}
		value, ok := paramStringOK(params, "value")
		if !ok {
			return nil, errors.New(`set requires params: {"module": "<module>", "param": "<param>", "value": "<value>", "dry_run": false}`)
		}
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		return config.SetParam(module, param, value, paramBool(params, "dry_run"))

	default:
		return nil, fmt.Errorf("unknown method: %s (supported: %s)", method, strings.Join(methods, ", "))
	}
}

// paramStringOK fetches a required string-ish param, coercing numbers and
// booleans (module param values arrive as float64 from encoding/json).
func paramStringOK(params map[string]any, key string) (string, bool) {
	if params == nil {
		return "", false
	}
	v, ok := params[key]
	if !ok || v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case json.Number:
		return t.String(), true
	case float64:
		// Only reachable when a Request was built in-process (tests/lib use);
		// wire requests carry json.Number thanks to UseNumber.
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case bool:
		return fmt.Sprintf("%t", t), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

func paramString(params map[string]any, key string) string {
	v, _ := paramStringOK(params, key)
	return v
}

func paramBool(params map[string]any, key string) bool {
	if params == nil {
		return false
	}
	v, ok := params[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}