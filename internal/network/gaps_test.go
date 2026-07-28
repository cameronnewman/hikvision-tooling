package network

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func swap[T any](t *testing.T, target *T, replacement T) {
	t.Helper()
	orig := *target
	*target = replacement
	t.Cleanup(func() { *target = orig })
}

func TestGetARPTable_CmdNil(t *testing.T) {
	swap(t, &arpCommand, func() *exec.Cmd { return nil })
	table, err := GetARPTable()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(table) != 0 {
		t.Errorf("table = %v, want empty", table)
	}
}

func TestGetARPTable_CmdError(t *testing.T) {
	swap(t, &arpCommand, func() *exec.Cmd { return exec.Command("false") })
	table, err := GetARPTable()
	if err == nil {
		t.Fatal("expected error")
	}
	if table != nil {
		t.Errorf("table = %v, want nil", table)
	}
}

func TestIsValidMAC_PartOfWrongLength(t *testing.T) {
	if IsValidMAC("aaa:bb:cc:dd:ee:ff") {
		t.Error("expected false for MAC with 3-char part")
	}
}

func TestPingHost_InvalidIP(t *testing.T) {
	if PingHost("not-an-ip", 10*time.Millisecond) {
		t.Error("expected false for invalid IP")
	}
}

func TestPingHost_CmdNil(t *testing.T) {
	swap(t, &pingCommand, func(string) *exec.Cmd { return nil })
	if PingHost("127.0.0.1", 10*time.Millisecond) {
		t.Error("expected false when pingCommand returns nil")
	}
}

func TestPingHost_StartFails(t *testing.T) {
	swap(t, &pingCommand, func(string) *exec.Cmd {
		return exec.Command("/definitely/does/not/exist/binary")
	})
	if PingHost("127.0.0.1", 10*time.Millisecond) {
		t.Error("expected false when Start fails")
	}
}

func TestPingHost_SuccessBeforeTimeout(t *testing.T) {
	swap(t, &pingCommand, func(string) *exec.Cmd { return exec.Command("true") })
	if !PingHost("127.0.0.1", time.Second) {
		t.Error("expected true when command exits 0 before timeout")
	}
}

func TestPingHost_NonZeroBeforeTimeout(t *testing.T) {
	swap(t, &pingCommand, func(string) *exec.Cmd { return exec.Command("false") })
	if PingHost("127.0.0.1", time.Second) {
		t.Error("expected false when command exits non-zero")
	}
}

func TestGetWithAuth_HostWithoutPortUsesDefault(t *testing.T) {
	// Redirect dialTimeout so tests never hit the network but still exercise
	// the "port defaults to 80" branch.
	var gotAddr string
	swap(t, &dialTimeout, func(_, addr string, _ time.Duration) (net.Conn, error) {
		gotAddr = addr
		return nil, fmt.Errorf("stub-refused")
	})

	client := NewHTTPClient("t", 100*time.Millisecond)
	_, err := client.Get("127.0.0.1", "/")
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.HasSuffix(gotAddr, ":80") {
		t.Errorf("dial addr = %q, want default port 80 suffix", gotAddr)
	}
}

func TestGetWithAuth_ConnectionRefusedNonTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen err = %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	client := NewHTTPClient("t", 5*time.Second)
	_, err = client.Get(addr, "/")
	if err == nil {
		t.Fatal("expected connection failure")
	}
	if !strings.Contains(err.Error(), "connection failed to") {
		t.Errorf("err = %v, want 'connection failed to'", err)
	}
}

type writeFailConn struct {
	net.Conn
	writeErr error
}

func (w *writeFailConn) Write(_ []byte) (int, error)   { return 0, w.writeErr }
func (w *writeFailConn) Read(_ []byte) (int, error)    { return 0, io.EOF }
func (w *writeFailConn) Close() error                  { return nil }
func (w *writeFailConn) SetDeadline(_ time.Time) error { return nil }
func (w *writeFailConn) LocalAddr() net.Addr           { return &net.IPAddr{} }
func (w *writeFailConn) RemoteAddr() net.Addr          { return &net.IPAddr{} }
func (w *writeFailConn) SetReadDeadline(t time.Time) error {
	return w.SetDeadline(t)
}
func (w *writeFailConn) SetWriteDeadline(t time.Time) error {
	return w.SetDeadline(t)
}

func TestGetWithAuth_WriteFail(t *testing.T) {
	swap(t, &dialTimeout, func(string, string, time.Duration) (net.Conn, error) {
		return &writeFailConn{writeErr: fmt.Errorf("write busted")}, nil
	})

	client := NewHTTPClient("t", 100*time.Millisecond)
	_, err := client.Get("127.0.0.1:12345", "/")
	if err == nil || !strings.Contains(err.Error(), "failed to send request") {
		t.Fatalf("err = %v, want 'failed to send request'", err)
	}
}

func TestParseHTTPResponse_InvalidStatusCode(t *testing.T) {
	data := []byte("HTTP/1.1 abc OK\r\n\r\n")
	_, err := parseHTTPResponse(data)
	if err == nil || !strings.Contains(err.Error(), "invalid status code") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseHTTPResponse_EmptyHeaderBlock(t *testing.T) {
	data := []byte("\r\n\r\nbody")
	_, err := parseHTTPResponse(data)
	if err == nil || !strings.Contains(err.Error(), "invalid HTTP status line") {
		t.Fatalf("err = %v", err)
	}
}

func TestGetARPTable_ParsesOutput(t *testing.T) {
	fakeOutput := "? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]\n"
	swap(t, &arpCommand, func() *exec.Cmd {
		cmd := exec.Command("sh", "-c", "printf %s "+shellQuote(fakeOutput))
		return cmd
	})
	table, err := GetARPTable()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if table["192.168.1.1"] != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("table = %v, want 192.168.1.1", table)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func TestGetWithAuth_ExtraSmokeCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.Copy(w, bytes.NewReader([]byte("hi")))
	}))
	t.Cleanup(server.Close)

	addr := strings.TrimPrefix(server.URL, "http://")
	client := NewHTTPClient("t", 5*time.Second)
	resp, err := client.GetWithAuth(addr, "/x", "tok")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want 418", resp.StatusCode)
	}
}

func TestScanARP_SkipsMalformed(t *testing.T) {
	data := "garbage line\n? (192.168.1.5) at 11:22:33:44:55:66 on en0\n"
	scanner := bufio.NewScanner(bytes.NewReader([]byte(data)))
	table := ARPTable{}
	for scanner.Scan() {
		ip, mac := ParseARPLine(scanner.Text())
		if ip != "" && mac != "" {
			table[ip] = mac
		}
	}
	if table["192.168.1.5"] != "11:22:33:44:55:66" {
		t.Errorf("table = %v", table)
	}
}
