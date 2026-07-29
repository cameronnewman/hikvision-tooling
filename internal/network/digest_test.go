package network

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestComputeResponseRFC2617Vector(t *testing.T) {
	t.Parallel()

	// RFC 2617 §3.5 canonical vector.
	got := computeResponse(
		"Mufasa",
		"Circle Of Life",
		"testrealm@host.com",
		"dcd98b7102dd2f0e8b11d0f600bfb0c093",
		"0a4f113b",
		"00000001",
		"auth",
		"MD5",
		"GET",
		"/dir/index.html",
	)
	const want = "6629fae49393a05397450978507c4ef1"
	if got != want {
		t.Fatalf("computeResponse = %q, want %q", got, want)
	}
}

func TestComputeResponseVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		algorithm string
		qop       string
	}{
		{name: "MD5 with qop=auth", algorithm: "MD5", qop: "auth"},
		{name: "MD5 no qop (RFC 2069)", algorithm: "MD5", qop: ""},
		{name: "MD5-sess with qop=auth", algorithm: "MD5-sess", qop: "auth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := computeResponse(
				"testuser", "testpass", "realm@example",
				"nonce123", "cnonce456", "00000001", tt.qop, tt.algorithm,
				"GET", "/path",
			)
			if len(got) != 32 {
				t.Errorf("response length = %d, want 32 hex chars", len(got))
			}
		})
	}
}

