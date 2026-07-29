package cli

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cameronnewman/hikvision-tooling/internal/isapi"
)

// fakeISAPI is a stand-in for *isapi.Client that captures constructor args and
// returns preconfigured DeviceInfo/error values.
type fakeISAPI struct {
	info *isapi.DeviceInfo
	err  error
}

func (f *fakeISAPI) DeviceInfo(_ context.Context) (*isapi.DeviceInfo, error) {
	return f.info, f.err
}

type isapiCall struct {
	baseURL string
	opts    isapi.Options
}

// withISAPI swaps in a fake ISAPI client factory and captures each call so the
// test can assert on the base URL / credentials the CLI passed through.
func withISAPI(t *testing.T, fake isapiDeviceInfoFetcher, capture *isapiCall) {
	t.Helper()
	orig := newISAPIClient
	newISAPIClient = func(baseURL string, opts isapi.Options) isapiDeviceInfoFetcher {
		if capture != nil {
			capture.baseURL = baseURL
			capture.opts = opts
		}
		return fake
	}
	t.Cleanup(func() { newISAPIClient = orig })
}

// timeoutErr is a net.Error whose Timeout() returns true; used to exercise the
// generic network-timeout branch in mapISAPIError.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

var _ net.Error = timeoutErr{}

func populatedDeviceInfo() *isapi.DeviceInfo {
	return &isapi.DeviceInfo{
		XMLName:              xml.Name{Local: "DeviceInfo"},
		DeviceName:           "Front Door Cam",
		DeviceID:             "abcd-1234",
		Model:                "DS-2CD2143G0-I",
		SerialNumber:         "SN0123456789",
		MacAddress:           "AA:BB:CC:DD:EE:FF",
		FirmwareVersion:      "V5.5.82 build 190909",
		FirmwareReleasedDate: "2019-09-09",
		EncoderVersion:       "V7.3 build 190826",
		DeviceType:           "IPCamera",
		DeviceDescription:    "Hikvision IP Camera",
	}
}

func TestISAPICmd_HelpNoArgs(t *testing.T) {
	buf := withStdout(t)
	if err := ISAPICmd(nil); err != nil {
		t.Fatalf("ISAPICmd(nil) err = %v", err)
	}
	if !strings.Contains(buf.String(), "sadp isapi") {
		t.Errorf("stdout missing usage: %s", buf.String())
	}
}

func TestISAPICmd_HelpFlags(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			buf := withStdout(t)
			if err := ISAPICmd([]string{arg}); err != nil {
				t.Fatalf("err = %v", err)
			}
			if !strings.Contains(buf.String(), "Subcommands:") {
				t.Errorf("stdout missing Subcommands: %s", buf.String())
			}
		})
	}
}

func TestISAPICmd_InfoNoHostPrintsUsage(t *testing.T) {
	buf := withStdout(t)
	if err := ISAPICmd([]string{"info"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "Usage: sadp isapi info") {
		t.Errorf("stdout missing usage: %s", buf.String())
	}
}

func TestISAPICmd_UnknownSubcommand(t *testing.T) {
	withStdout(t)
	err := ISAPICmd([]string{"bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("err = %v, want 'unknown' error", err)
	}
}

func TestISAPIInfoCmd_HostNormalization(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantURL string
	}{
		{"bare IP", "192.168.1.64", "http://192.168.1.64"},
		{"http prefix", "http://192.168.1.64", "http://192.168.1.64"},
		{"https with port", "https://192.168.1.64:8443", "https://192.168.1.64:8443"},
		{"hostname", "camera.local", "http://camera.local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStdout(t)
			t.Setenv("HIKVISION_USERNAME", "testuser")
			t.Setenv("HIKVISION_PASSWORD", "testpass")

			var call isapiCall
			withISAPI(t, &fakeISAPI{info: populatedDeviceInfo()}, &call)

			if err := isapiInfoCmd([]string{tt.input}); err != nil {
				t.Fatalf("err = %v", err)
			}
			if call.baseURL != tt.wantURL {
				t.Errorf("baseURL = %q, want %q", call.baseURL, tt.wantURL)
			}
		})
	}
}

