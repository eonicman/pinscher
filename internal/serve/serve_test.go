package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eonicman/pinscher/internal/config"
)

// startTestServer spins up a real daemon on a temp socket and wires cleanup.
func startTestServer(t *testing.T, allowWrite bool) *Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pinscher-test.sock")
	srv, err := Start(path, allowWrite)
	if err != nil {
		t.Fatalf("Start(%s): %v", path, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := srv.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		<-done
	})
	waitDialable(t, path)
	return srv
}

func waitDialable(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never became dialable at %s: %v", path, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// rpc sends one NDJSON request line and returns the decoded response.
func rpc(t *testing.T, path, line string) map[string]any {
	t.Helper()
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", path, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := fmt.Fprintln(conn, line); err != nil {
		t.Fatalf("write: %v", err)
	}
	respLine, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(respLine)), &resp); err != nil {
		t.Fatalf("unmarshal %q: %v", respLine, err)
	}
	return resp
}

func TestPing(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":1,"method":"ping"}`)
	if resp["ok"] != true {
		t.Fatalf("ping not ok: %v", resp)
	}
	if resp["id"] != float64(1) {
		t.Errorf("id not echoed: %v", resp["id"])
	}
	result, _ := resp["result"].(map[string]any)
	if result == nil || result["pong"] != true {
		t.Fatalf("bad ping result: %v", resp["result"])
	}
	if result["version"] != Version {
		t.Errorf("version mismatch: got %v, want %v", result["version"], Version)
	}
	if result["allow_write"] != false {
		t.Errorf("allow_write should be false: %v", result["allow_write"])
	}
}

func TestVersion(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":"abc","method":"version"}`)
	if resp["ok"] != true {
		t.Fatalf("version not ok: %v", resp)
	}
	// Non-numeric IDs must survive the round trip.
	if resp["id"] != "abc" {
		t.Errorf("string id not echoed: %v", resp["id"])
	}
	result, _ := resp["result"].(map[string]any)
	if result["version"] != Version {
		t.Errorf("version mismatch: %v", result["version"])
	}
}

func TestQuirksAll(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":2,"method":"quirks"}`)
	if resp["ok"] != true {
		t.Fatalf("quirks not ok: %v", resp)
	}
	quirks, _ := resp["result"].([]any)
	if len(quirks) == 0 {
		t.Fatalf("quirk database returned empty: %v", resp)
	}
}

func TestQuirksByModule(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":3,"method":"quirks","params":{"module":"amdgpu"}}`)
	if resp["ok"] != true {
		t.Fatalf("quirks(amdgpu) not ok: %v", resp)
	}
	quirks, _ := resp["result"].([]any)
	if len(quirks) == 0 {
		t.Fatalf("expected amdgpu quirks, got none: %v", resp)
	}
}

func TestQuirksByDevice(t *testing.T) {
	srv := startTestServer(t, false)
	// Tahiti XT / FirePro D700 — present in the quirk database since v0.3.
	resp := rpc(t, srv.SocketPath(), `{"id":4,"method":"quirks","params":{"device":"1002:6799"}}`)
	if resp["ok"] != true {
		t.Fatalf("quirks(1002:6799) not ok: %v", resp)
	}
	quirks, _ := resp["result"].([]any)
	if len(quirks) == 0 {
		t.Fatalf("expected quirks for 1002:6799, got none: %v", resp)
	}
}

func TestQuirksBadDeviceFormat(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":5,"method":"quirks","params":{"device":"justastring"}}`)
	if resp["ok"] != false {
		t.Fatalf("expected error for malformed device id: %v", resp)
	}
	if !strings.Contains(resp["error"].(string), "vendor:device") {
		t.Errorf("error should mention vendor:device format: %v", resp["error"])
	}
}

func TestProfilesAndProfileLookup(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":6,"method":"profiles"}`)
	if resp["ok"] != true {
		t.Fatalf("profiles not ok: %v", resp)
	}
	result, _ := resp["result"].(map[string]any)
	names, _ := result["builtins"].([]any)
	if len(names) == 0 {
		t.Fatalf("no builtin profiles listed: %v", resp)
	}

	// Fetch the first listed profile by name.
	first, _ := names[0].(string)
	resp = rpc(t, srv.SocketPath(), fmt.Sprintf(`{"id":7,"method":"profile","params":{"name":%q}}`, first))
	if resp["ok"] != true {
		t.Fatalf("profile(%s) not ok: %v", first, resp)
	}
	p, _ := resp["result"].(map[string]any)
	if p["name"] != first {
		t.Errorf("profile name mismatch: got %v want %v", p["name"], first)
	}
}

func TestUnknownMethod(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":8,"method":"frobnicate"}`)
	if resp["ok"] != false {
		t.Fatalf("unknown method should fail: %v", resp)
	}
	if !strings.Contains(resp["error"].(string), "unknown method") {
		t.Errorf("error should say 'unknown method': %v", resp["error"])
	}
	if !strings.Contains(resp["error"].(string), "ping") {
		t.Errorf("error should advertise supported methods: %v", resp["error"])
	}
}

func TestMalformedJSON(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `this is not json`)
	if resp["ok"] != false {
		t.Fatalf("malformed JSON should fail: %v", resp)
	}
	if !strings.Contains(resp["error"].(string), "invalid request JSON") {
		t.Errorf("error should mention invalid JSON: %v", resp["error"])
	}
}

