// Package isapi is a thin client for Hikvision's ISAPI over HTTP(S) with
// digest authentication supplied by internal/network.
package isapi

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cameronnewman/hikvision-tooling/internal/network"
)

// maxErrorBody caps the response body copied into error messages so a
// runaway server can't blow up log lines.
const maxErrorBody = 512

// Sentinel errors for the two status codes ISAPI callers commonly branch on.
var (
	ErrUnauthorized = errors.New("isapi: unauthorized")
	ErrNotFound     = errors.New("isapi: not found")
)

// StatusError is returned for non-2xx responses that aren't 401 or 404.
type StatusError struct {
	StatusCode int
	Body       string // truncated to maxErrorBody bytes
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("isapi: unexpected status %d", e.StatusCode)
	}
	return fmt.Sprintf("isapi: unexpected status %d: %s", e.StatusCode, e.Body)
}

// Client talks to a single Hikvision device over ISAPI.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// Options configures a Client. Zero values are sensible defaults: no timeout,
// verified TLS, empty credentials (works against endpoints that don't challenge).
type Options struct {
	Username           string
	Password           string
	Timeout            time.Duration
	InsecureSkipVerify bool
}

// New builds a Client whose transport is the digest wrapper on top of a
// fresh *http.Transport carrying the requested TLS settings.
func New(baseURL string, opts Options) *Client {
	base := &http.Transport{
		// #nosec G402 -- InsecureSkipVerify is opt-in via Options.InsecureSkipVerify
		TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.InsecureSkipVerify},
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: opts.Timeout,
			Transport: &network.Transport{
				Username: opts.Username,
				Password: opts.Password,
				Base:     base,
			},
		},
	}
}

// Get issues a GET against BaseURL+path. Caller closes the response body.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	if path == "" || path[0] != '/' {
		path = "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("isapi: build request: %w", err)
	}
	return c.HTTPClient.Do(req)
}

// DeviceInfo is the parsed shape of GET /ISAPI/System/deviceInfo. Fields
// missing from the device response stay at their zero value.
type DeviceInfo struct {
	XMLName              xml.Name `xml:"DeviceInfo" json:"-"`
	DeviceName           string   `xml:"deviceName" json:"deviceName,omitempty"`
	DeviceID             string   `xml:"deviceID" json:"deviceID,omitempty"`
	Model                string   `xml:"model" json:"model,omitempty"`
	SerialNumber         string   `xml:"serialNumber" json:"serialNumber,omitempty"`
	MacAddress           string   `xml:"macAddress" json:"macAddress,omitempty"`
	FirmwareVersion      string   `xml:"firmwareVersion" json:"firmwareVersion,omitempty"`
	FirmwareReleasedDate string   `xml:"firmwareReleasedDate" json:"firmwareReleasedDate,omitempty"`
	EncoderVersion       string   `xml:"encoderVersion" json:"encoderVersion,omitempty"`
	DeviceType           string   `xml:"deviceType" json:"deviceType,omitempty"`
	DeviceDescription    string   `xml:"deviceDescription" json:"deviceDescription,omitempty"`
}

// DeviceInfo fetches and decodes /ISAPI/System/deviceInfo.
func (c *Client) DeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	resp, err := c.Get(ctx, "/ISAPI/System/deviceInfo")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("isapi: read body: %w", err)
	}

	if err := statusError(resp.StatusCode, body); err != nil {
		return nil, err
	}

	var info DeviceInfo
	if err := xml.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("isapi: parse deviceInfo: %w", err)
	}
	return &info, nil
}

// statusError maps a status code to a typed error, or nil for 2xx.
func statusError(code int, body []byte) error {
	if code >= 200 && code < 300 {
		return nil
	}
	switch code {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return &StatusError{StatusCode: code, Body: truncate(body, maxErrorBody)}
	}
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n])
}
