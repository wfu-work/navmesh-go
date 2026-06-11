package main

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/quic-go/quic-go"
)

func TestBridgePrefersHalfClose(t *testing.T) {
	a := newHalfCloseBufferConn("from-a")
	b := newHalfCloseBufferConn("from-b")

	bridge(a, b)

	if got := a.String(); got != "from-b" {
		t.Fatalf("a received %q, want from-b", got)
	}
	if got := b.String(); got != "from-a" {
		t.Fatalf("b received %q, want from-a", got)
	}
	if a.closeWriteCount == 0 || b.closeWriteCount == 0 {
		t.Fatalf("CloseWrite was not called: a=%d b=%d", a.closeWriteCount, b.closeWriteCount)
	}
	if a.closeReadCount == 0 || b.closeReadCount == 0 {
		t.Fatalf("CloseRead was not called: a=%d b=%d", a.closeReadCount, b.closeReadCount)
	}
	if a.closeCount == 0 || b.closeCount == 0 {
		t.Fatalf("Close was not called: a=%d b=%d", a.closeCount, b.closeCount)
	}
}

func TestCloseReadCancelsQUICReadSide(t *testing.T) {
	conn := &quicCancelReadConn{}
	closeRead(conn)
	if conn.cancelReadCount != 1 {
		t.Fatalf("CancelRead count = %d, want 1", conn.cancelReadCount)
	}
	if conn.closeCount != 0 {
		t.Fatalf("Close count = %d, want 0", conn.closeCount)
	}
}

func TestHeartbeatFailedRequiresTunnelHeartbeat(t *testing.T) {
	err := errors.New("deadline exceeded")
	tests := []struct {
		name    string
		quicErr error
		httpErr error
		want    bool
	}{
		{name: "both ok", want: false},
		{name: "quic failed http ok", quicErr: err, want: true},
		{name: "quic ok http failed", httpErr: err, want: false},
		{name: "both failed", quicErr: err, httpErr: err, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := heartbeatFailed(tt.quicErr, tt.httpErr); got != tt.want {
				t.Fatalf("heartbeatFailed() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestNormalizeTransport(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "", want: clientTransportAuto},
		{value: "AUTO", want: clientTransportAuto},
		{value: "quic", want: "quic"},
		{value: "tcp", want: "tcp"},
		{value: "bad", want: clientTransportAuto},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := normalizeTransport(tt.value); got != tt.want {
				t.Fatalf("normalizeTransport(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestNormalizeIPv4RejectsIPv6(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "plain ipv4", value: " 203.0.113.10 ", want: "203.0.113.10"},
		{name: "ipv4 with port", value: "203.0.113.10:8080", want: "203.0.113.10"},
		{name: "ipv6", value: "240e:36d:389:5a0::1", want: ""},
		{name: "ipv6 with brackets", value: "[240e:36d:389:5a0::1]", want: ""},
		{name: "invalid", value: "not-an-ip", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeIPv4(tt.value); got != tt.want {
				t.Fatalf("normalizeIPv4(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestIsPublicIPv4RejectsIPv6AndPrivateIP(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "8.8.8.8", want: true},
		{value: "192.168.1.10", want: false},
		{value: "240e:36d:389:5a0::1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := isPublicIPv4(tt.value); got != tt.want {
				t.Fatalf("isPublicIPv4(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}

func TestDefaultTCPDataChannelsSupportsResourceHeavyPages(t *testing.T) {
	if defaultTCPDataChannels < 32 {
		t.Fatalf("defaultTCPDataChannels = %d, want at least 32", defaultTCPDataChannels)
	}
}

type halfCloseBufferConn struct {
	mu              sync.Mutex
	readBuffer      *bytes.Buffer
	writeBuffer     bytes.Buffer
	closeWriteCount int
	closeReadCount  int
	closeCount      int
}

func newHalfCloseBufferConn(read string) *halfCloseBufferConn {
	return &halfCloseBufferConn{readBuffer: bytes.NewBufferString(read)}
}

func (c *halfCloseBufferConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readBuffer.Len() == 0 {
		return 0, io.EOF
	}
	return c.readBuffer.Read(p)
}

func (c *halfCloseBufferConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeBuffer.Write(p)
}

func (c *halfCloseBufferConn) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	return nil
}

func (c *halfCloseBufferConn) CloseWrite() error {
	c.mu.Lock()
	c.closeWriteCount++
	c.mu.Unlock()
	return nil
}

func (c *halfCloseBufferConn) CloseRead() error {
	c.mu.Lock()
	c.closeReadCount++
	c.mu.Unlock()
	return nil
}

func (c *halfCloseBufferConn) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeBuffer.String()
}

type quicCancelReadConn struct {
	closeCount      int
	cancelReadCount int
}

func (c *quicCancelReadConn) Close() error {
	c.closeCount++
	return nil
}

func (c *quicCancelReadConn) CancelRead(quic.StreamErrorCode) {
	c.cancelReadCount++
}
