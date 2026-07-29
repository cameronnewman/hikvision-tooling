package isapi

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testUser = "testuser"
	testPass = "testpass"
)

const fullDeviceInfoXML = `<?xml version="1.0" encoding="UTF-8"?>
<DeviceInfo>
  <deviceName>Front Door Cam</deviceName>
  <deviceID>abc-123</deviceID>
  <model>DS-2CD2042WD-I</model>
  <serialNumber>SN0123456789</serialNumber>
  <macAddress>44:47:cc:11:22:33</macAddress>
  <firmwareVersion>V5.6.5</firmwareVersion>
  <firmwareReleasedDate>build 180316</firmwareReleasedDate>
  <encoderVersion>V7.3</encoderVersion>
  <deviceType>IPCamera</deviceType>
  <deviceDescription>IPC</deviceDescription>
</DeviceInfo>`

const partialDeviceInfoXML = `<?xml version="1.0" encoding="UTF-8"?>
<DeviceInfo>
  <deviceName>Partial Cam</deviceName>
  <model>DS-XYZ</model>
</DeviceInfo>`

// newTestClient wires a Client at the given URL with test credentials and a
// short timeout so tests never hang.
func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	return New(baseURL, Options{
		Username: testUser,
		Password: testPass,
		Timeout:  5 * time.Second,
	})
}

func TestNewNormalisesBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no trailing slash", in: "http://192.0.2.1", want: "http://192.0.2.1"},
		{name: "single trailing slash", in: "http://192.0.2.1/", want: "http://192.0.2.1"},
		{name: "multiple trailing slashes", in: "http://192.0.2.1///", want: "http://192.0.2.1"},
		{name: "https preserved", in: "https://192.0.2.1:443/", want: "https://192.0.2.1:443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New(tt.in, Options{})
			if c.BaseURL != tt.want {
				t.Errorf("BaseURL = %q, want %q", c.BaseURL, tt.want)
			}
			if c.HTTPClient == nil {
				t.Fatal("HTTPClient is nil")
			}
		})
	}
}

func TestNewAppliesTimeout(t *testing.T) {
	t.Parallel()

	c := New("http://x", Options{Timeout: 250 * time.Millisecond})
	if c.HTTPClient.Timeout != 250*time.Millisecond {
		t.Errorf("Timeout = %v, want 250ms", c.HTTPClient.Timeout)
	}
}

func TestGetPrependsSlash(t *testing.T) {
	t.Parallel()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	resp, err := c.Get(context.Background(), "no-slash")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()
	if gotPath != "/no-slash" {
		t.Errorf("path = %q, want %q", gotPath, "/no-slash")
	}
}

func TestGetBuildRequestError(t *testing.T) {
	t.Parallel()

	// Control character in BaseURL causes http.NewRequestWithContext to fail.
	c := New("http://exa\x7fmple", Options{})
	resp, err := c.Get(context.Background(), "/x")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("err = nil, want request build error")
	}
}

func TestDeviceInfoParsesXML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want DeviceInfo
	}{
		{
			name: "all fields populated",
			body: fullDeviceInfoXML,
			want: DeviceInfo{
				DeviceName:           "Front Door Cam",
				DeviceID:             "abc-123",
				Model:                "DS-2CD2042WD-I",
				SerialNumber:         "SN0123456789",
				MacAddress:           "44:47:cc:11:22:33",
				FirmwareVersion:      "V5.6.5",
				FirmwareReleasedDate: "build 180316",
				EncoderVersion:       "V7.3",
				DeviceType:           "IPCamera",
				DeviceDescription:    "IPC",
			},
		},
		{
			name: "partial fields leave zero values",
			body: partialDeviceInfoXML,
			want: DeviceInfo{
				DeviceName: "Partial Cam",
				Model:      "DS-XYZ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/ISAPI/System/deviceInfo" {
					t.Errorf("path = %q, want /ISAPI/System/deviceInfo", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			got, err := c.DeviceInfo(context.Background())
			if err != nil {
				t.Fatalf("DeviceInfo: %v", err)
			}

			// XMLName is populated by the decoder; compare the payload fields explicitly.
			if got.DeviceName != tt.want.DeviceName {
				t.Errorf("DeviceName = %q, want %q", got.DeviceName, tt.want.DeviceName)
			}
			if got.DeviceID != tt.want.DeviceID {
				t.Errorf("DeviceID = %q, want %q", got.DeviceID, tt.want.DeviceID)
			}
			if got.Model != tt.want.Model {
				t.Errorf("Model = %q, want %q", got.Model, tt.want.Model)
			}
			if got.SerialNumber != tt.want.SerialNumber {
				t.Errorf("SerialNumber = %q, want %q", got.SerialNumber, tt.want.SerialNumber)
			}
			if got.MacAddress != tt.want.MacAddress {
				t.Errorf("MacAddress = %q, want %q", got.MacAddress, tt.want.MacAddress)
			}
			if got.FirmwareVersion != tt.want.FirmwareVersion {
				t.Errorf("FirmwareVersion = %q, want %q", got.FirmwareVersion, tt.want.FirmwareVersion)
			}
			if got.FirmwareReleasedDate != tt.want.FirmwareReleasedDate {
				t.Errorf("FirmwareReleasedDate = %q, want %q", got.FirmwareReleasedDate, tt.want.FirmwareReleasedDate)
			}
			if got.EncoderVersion != tt.want.EncoderVersion {
				t.Errorf("EncoderVersion = %q, want %q", got.EncoderVersion, tt.want.EncoderVersion)
			}
			if got.DeviceType != tt.want.DeviceType {
				t.Errorf("DeviceType = %q, want %q", got.DeviceType, tt.want.DeviceType)
			}
			if got.DeviceDescription != tt.want.DeviceDescription {
				t.Errorf("DeviceDescription = %q, want %q", got.DeviceDescription, tt.want.DeviceDescription)
			}
		})
	}
}

func TestDeviceInfoStatusErrors(t *testing.T) {
	t.Parallel()

	// For 401 we serve a valid Digest challenge on every call so the digest
	// transport retries once and then hands the second 401 back to the client
	// (rather than returning a parse error for a missing challenge).
	const challenge = `Digest realm="r", nonce="n", qop="auth", algorithm=MD5`

	tests := []struct {
		name       string
		status     int
		challenge  string // WWW-Authenticate value; only set for 401
		body       string
		wantErrIs  error
		wantStatus int // when we expect *StatusError
	}{
		{name: "401 -> ErrUnauthorized", status: http.StatusUnauthorized, challenge: challenge, wantErrIs: ErrUnauthorized},
		{name: "404 -> ErrNotFound", status: http.StatusNotFound, wantErrIs: ErrNotFound},
		{name: "500 -> StatusError", status: http.StatusInternalServerError, body: "boom", wantStatus: 500},
		{name: "418 -> StatusError empty body", status: http.StatusTeapot, wantStatus: http.StatusTeapot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.challenge != "" {
					w.Header().Set("WWW-Authenticate", tt.challenge)
				}
				w.WriteHeader(tt.status)
				if tt.body != "" {
					_, _ = w.Write([]byte(tt.body))
				}
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			_, err := c.DeviceInfo(context.Background())
			if err == nil {
				t.Fatal("err = nil, want error")
			}

			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("errors.Is(err, %v) = false; err = %v", tt.wantErrIs, err)
				}
				return
			}

			var se *StatusError
			if !errors.As(err, &se) {
				t.Fatalf("err type = %T (%v), want *StatusError", err, err)
			}
			if se.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %d, want %d", se.StatusCode, tt.wantStatus)
			}
			if tt.body != "" && !strings.Contains(se.Error(), tt.body) {
				t.Errorf("error %q missing body %q", se.Error(), tt.body)
			}
		})
	}
}