func TestParseChallenge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		header    string
		wantErr   bool
		wantRealm string
		wantNonce string
		wantAlgo  string
		wantQop   string
		wantOpaq  string
	}{
		{
			name:      "quoted values",
			header:    `Digest realm="testrealm@host.com", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", qop="auth", opaque="5ccc069c403ebaf9f0171e9517f40e41"`,
			wantRealm: "testrealm@host.com",
			wantNonce: "dcd98b7102dd2f0e8b11d0f600bfb0c093",
			wantAlgo:  "MD5",
			wantQop:   "auth",
			wantOpaq:  "5ccc069c403ebaf9f0171e9517f40e41",
		},
		{
			name:      "unquoted algorithm token",
			header:    `Digest realm="r", nonce="n", algorithm=MD5, qop="auth"`,
			wantRealm: "r",
			wantNonce: "n",
			wantAlgo:  "MD5",
			wantQop:   "auth",
		},
		{
			name:      "qop list with auth-int",
			header:    `Digest realm="r", nonce="n", qop="auth,auth-int"`,
			wantRealm: "r",
			wantNonce: "n",
			wantAlgo:  "MD5",
			wantQop:   "auth,auth-int",
		},
		{
			name:      "missing opaque",
			header:    `Digest realm="r", nonce="n", qop="auth"`,
			wantRealm: "r",
			wantNonce: "n",
			wantAlgo:  "MD5",
			wantQop:   "auth",
			wantOpaq:  "",
		},
		{
			name:      "MD5-sess algorithm",
			header:    `Digest realm="r", nonce="n", algorithm=MD5-sess, qop="auth"`,
			wantRealm: "r",
			wantNonce: "n",
			wantAlgo:  "MD5-sess",
			wantQop:   "auth",
		},
		{
			name:      "extra whitespace",
			header:    `Digest    realm="r" ,   nonce="n"  ,  qop="auth"`,
			wantRealm: "r",
			wantNonce: "n",
			wantAlgo:  "MD5",
			wantQop:   "auth",
		},
		{
			name:      "mixed-case scheme Digest",
			header:    `Digest realm="r", nonce="n"`,
			wantRealm: "r",
			wantNonce: "n",
			wantAlgo:  "MD5",
		},
		{
			name:      "lowercase scheme digest",
			header:    `digest realm="r", nonce="n"`,
			wantRealm: "r",
			wantNonce: "n",
			wantAlgo:  "MD5",
		},
		{
			name:      "uppercase scheme DIGEST",
			header:    `DIGEST realm="r", nonce="n"`,
			wantRealm: "r",
			wantNonce: "n",
			wantAlgo:  "MD5",
		},
		{
			name:    "missing realm",
			header:  `Digest nonce="n", qop="auth"`,
			wantErr: true,
		},
		{
			name:    "unterminated quoted string",
			header:  `Digest realm="r, nonce="n"`,
			wantErr: true,
		},
		{
			name:    "not a Digest scheme",
			header:  `Basic realm="r"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseChallenge([]string{tt.header})
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseChallenge() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.realm != tt.wantRealm {
				t.Errorf("realm = %q, want %q", got.realm, tt.wantRealm)
			}
			if got.nonce != tt.wantNonce {
				t.Errorf("nonce = %q, want %q", got.nonce, tt.wantNonce)
			}
			if got.algorithm != tt.wantAlgo {
				t.Errorf("algorithm = %q, want %q", got.algorithm, tt.wantAlgo)
			}
			if got.qop != tt.wantQop {
				t.Errorf("qop = %q, want %q", got.qop, tt.wantQop)
			}
			if got.opaque != tt.wantOpaq {
				t.Errorf("opaque = %q, want %q", got.opaque, tt.wantOpaq)
			}
		})
	}
}

func TestSelectQop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{name: "empty", in: "", want: ""},
		{name: "auth only", in: "auth", want: "auth"},
		{name: "auth in list", in: "auth-int, auth", want: "auth"},
		{name: "auth-int only", in: "auth-int", wantErr: ErrAuthIntUnsupported},
		{name: "unknown", in: "foo", wantErr: errors.New("stub")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := selectQop(tt.in)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("selectQop(%q) err = nil, want error", tt.in)
				}
				if errors.Is(tt.wantErr, ErrAuthIntUnsupported) && !errors.Is(err, ErrAuthIntUnsupported) {
					t.Errorf("selectQop(%q) err = %v, want ErrAuthIntUnsupported", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectQop(%q) unexpected err: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("selectQop(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildAuthHeaderOrdering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		p        authParams
		contains []string
		absent   []string
	}{
		{
			name: "with qop and opaque",
			p: authParams{
				username: "u", realm: "r", nonce: "n", uri: "/x",
				algorithm: "MD5", qop: "auth", nc: "00000001", cnonce: "cn",
				response: "abc", opaque: "op",
			},
			contains: []string{
				`Digest username="u"`,
				`realm="r"`,
				`nonce="n"`,
				`uri="/x"`,
				`algorithm=MD5`,
				`qop=auth`,
				`nc=00000001`,
				`cnonce="cn"`,
				`response="abc"`,
				`opaque="op"`,
			},
		},
		{
			name: "no qop no opaque (RFC 2069)",
			p: authParams{
				username: "u", realm: "r", nonce: "n", uri: "/x",
				algorithm: "MD5", response: "abc",
			},
			contains: []string{
				`Digest username="u"`,
				`realm="r"`,
				`response="abc"`,
			},
			absent: []string{"qop=", "nc=", "cnonce=", "opaque="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildAuthHeader(tt.p)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("header missing %q\ngot: %s", want, got)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("header should not contain %q\ngot: %s", unwanted, got)
				}
			}

			// Verify field ordering: username < realm < nonce < uri < algorithm < response.
			order := []string{"username=", "realm=", "nonce=", "uri=", "algorithm=", "response="}
			last := -1
			for _, k := range order {
				idx := strings.Index(got, k)
				if idx == -1 {
					continue
				}
				if idx < last {
					t.Errorf("field %q out of order in %s", k, got)
				}
				last = idx
			}
		})
	}
}

func TestTransportRoundTripSuccess(t *testing.T) {
	t.Parallel()

	const (
		user   = "testuser"
		pass   = "testpass"
		realm  = "testrealm"
		nonce  = "abcdef1234567890"
		opaque = "opaque-value"
	)

	payload := strings.Repeat("A", 100)
	var receivedBody string
	calls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("WWW-Authenticate",
				`Digest realm="`+realm+`", nonce="`+nonce+`", qop="auth", opaque="`+opaque+`", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got, _ := io.ReadAll(r.Body)
		receivedBody = string(got)

		auth := r.Header.Get("Authorization")
		params, err := parseParams(strings.TrimPrefix(auth, "Digest "))
		if err != nil {
			t.Errorf("server: parse Authorization: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		want := computeResponse(user, pass, realm, nonce,
			params["cnonce"], params["nc"], params["qop"], params["algorithm"],
			r.Method, params["uri"])
		if params["response"] != want {
			t.Errorf("server: response = %q, want %q", params["response"], want)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if params["opaque"] != opaque {
			t.Errorf("server: opaque = %q, want %q", params["opaque"], opaque)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Transport: &Transport{Username: user, Password: pass}}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if calls != 2 {
		t.Errorf("server calls = %d, want 2", calls)
	}
	if receivedBody != payload {
		t.Errorf("body on retry = %q, want %q", receivedBody, payload)
	}
}

func TestTransportAuthIntOnly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate",
			`Digest realm="r", nonce="n", qop="auth-int"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// Drive RoundTrip directly: http.Client discards the response when the
	// RoundTripper returns both, but the transport contract preserves it.
	tr := &Transport{Username: "testuser", Password: "testpass"}
	req, _ := http.NewRequest(http.MethodGet, server.URL, http.NoBody)
	resp, err := tr.RoundTrip(req)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("RoundTrip() err = nil, want ErrAuthIntUnsupported")
	}
	if !errors.Is(err, ErrAuthIntUnsupported) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("err = %v, want ErrAuthIntUnsupported", err)
	}
	if resp == nil {
		t.Fatal("resp = nil, want original 401 preserved")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
	}
}

func TestTransportSecondStillUnauthorized(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("WWW-Authenticate",
			`Digest realm="r", nonce="n`+"|"+`", qop="auth", algorithm=MD5`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &http.Client{Transport: &Transport{Username: "testuser", Password: "wrongpass"}}
	req, _ := http.NewRequest(http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (no retry loop)", calls)
	}
}

func TestTransportMalformedChallenge(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="r"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &http.Client{Transport: &Transport{Username: "testuser", Password: "testpass"}}
	req, _ := http.NewRequest(http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("Do() err = nil, want malformed challenge error")
	}
}

func TestTransportNonDigestUnauthorized(t *testing.T) {
	t.Parallel()

	// Missing WWW-Authenticate entirely -> parse error, original 401 preserved.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &http.Client{Transport: &Transport{Username: "testuser", Password: "testpass"}}
	req, _ := http.NewRequest(http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("Do() err = nil, want error for missing challenge")
	}
}

func TestTransportPassesThroughNon401(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &http.Client{Transport: &Transport{Username: "testuser", Password: "testpass"}}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestTransportNilBodyPreserved(t *testing.T) {
	t.Parallel()

	const user, pass = "testuser", "testpass"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("WWW-Authenticate",
				`Digest realm="r", nonce="n", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got, _ := io.ReadAll(r.Body)
		if len(got) != 0 {
			t.Errorf("expected empty body on retry, got %d bytes", len(got))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Transport: &Transport{Username: user, Password: pass}}
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestSnapshotBodyOversized(t *testing.T) {
	t.Parallel()

	big := strings.NewReader(strings.Repeat("x", maxBufferedBody+1))
	req, _ := http.NewRequest(http.MethodPost, "http://example.invalid/", big)
	tr := &Transport{Username: "u", Password: "p"}
	resp, err := tr.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
		t.Fatalf("resp = %v, want nil for oversize body", resp)
	}
	if err == nil {
		t.Fatal("RoundTrip: err = nil, want oversize error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("err = %v, want oversize error mentioning 'exceeds'", err)
	}
}

func TestSnapshotBodyUnderLimit(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodPost, "http://example.invalid/",
		strings.NewReader("hello"))
	got, err := snapshotBody(req)
	if err != nil {
		t.Fatalf("snapshotBody: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("body = %q, want %q", got, "hello")
	}
}

func TestSnapshotBodyNil(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	got, err := snapshotBody(req)
	if err != nil {
		t.Fatalf("snapshotBody: %v", err)
	}
	if got != nil {
		t.Errorf("body = %v, want nil", got)
	}
}
