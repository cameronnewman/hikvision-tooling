package network

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTPClient handles raw HTTP requests
type HTTPClient struct {
	UserAgent string
	Timeout   time.Duration
}

// NewHTTPClient creates a new HTTP client
func NewHTTPClient(userAgent string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		UserAgent: userAgent,
		Timeout:   timeout,
	}
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

// Get performs an HTTP GET request
func (c *HTTPClient) Get(ipAddress, path string) (*HTTPResponse, error) {
	return c.GetWithAuth(ipAddress, path, "")
}

// GetWithAuth performs an HTTP GET request with an auth token
func (c *HTTPClient) GetWithAuth(ipAddress, path, authToken string) (*HTTPResponse, error) {
	fullURL := fmt.Sprintf("http://%s%s", ipAddress, path)
	if authToken != "" {
		fullURL += "?auth=" + authToken
	}

	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	port := parsedURL.Port()
	if port == "" {
		port = "80"
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(parsedURL.Hostname(), port), c.Timeout)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, fmt.Errorf("connection timeout to %s", ipAddress)
		}
		return nil, fmt.Errorf("connection failed to %s: %w", ipAddress, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(c.Timeout))

	pathWithQuery := parsedURL.Path
	if parsedURL.RawQuery != "" {
		pathWithQuery += "?" + parsedURL.RawQuery
	}

	httpRequest := fmt.Sprintf(
		"GET %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"User-Agent: %s\r\n"+
			"Accept: */*\r\n"+
			"Connection: close\r\n"+
			"\r\n",
		pathWithQuery,
		parsedURL.Hostname(),
		c.UserAgent,
	)

	if _, err := conn.Write([]byte(httpRequest)); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, conn); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseHTTPResponse(buf.Bytes())
}

func parseHTTPResponse(data []byte) (*HTTPResponse, error) {
	headerEnd := bytes.Index(data, []byte("\r\n\r\n"))
	if headerEnd == -1 {
		return nil, fmt.Errorf("invalid HTTP response: no header separator found")
	}

	headerBytes := data[:headerEnd]
	body := data[headerEnd+4:]

	lines := strings.Split(string(headerBytes), "\r\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("invalid HTTP response: no status line")
	}

	statusParts := strings.SplitN(lines[0], " ", 3)
	if len(statusParts) < 2 {
		return nil, fmt.Errorf("invalid HTTP status line: %s", lines[0])
	}

	statusCode, err := strconv.Atoi(statusParts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid status code: %s", statusParts[1])
	}

	headers := make(map[string]string)
	for _, line := range lines[1:] {
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			headers[strings.ToLower(parts[0])] = parts[1]
		}
	}

	return &HTTPResponse{
		StatusCode: statusCode,
		Body:       body,
		Headers:    headers,
	}, nil
}