func TestISAPIInfoCmd_PrettyOutput(t *testing.T) {
	buf := withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "testuser")
	t.Setenv("HIKVISION_PASSWORD", "testpass")
	withISAPI(t, &fakeISAPI{info: populatedDeviceInfo()}, nil)

	if err := isapiInfoCmd([]string{"192.168.1.64"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{
		"Device Name:",
		"Front Door Cam",
		"Model:",
		"DS-2CD2143G0-I",
		"Serial Number:",
		"MAC Address:",
		"Firmware Version:",
		"Firmware Date:",
		"Device Type:",
		"Description:",
		"Encoder Version:",
		"Device ID:",
	}
	got := buf.String()
	for _, s := range want {
		if !strings.Contains(got, s) {
			t.Errorf("stdout missing %q\nfull output:\n%s", s, got)
		}
	}
}

func TestISAPIInfoCmd_JSONOutput(t *testing.T) {
	buf := withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "testuser")
	t.Setenv("HIKVISION_PASSWORD", "testpass")
	want := populatedDeviceInfo()
	withISAPI(t, &fakeISAPI{info: want}, nil)

	if err := isapiInfoCmd([]string{"--json", "192.168.1.64"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	var got isapi.DeviceInfo
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, buf.String())
	}
	// Reset XMLName since json.Unmarshal won't populate it.
	got.XMLName = want.XMLName
	if got != *want {
		t.Errorf("round-tripped DeviceInfo mismatch:\ngot  %+v\nwant %+v", got, *want)
	}
}

func TestISAPIInfoCmd_EmptyFieldsSkipped(t *testing.T) {
	buf := withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "testuser")
	t.Setenv("HIKVISION_PASSWORD", "testpass")
	// Only Model set; every other field is empty and should be omitted.
	info := &isapi.DeviceInfo{Model: "DS-Only"}
	withISAPI(t, &fakeISAPI{info: info}, nil)

	if err := isapiInfoCmd([]string{"192.168.1.64"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "Model:") || !strings.Contains(got, "DS-Only") {
		t.Errorf("stdout missing Model line: %s", got)
	}
	for _, s := range []string{"Device Name:", "Serial Number:", "MAC Address:", "Description:"} {
		if strings.Contains(got, s) {
			t.Errorf("stdout unexpectedly contains %q for empty field: %s", s, got)
		}
	}
}

func TestISAPIInfoCmd_MissingCredentials(t *testing.T) {
	withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "")
	t.Setenv("HIKVISION_PASSWORD", "")
	// Fake never gets called, but wire it anyway to avoid a real http.Client.
	withISAPI(t, &fakeISAPI{info: populatedDeviceInfo()}, nil)

	err := isapiInfoCmd([]string{"192.168.1.64"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exitErr.Code != 2 {
		t.Errorf("code = %d, want 2", exitErr.Code)
	}
}

func TestISAPIInfoCmd_UnauthorizedExitsTwo(t *testing.T) {
	withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "testuser")
	t.Setenv("HIKVISION_PASSWORD", "testpass")
	withISAPI(t, &fakeISAPI{err: isapi.ErrUnauthorized}, nil)

	err := isapiInfoCmd([]string{"192.168.1.64"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exitErr.Code != 2 {
		t.Errorf("code = %d, want 2", exitErr.Code)
	}
	if !errors.Is(err, isapi.ErrUnauthorized) {
		t.Errorf("errors.Is(err, ErrUnauthorized) = false, want true")
	}
}

func TestISAPIInfoCmd_TimeoutExitsThree(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"context deadline", context.DeadlineExceeded},
		{"context canceled", context.Canceled},
		{"net timeout", timeoutErr{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStdout(t)
			t.Setenv("HIKVISION_USERNAME", "testuser")
			t.Setenv("HIKVISION_PASSWORD", "testpass")
			withISAPI(t, &fakeISAPI{err: tt.err}, nil)

			err := isapiInfoCmd([]string{"192.168.1.64"})
			var exitErr *ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("err = %v, want *ExitError", err)
			}
			if exitErr.Code != 3 {
				t.Errorf("code = %d, want 3", exitErr.Code)
			}
		})
	}
}

