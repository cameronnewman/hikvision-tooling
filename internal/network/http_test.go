package network

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name        string
		userAgent   string
		timeout     time.Duration
		wantAgent   string
		wantTimeout time.Duration
	}{
		{
			name:        "standard values",
			userAgent:   "TestAgent/1.0",
			timeout:     5 * time.Second,
			wantAgent:   "TestAgent/1.0",
			wantTimeout: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewHTTPClient(tt.userAgent, tt.timeout)
			if client == nil {
				t.Fatal("NewHTTPClient returned nil")
			}
			if client.UserAgent != tt.wantAgent {
				t.Errorf("UserAgent = %q, want %q", client.UserAgent, tt.wantAgent)
			}
			if client.Timeout != tt.wantTimeout {
				t.Errorf("Timeout = %v, want %v", client.Timeout, tt.wantTimeout)
			}
		})
	}
}

func TestHTTPClientGet(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		responseCode   int
		expectedBody   string
		expectedStatus int
	}{
		{
			name:           "successful GET request",
			responseBody:   "Hello, World!",
			responseCode:   http.StatusOK,
			expectedBody:   "Hello, World!",
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(tt.responseCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			addr := strings.TrimPrefix(server.URL, "http://")

			client := NewHTTPClient("TestAgent", 5*time.Second)
			resp, err := client.Get(addr, "/")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("StatusCode = %d, want %d", resp.StatusCode, tt.expectedStatus)
			}

			if string(resp.Body) != tt.expectedBody {
				t.Errorf("Body = %q, want %q", resp.Body, tt.expectedBody)
			}
		})
	}
}

func TestHTTPClientGetWithAuth(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		expectedToken string
		wantErr       bool
	}{
		{
			name:          "with auth token",
			token:         "mytoken",
			expectedToken: "mytoken",
			wantErr:       false,
		},
		{
			name:          "empty auth token",
			token:         "",
			expectedToken: "",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedAuth string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedAuth = r.URL.Query().Get("auth")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("authenticated"))
			}))
			defer server.Close()

			addr := strings.TrimPrefix(server.URL, "http://")

			client := NewHTTPClient("TestAgent", 5*time.Second)
			resp, err := client.GetWithAuth(addr, "/test", tt.token)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetWithAuth() error = %v, wantErr %v", err, tt.wantErr)
			}

			if resp.StatusCode != 200 {
				t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
			}

			if receivedAuth != tt.expectedToken {
				t.Errorf("auth token = %q, want %q", receivedAuth, tt.expectedToken)
			}
		})
	}
}

func TestHTTPClientConnectionError(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		timeout time.Duration
		wantErr bool
	}{
		{
			name:    "unreachable TEST-NET-1 address",
			addr:    "192.0.2.1:12345",
			timeout: 100 * time.Millisecond,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewHTTPClient("TestAgent", tt.timeout)
			_, err := client.Get(tt.addr, "/")
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPClientTimeout(t *testing.T) {
	tests := []struct {
		name         string
		serverDelay  time.Duration
		clientTimout time.Duration
		wantErr      bool
	}{
		{
			name:         "server slower than client timeout",
			serverDelay:  2 * time.Second,
			clientTimout: 100 * time.Millisecond,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.serverDelay)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			addr := strings.TrimPrefix(server.URL, "http://")

			client := NewHTTPClient("TestAgent", tt.clientTimout)
			_, err := client.Get(addr, "/")
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPClientDifferentStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "status 200", statusCode: 200},
		{name: "status 201", statusCode: 201},
		{name: "status 301", statusCode: 301},
		{name: "status 400", statusCode: 400},
		{name: "status 401", statusCode: 401},
		{name: "status 403", statusCode: 403},
		{name: "status 404", statusCode: 404},
		{name: "status 500", statusCode: 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			addr := strings.TrimPrefix(server.URL, "http://")

			client := NewHTTPClient("TestAgent", 5*time.Second)
			resp, err := client.Get(addr, "/")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if resp.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %d, want %d", resp.StatusCode, tt.statusCode)
			}
		})
	}
}

func TestHTTPClientHeaders(t *testing.T) {
	tests := []struct {
		name                string
		customHeader        string
		customValue         string
		contentType         string
		expectedContentType string
	}{
		{
			name:                "custom headers returned",
			customHeader:        "X-Custom-Header",
			customValue:         "test-value",
			contentType:         "application/json",
			expectedContentType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(tt.customHeader, tt.customValue)
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))
			defer server.Close()

			addr := strings.TrimPrefix(server.URL, "http://")

			client := NewHTTPClient("TestAgent", 5*time.Second)
			resp, err := client.Get(addr, "/")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if resp.Headers == nil {
				t.Fatal("Headers is nil")
			}

			if resp.Headers["content-type"] != tt.expectedContentType {
				t.Errorf("Content-Type = %q, want %q", resp.Headers["content-type"], tt.expectedContentType)
			}
		})
	}
}

func TestParseHTTPResponseInvalid(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "no separator",
			data:    []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain"),
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "invalid status",
			data:    []byte("INVALID\r\n\r\n"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseHTTPResponse(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseHTTPResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPClientInvalidURL(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		path    string
		token   string
		wantErr bool
	}{
		{
			name:    "space in host creates invalid URL",
			addr:    "invalid host",
			path:    "/path",
			token:   "token",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewHTTPClient("TestAgent", 5*time.Second)
			_, err := client.GetWithAuth(tt.addr, tt.path, tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetWithAuth() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPClientWithPort(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		expectedStatus int
	}{
		{
			name:           "custom port server",
			responseBody:   "OK",
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Find a free port
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("Failed to get free port: %v", err)
			}
			port := listener.Addr().(*net.TCPAddr).Port
			_ = listener.Close()

			// Start server on that port
			server := &http.Server{
				Addr: fmt.Sprintf("127.0.0.1:%d", port),
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tt.responseBody))
				}),
			}
			go func() { _ = server.ListenAndServe() }()
			defer func() { _ = server.Close() }()

			// Give server time to start
			time.Sleep(100 * time.Millisecond)

			client := NewHTTPClient("TestAgent", 5*time.Second)
			resp, err := client.Get(fmt.Sprintf("127.0.0.1:%d", port), "/")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("StatusCode = %d, want %d", resp.StatusCode, tt.expectedStatus)
			}
		})
	}
}
