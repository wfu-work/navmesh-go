package httpgateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	"navmesh-go/domains"

	"github.com/quic-go/quic-go"
)

func TestHTTPReverseProxyRewritePreservesUpgradeAndSetsForwardedHeaders(t *testing.T) {
	in := httptest.NewRequest(http.MethodGet, "http://device.example.com/ws", nil)
	in.Host = "device.example.com"
	in.RemoteAddr = "10.0.0.10:43210"
	in.Header.Set("Connection", "Upgrade")
	in.Header.Set("Upgrade", "websocket")
	in.Header.Set("X-Forwarded-Proto", "https")
	out := in.Clone(context.Background())

	mapping := domains.PortMapping{
		PublicHost: "device.example.com",
		TargetPort: 8080,
	}
	mapping.Guid = "mapping-guid"
	proxy := newHTTPReverseProxy(&http.Transport{}, mapping, domains.Device{}, "device.example.com", "198.51.100.23")
	proxy.proxy.Rewrite(&httputil.ProxyRequest{In: in, Out: out})

	if out.URL.Scheme != "http" {
		t.Fatalf("scheme = %q, want http", out.URL.Scheme)
	}
	if out.URL.Host != "mapping-guid.navmesh-http.local:8080" {
		t.Fatalf("url host = %q", out.URL.Host)
	}
	if out.Host != "device.example.com" {
		t.Fatalf("host = %q", out.Host)
	}
	if got := out.Header.Get("Connection"); got != "Upgrade" {
		t.Fatalf("Connection = %q, want Upgrade", got)
	}
	if got := out.Header.Get("Upgrade"); got != "websocket" {
		t.Fatalf("Upgrade = %q, want websocket", got)
	}
	if got := out.Header.Get("X-Real-IP"); got != "198.51.100.23" {
		t.Fatalf("X-Real-IP = %q", got)
	}
	if got := out.Header.Get("X-Forwarded-For"); got != "198.51.100.23" {
		t.Fatalf("X-Forwarded-For = %q", got)
	}
	if got := out.Header.Get("X-Forwarded-Host"); got != "device.example.com" {
		t.Fatalf("X-Forwarded-Host = %q", got)
	}
	if got := out.Header.Get("X-Forwarded-Proto"); got != "https" {
		t.Fatalf("X-Forwarded-Proto = %q", got)
	}
	if got := out.Header.Get("X-Forwarded-Port"); got != "443" {
		t.Fatalf("X-Forwarded-Port = %q", got)
	}
}

func TestHTTPReverseProxyServeHTTPForwardsUpgradeAndHeaders(t *testing.T) {
	in := httptest.NewRequest(http.MethodGet, "http://device.example.com/ws", nil)
	in.Host = "device.example.com"
	in.RemoteAddr = "10.0.0.10:43210"
	in.Header.Set("Connection", "Upgrade")
	in.Header.Set("Upgrade", "websocket")
	in.Header.Set("X-Forwarded-Proto", "https")

	mapping := domains.PortMapping{
		PublicHost: "device.example.com",
		TargetPort: 8080,
	}
	mapping.Guid = "mapping-guid"
	proxy := newHTTPReverseProxy(&http.Transport{}, mapping, domains.Device{}, "device.example.com", "198.51.100.23")
	capture := &captureRoundTripper{}
	proxy.proxy.Transport = capture
	proxy.ServeHTTP(httptest.NewRecorder(), in)

	if capture.request == nil {
		t.Fatal("outbound request was not captured")
	}
	out := capture.request
	if got := out.Header.Get("Connection"); got != "Upgrade" {
		t.Fatalf("Connection = %q, want Upgrade", got)
	}
	if got := out.Header.Get("Upgrade"); got != "websocket" {
		t.Fatalf("Upgrade = %q, want websocket", got)
	}
	if got := out.Header.Get("X-Real-IP"); got != "198.51.100.23" {
		t.Fatalf("X-Real-IP = %q", got)
	}
	if got := out.Header.Get("X-Forwarded-For"); got != "198.51.100.23" {
		t.Fatalf("X-Forwarded-For = %q", got)
	}
	if got := out.Header.Get("X-Forwarded-Host"); got != "device.example.com" {
		t.Fatalf("X-Forwarded-Host = %q", got)
	}
	if got := out.Header.Get("X-Forwarded-Proto"); got != "https" {
		t.Fatalf("X-Forwarded-Proto = %q", got)
	}
	if got := out.Header.Get("X-Forwarded-Port"); got != "443" {
		t.Fatalf("X-Forwarded-Port = %q", got)
	}
}

func TestRequestProtoReadsForwardedHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://device.example.com/", nil)
	req.Header.Set("Forwarded", `for=198.51.100.23;proto=https`)

	if got := requestProto(req); got != "https" {
		t.Fatalf("proto = %q, want https", got)
	}
}

func TestStatusCodeForProxyErrorUsesTooManyRequestsForPolicyRejects(t *testing.T) {
	if got := statusCodeForProxyError(errors.New("max device sessions exceeded")); got != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", got, http.StatusTooManyRequests)
	}
	if got := statusCodeForProxyError(errors.New("tcp data channel exhausted: context deadline exceeded")); got != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", got, http.StatusTooManyRequests)
	}
	if got := statusCodeForProxyError(errors.New("device tunnel offline")); got != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", got, http.StatusBadGateway)
	}
}

func TestNewTunnelHTTPTransportConnectionPolicy(t *testing.T) {
	transport := newTunnelHTTPTransport(nil)

	if transport.MaxConnsPerHost != tunnelHTTPMaxConnsPerHost {
		t.Fatalf("MaxConnsPerHost = %d, want %d", transport.MaxConnsPerHost, tunnelHTTPMaxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != tunnelHTTPMaxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, tunnelHTTPMaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != tunnelHTTPIdleConnTimeout {
		t.Fatalf("IdleConnTimeout = %s, want %s", transport.IdleConnTimeout, tunnelHTTPIdleConnTimeout)
	}
}

func TestAccessLogErrorMessageKeepsSuccessfulClosesClean(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "empty", reason: "", want: ""},
		{name: "normal close", reason: "closed", want: ""},
		{name: "trim normal close", reason: " closed ", want: ""},
		{name: "proxy error", reason: "proxy_error: timeout", want: "proxy_error: timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accessLogErrorMessage(tt.reason); got != tt.want {
				t.Fatalf("error message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTunnelHTTPConnRefreshesIdleDeadlineOnActivity(t *testing.T) {
	base := &deadlineConn{readBuffer: bytes.NewBufferString("hello")}
	conn := &tunnelHTTPConn{
		ReadWriteCloser: base,
		idleTimeout:     time.Minute,
	}
	buf := make([]byte, 5)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	readDeadline := base.deadline
	if readDeadline.IsZero() {
		t.Fatal("deadline was not refreshed after read")
	}
	if _, err := conn.Write([]byte("world")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if base.deadline.Before(readDeadline) {
		t.Fatalf("deadline moved backwards: before=%s after=%s", readDeadline, base.deadline)
	}
}

func TestCloseTunnelReadWriteCloserCancelsQUICReadSide(t *testing.T) {
	conn := &cancelReadConn{}
	if err := closeTunnelReadWriteCloser(conn); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if conn.closeCount != 1 {
		t.Fatalf("Close count = %d, want 1", conn.closeCount)
	}
	if conn.cancelReadCount != 1 {
		t.Fatalf("CancelRead count = %d, want 1", conn.cancelReadCount)
	}
}

type captureRoundTripper struct {
	request *http.Request
}

func (rt *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.request = req.Clone(req.Context())
	rt.request.Header = req.Header.Clone()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

type deadlineConn struct {
	readBuffer  *bytes.Buffer
	writeBuffer bytes.Buffer
	deadline    time.Time
}

func (c *deadlineConn) Read(p []byte) (int, error) {
	return c.readBuffer.Read(p)
}

func (c *deadlineConn) Write(p []byte) (int, error) {
	return c.writeBuffer.Write(p)
}

func (c *deadlineConn) Close() error {
	return nil
}

func (c *deadlineConn) SetDeadline(t time.Time) error {
	c.deadline = t
	return nil
}

type cancelReadConn struct {
	closeCount      int
	cancelReadCount int
}

func (c *cancelReadConn) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (c *cancelReadConn) Write(p []byte) (int, error) {
	return len(p), nil
}

func (c *cancelReadConn) Close() error {
	c.closeCount++
	return nil
}

func (c *cancelReadConn) CancelRead(quic.StreamErrorCode) {
	c.cancelReadCount++
}