func TestStatusErrorTruncatesBody(t *testing.T) {
	t.Parallel()

	// Two variants: exactly-at-cap keeps the whole body; over-cap truncates.
	tests := []struct {
		name   string
		body   string
		wantLn int
	}{
		{name: "at cap", body: strings.Repeat("x", maxErrorBody), wantLn: maxErrorBody},
		{name: "over cap", body: strings.Repeat("x", maxErrorBody+256), wantLn: maxErrorBody},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL)
			_, err := c.DeviceInfo(context.Background())
			var se *StatusError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want *StatusError", err)
			}
			if len(se.Body) != tt.wantLn {
				t.Errorf("body len = %d, want %d", len(se.Body), tt.wantLn)
			}
		})
	}
}

func TestStatusErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		e    *StatusError
		want string
	}{
		{name: "with body", e: &StatusError{StatusCode: 500, Body: "nope"}, want: "isapi: unexpected status 500: nope"},
		{name: "empty body", e: &StatusError{StatusCode: 502}, want: "isapi: unexpected status 502"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.e.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeviceInfoMalformedXML(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<DeviceInfo><deviceName>oops`)) // never closed
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.DeviceInfo(context.Background())
	if err == nil {
		t.Fatal("err = nil, want parse error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("err = %v, want message containing 'parse'", err)
	}
}

func TestDeviceInfoContextCancelled(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	c := newTestClient(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after the request starts.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := c.DeviceInfo(ctx)
	if err == nil {
		t.Fatal("err = nil, want context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; err = %v", err)
	}
}

// TestDeviceInfoDigestChallengeRoundTrip proves the wired-up digest transport
// actually challenges (401 with WWW-Authenticate) and then succeeds on retry.
func TestDeviceInfoDigestChallengeRoundTrip(t *testing.T) {
	t.Parallel()

	const (
		realm  = "testrealm"
		nonce  = "abcdef1234567890"
		opaque = "opaque-value"
	)
	var calls int
	var sawAuthorization string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// Match the challenge shape used in internal/network/digest_test.go.
			w.Header().Set("WWW-Authenticate",
				`Digest realm="`+realm+`", nonce="`+nonce+`", qop="auth", opaque="`+opaque+`", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		sawAuthorization = r.Header.Get("Authorization")
		// Verify the client actually computed a digest response for testuser.
		expectedHA1 := md5Hex(testUser + ":" + realm + ":" + testPass)
		if !strings.Contains(sawAuthorization, `username="`+testUser+`"`) {
			t.Errorf("Authorization missing username: %q", sawAuthorization)
		}
		if !strings.Contains(sawAuthorization, `realm="`+realm+`"`) {
			t.Errorf("Authorization missing realm: %q", sawAuthorization)
		}
		if expectedHA1 == "" {
			t.Fatal("HA1 empty")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fullDeviceInfoXML))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	info, err := c.DeviceInfo(context.Background())
	if err != nil {
		t.Fatalf("DeviceInfo: %v", err)
	}

	// The digest round-trip must have issued exactly two calls: 401 then 200.
	if calls != 2 {
		t.Fatalf("server calls = %d, want 2 (401 then 200)", calls)
	}
	if sawAuthorization == "" {
		t.Fatal("retry request carried no Authorization header")
	}
	if !strings.HasPrefix(sawAuthorization, "Digest ") {
		t.Errorf("Authorization = %q, want Digest scheme", sawAuthorization)
	}
	if info.DeviceName != "Front Door Cam" {
		t.Errorf("DeviceName = %q, want %q", info.DeviceName, "Front Door Cam")
	}
	if info.SerialNumber != "SN0123456789" {
		t.Errorf("SerialNumber = %q, want %q", info.SerialNumber, "SN0123456789")
	}
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
