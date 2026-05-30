package main

import (
	"bytes"
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
