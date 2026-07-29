package network

import (
	"bytes"
	"crypto/md5" // #nosec G501 -- MD5 is mandated by RFC 2617 for HTTP Digest auth
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxBufferedBody caps in-memory copies of request bodies. Digest requires
// a retry with the same payload; unbounded buffering would be a footgun.
const maxBufferedBody = 10 * 1024 * 1024

// ErrAuthIntUnsupported is returned when a server offers only qop=auth-int.
var ErrAuthIntUnsupported = errors.New("digest: server requires auth-int which is not supported")

// Transport wraps a RoundTripper and answers 401 Digest challenges per RFC 2617.
type Transport struct {
	Username string
	Password string
	Base     http.RoundTripper
}

// RoundTrip issues req and, on a 401 Digest challenge, retries once with a
// computed Authorization header. Bodies are buffered so the retry sees the
// same payload the caller provided.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	body, err := snapshotBody(req)
	if err != nil {
		return nil, err
	}

	first := cloneWithBody(req, body)
	resp, err := base.RoundTrip(first)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge, perr := parseChallenge(resp.Header.Values("WWW-Authenticate"))
	if perr != nil {
		return resp, perr
	}

	qop, qerr := selectQop(challenge.qop)
	if qerr != nil {
		return resp, qerr
	}

	// Drain and close so the underlying transport can reuse the connection.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	cnonce, err := newCnonce()
	if err != nil {
		return nil, err
	}
	const nc = "00000001"

	uri := req.URL.RequestURI()
	response := computeResponse(
		t.Username, t.Password, challenge.realm, challenge.nonce,
		cnonce, nc, qop, challenge.algorithm, req.Method, uri,
	)

	auth := buildAuthHeader(authParams{
		username:  t.Username,
		realm:     challenge.realm,
		nonce:     challenge.nonce,
		uri:       uri,
		algorithm: challenge.algorithm,
		qop:       qop,
		nc:        nc,
		cnonce:    cnonce,
		response:  response,
		opaque:    challenge.opaque,
	})

	second := cloneWithBody(req, body)
	second.Header.Set("Authorization", auth)
	return base.RoundTrip(second)
}

type challenge struct {
	realm     string
	nonce     string
	opaque    string
	algorithm string
	qop       string
}

// parseChallenge scans one or more WWW-Authenticate values and returns the
// first Digest challenge found.
func parseChallenge(headers []string) (challenge, error) {
	for _, h := range headers {
		trimmed := strings.TrimLeft(h, " \t")
		if len(trimmed) < 6 || !strings.EqualFold(trimmed[:6], "Digest") {
			continue
		}
		rest := strings.TrimLeft(trimmed[6:], " \t")
		params, err := parseParams(rest)
		if err != nil {
			return challenge{}, err
		}
		c := challenge{
			realm:     params["realm"],
			nonce:     params["nonce"],
			opaque:    params["opaque"],
			algorithm: params["algorithm"],
			qop:       params["qop"],
		}
		if c.realm == "" || c.nonce == "" {
			return challenge{}, errors.New("digest: challenge missing realm or nonce")
		}
		if c.algorithm == "" {
			c.algorithm = "MD5"
		}
		return c, nil
	}
	return challenge{}, errors.New("digest: no Digest challenge in WWW-Authenticate")
}

// parseParams handles the auth-param list, respecting quoted strings so
// commas inside qop="auth,auth-int" don't split the value.
func parseParams(s string) (map[string]string, error) {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}
		eq := indexOutsideQuotes(s[i:], '=')
		if eq < 0 {
			return nil, fmt.Errorf("digest: malformed parameter near %q", s[i:])
		}
		key := strings.TrimSpace(s[i : i+eq])
		i += eq + 1
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			out[strings.ToLower(key)] = ""
			break
		}
		var val string
		if s[i] == '"' {
			end := i + 1
			for end < len(s) && s[end] != '"' {
				if s[end] == '\\' && end+1 < len(s) {
					end += 2
					continue
				}
				end++
			}
			if end >= len(s) {
				return nil, errors.New("digest: unterminated quoted string")
			}
			val = unescape(s[i+1 : end])
			i = end + 1
		} else {
			end := i
			for end < len(s) && s[end] != ',' {
				end++
			}
			val = strings.TrimSpace(s[i:end])
			i = end
		}
		out[strings.ToLower(key)] = val
	}
	return out, nil
}