func TestMissingMethod(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":9}`)
	if resp["ok"] != false {
		t.Fatalf("missing method should fail: %v", resp)
	}
	if !strings.Contains(resp["error"].(string), "missing method") {
		t.Errorf("error should mention missing method: %v", resp["error"])
	}
}

func TestDiagnoseDeviceNotFound(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":10,"method":"diagnose","params":{"device":"totally-fake-driver-xyz"}}`)
	if resp["ok"] != false {
		t.Fatalf("diagnose of nonexistent device should fail: %v", resp)
	}
	if resp["error"].(string) == "" {
		t.Errorf("error should be non-empty: %v", resp)
	}
}

func TestDiagnoseMissingParam(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":11,"method":"diagnose"}`)
	if resp["ok"] != false {
		t.Fatalf("diagnose without device param should fail: %v", resp)
	}
}

func TestSetRefusedWithoutAllowWrite(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":12,"method":"set","params":{"module":"amdgpu","param":"dc","value":"1"}}`)
	if resp["ok"] != false {
		t.Fatalf("set must be refused on read-only server: %v", resp)
	}
	if !strings.Contains(resp["error"].(string), "--allow-write") {
		t.Errorf("error should mention --allow-write: %v", resp["error"])
	}
}

func TestParamStringNumericFidelity(t *testing.T) {
	// Mirror the exact wire path: decode with UseNumber (as handleRequest
	// does) so numeric params arrive as json.Number, then verify the value
	// keeps its exact source text instead of decaying to scientific
	// notation (120000 must stay "120000", not become "1.2e+05").
	dec := json.NewDecoder(strings.NewReader(`{"id":1,"method":"set","params":{"module":"nvme","param":"default_ps_max_latency_us","value":120000}}`))
	dec.UseNumber()
	var req Request
	if err := dec.Decode(&req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := paramStringOK(req.Params, "value")
	if !ok {
		t.Fatal("value param not found")
	}
	if got != "120000" {
		t.Fatalf("numeric fidelity broken: got %q, want %q", got, "120000")
	}
}

func TestSetNumericValueDryRun(t *testing.T) {
	srv := startTestServer(t, true)
	if _, err := config.GetParam("amdgpu", "si_support"); err != nil {
		t.Skip("amdgpu module not present on this machine")
	}
	// dry_run=true exercises the full set path (safety check, value read,
	// audit log) without writing anything to the running system.
	resp := rpc(t, srv.SocketPath(), `{"id":13,"method":"set","params":{"module":"amdgpu","param":"si_support","value":120000,"dry_run":true}}`)
	if resp["ok"] != true {
		t.Fatalf("dry-run set should succeed: %v", resp)
	}
	result, _ := resp["result"].(map[string]any)
	if result["new_value"] != "120000" {
		t.Errorf("numeric value decayed in set path: %v", result["new_value"])
	}
	// And the dry run must be in the audit log.
	resp = rpc(t, srv.SocketPath(), `{"id":14,"method":"audit-log"}`)
	if resp["ok"] != true {
		t.Fatalf("audit-log not ok: %v", resp)
	}
	entries, _ := resp["result"].([]any)
	if len(entries) == 0 {
		t.Errorf("dry-run set should appear in audit log")
	}
}

func TestGetUnknownModule(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":14,"method":"get","params":{"module":"definitely-no-such-module","param":"x"}}`)
	if resp["ok"] != false {
		t.Fatalf("get on unknown module should fail: %v", resp)
	}
}

func TestListParamsMissingModule(t *testing.T) {
	srv := startTestServer(t, false)
	resp := rpc(t, srv.SocketPath(), `{"id":15,"method":"list-params"}`)
	if resp["ok"] != false {
		t.Fatalf("list-params without module should fail: %v", resp)
	}
}

func TestConcurrentRequests(t *testing.T) {
	srv := startTestServer(t, false)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			resp := rpc(t, srv.SocketPath(), fmt.Sprintf(`{"id":%d,"method":"ping"}`, n))
			if resp["ok"] != true {
				errs <- fmt.Errorf("ping %d failed: %v", n, resp)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestMultipleRequestsOneConnection(t *testing.T) {
	srv := startTestServer(t, false)
	conn, err := net.DialTimeout("unix", srv.SocketPath(), time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	for i := 0; i < 3; i++ {
		if _, err := fmt.Fprintf(w, "{\"id\":%d,\"method\":\"version\"}\n", i); err != nil {
			t.Fatalf("write: %v", err)
		}
		w.Flush()
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["ok"] != true {
			t.Errorf("request %d failed: %v", i, resp)
		}
		if resp["id"] != float64(i) {
			t.Errorf("request %d: id mismatch: %v", i, resp["id"])
		}
	}
}

func TestStartRefusesSecondInstance(t *testing.T) {
	srv := startTestServer(t, false)
	if _, err := Start(srv.SocketPath(), false); err == nil {
		t.Fatal("second Start on live socket should fail")
	} else if !strings.Contains(err.Error(), "another pinscher serve") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartReclaimsStaleSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.sock")

	// Create a socket file with no listener behind it (close without remove).
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("setup listen: %v", err)
	}
	ln.Close() // leaves the filesystem socket node behind on Linux

	srv, err := Start(path, false)
	if err != nil {
		t.Fatalf("Start should reclaim stale socket: %v", err)
	}
	defer srv.Close()
	waitDialable(t, path)
}

func TestStartRefusesNonSocketFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "regular-file")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := Start(path, false); err == nil {
		t.Fatal("Start on a regular file should fail")
	}
}