func TestISAPIInfoCmd_NotFound(t *testing.T) {
	withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "testuser")
	t.Setenv("HIKVISION_PASSWORD", "testpass")
	withISAPI(t, &fakeISAPI{err: isapi.ErrNotFound}, nil)

	err := isapiInfoCmd([]string{"192.168.1.64"})
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want plain error (code 1), got ExitError code %d", err, exitErr.Code)
	}
	if !strings.Contains(err.Error(), "/ISAPI/System/deviceInfo") {
		t.Errorf("err = %v, want message mentioning endpoint", err)
	}
	if !errors.Is(err, isapi.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false, want true")
	}
}

func TestISAPIInfoCmd_GenericError(t *testing.T) {
	withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "testuser")
	t.Setenv("HIKVISION_PASSWORD", "testpass")
	withISAPI(t, &fakeISAPI{err: errors.New("boom")}, nil)

	err := isapiInfoCmd([]string{"192.168.1.64"})
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want plain error (code 1), got ExitError code %d", err, exitErr.Code)
	}
}

func TestISAPIInfoCmd_FlagOverridesCredentials(t *testing.T) {
	withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "")
	t.Setenv("HIKVISION_PASSWORD", "")
	var call isapiCall
	withISAPI(t, &fakeISAPI{info: populatedDeviceInfo()}, &call)

	err := isapiInfoCmd([]string{
		"--username", "testuser",
		"--password", "testpass",
		"--insecure",
		"--timeout", "2s",
		"192.168.1.64",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if call.opts.Username != "testuser" || call.opts.Password != "testpass" {
		t.Errorf("creds not passed through: %+v", call.opts)
	}
	if !call.opts.InsecureSkipVerify {
		t.Errorf("InsecureSkipVerify = false, want true")
	}
	if call.opts.Timeout != 2*time.Second {
		t.Errorf("Timeout = %v, want 2s", call.opts.Timeout)
	}
}

func TestISAPIInfoCmd_InvalidHost(t *testing.T) {
	withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "testuser")
	t.Setenv("HIKVISION_PASSWORD", "testpass")
	withISAPI(t, &fakeISAPI{info: populatedDeviceInfo()}, nil)

	err := isapiInfoCmd([]string{"ftp://example.com"})
	if err == nil || !strings.Contains(err.Error(), "unsupported scheme") {
		t.Fatalf("err = %v, want unsupported scheme error", err)
	}
}

func TestISAPIInfoCmd_ConfigLoadError(t *testing.T) {
	withStdout(t)
	t.Setenv("HTTP_TIMEOUT", "nonsense")
	err := isapiInfoCmd([]string{"192.168.1.64"})
	if err == nil || !strings.Contains(err.Error(), "failed to load config") {
		t.Fatalf("err = %v, want config error", err)
	}
}

func TestRun_ISAPIDispatch(t *testing.T) {
	withStdout(t)
	t.Setenv("HIKVISION_USERNAME", "testuser")
	t.Setenv("HIKVISION_PASSWORD", "testpass")
	var call isapiCall
	withISAPI(t, &fakeISAPI{info: populatedDeviceInfo()}, &call)

	if err := Run([]string{"isapi", "info", "1.2.3.4"}); err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if call.baseURL != "http://1.2.3.4" {
		t.Errorf("baseURL = %q, want http://1.2.3.4", call.baseURL)
	}
}

func TestRun_ISAPIHelpOnly(t *testing.T) {
	withStdout(t)
	if err := Run([]string{"isapi"}); err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

func TestExitError_UnwrapAndErrorMessage(t *testing.T) {
	inner := errors.New("boom")
	e := &ExitError{Code: 7, Err: inner}
	if e.Error() != "boom" {
		t.Errorf("Error() = %q, want boom", e.Error())
	}
	if !errors.Is(e, inner) {
		t.Errorf("errors.Is(e, inner) = false")
	}
	var nilErr *ExitError
	if nilErr.Error() != "" {
		t.Errorf("nil ExitError Error() = %q, want empty", nilErr.Error())
	}
	empty := &ExitError{}
	if empty.Error() != "" {
		t.Errorf("empty ExitError Error() = %q, want empty", empty.Error())
	}
}

func TestNormalizeISAPIHost_Errors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"empty", "", "empty"},
		{"bad scheme", "gopher://x", "unsupported scheme"},
		{"missing host", "http://", "missing host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeISAPIHost(tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