func indexOutsideQuotes(s string, c byte) int {
	inQ := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && inQ && i+1 < len(s):
			i++
		case s[i] == '"':
			inQ = !inQ
		case s[i] == c && !inQ:
			return i
		}
	}
	return -1
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// selectQop picks "auth" when offered, tolerates no-qop (RFC 2069), and
// refuses auth-int-only challenges.
func selectQop(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	hasAuthInt := false
	for _, part := range strings.Split(raw, ",") {
		v := strings.TrimSpace(part)
		if strings.EqualFold(v, "auth") {
			return "auth", nil
		}
		if strings.EqualFold(v, "auth-int") {
			hasAuthInt = true
		}
	}
	if hasAuthInt {
		return "", ErrAuthIntUnsupported
	}
	return "", fmt.Errorf("digest: no supported qop in %q", raw)
}

// computeResponse implements the RFC 2617 §3.2.2.1 formula.
func computeResponse(username, password, realm, nonce, cnonce, nc, qop, algorithm, method, uri string) string {
	ha1 := md5Hex(username + ":" + realm + ":" + password)
	if strings.EqualFold(algorithm, "MD5-sess") {
		ha1 = md5Hex(ha1 + ":" + nonce + ":" + cnonce)
	}
	ha2 := md5Hex(method + ":" + uri)
	if qop == "" {
		return md5Hex(ha1 + ":" + nonce + ":" + ha2)
	}
	return md5Hex(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s)) // #nosec G401 -- MD5 is mandated by RFC 2617
	return hex.EncodeToString(sum[:])
}

type authParams struct {
	username, realm, nonce, uri string
	algorithm, qop, nc, cnonce  string
	response, opaque            string
}

func buildAuthHeader(p authParams) string {
	var b strings.Builder
	b.WriteString("Digest ")
	writeQ := func(k, v string) { fmt.Fprintf(&b, `%s="%s"`, k, v) }
	writeU := func(k, v string) { fmt.Fprintf(&b, `%s=%s`, k, v) }

	writeQ("username", p.username)
	b.WriteString(", ")
	writeQ("realm", p.realm)
	b.WriteString(", ")
	writeQ("nonce", p.nonce)
	b.WriteString(", ")
	writeQ("uri", p.uri)
	b.WriteString(", ")
	writeU("algorithm", p.algorithm)
	if p.qop != "" {
		b.WriteString(", ")
		writeU("qop", p.qop)
		b.WriteString(", ")
		writeU("nc", p.nc)
		b.WriteString(", ")
		writeQ("cnonce", p.cnonce)
	}
	b.WriteString(", ")
	writeQ("response", p.response)
	if p.opaque != "" {
		b.WriteString(", ")
		writeQ("opaque", p.opaque)
	}
	return b.String()
}

func newCnonce() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("digest: cnonce: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// snapshotBody reads req.Body once so the retry can replay the same payload.
// A nil body stays nil. Non-seekable bodies over maxBufferedBody are refused
// rather than silently truncated.
func snapshotBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	limited := io.LimitReader(req.Body, maxBufferedBody+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("digest: buffer body: %w", err)
	}
	_ = req.Body.Close()
	if len(buf) > maxBufferedBody {
		return nil, fmt.Errorf("digest: request body exceeds %d bytes", maxBufferedBody)
	}
	return buf, nil
}

func cloneWithBody(req *http.Request, body []byte) *http.Request {
	r := req.Clone(req.Context())
	if body == nil {
		r.Body = nil
		r.ContentLength = 0
		r.GetBody = nil
		return r
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return r
}
