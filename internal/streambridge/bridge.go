// Package streambridge contains the common bidirectional stream forwarding
// primitives used by the SSH and TCP gateways.
package streambridge

import (
	"io"
	"net"
	"strings"
	"time"
)

// Bridge copies bytes in both directions until both sides finish. Each side
// is closed in the direction it no longer needs, which preserves half-close
// semantics for TCP-like connections. The returned values are bytes written to
// b and a respectively.
func Bridge(a io.ReadWriteCloser, b io.ReadWriteCloser, idleTimeout time.Duration) (int64, int64) {
	aCounter := &countingWriter{w: a}
	bCounter := &countingWriter{w: b}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = CopyWithIdleDeadline(bCounter, a, idleTimeout)
		_ = b.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = CopyWithIdleDeadline(aCounter, b, idleTimeout)
		_ = a.Close()
		done <- struct{}{}
	}()
	<-done
	<-done
	return bCounter.n, aCounter.n
}

// CopyWithIdleDeadline copies src to dst and refreshes read/write deadlines
// after every successful operation. A non-positive timeout disables deadline
// handling and uses io.Copy directly.
func CopyWithIdleDeadline(dst io.Writer, src io.Reader, idleTimeout time.Duration) (int64, error) {
	if idleTimeout <= 0 {
		return io.Copy(dst, src)
	}
	buf := make([]byte, 32*1024)
	var written int64
	for {
		setReadDeadline(src, time.Now().Add(idleTimeout))
		nr, er := src.Read(buf)
		if nr > 0 {
			setWriteDeadline(dst, time.Now().Add(idleTimeout))
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			return written, er
		}
	}
}

// NormalizeIP normalizes an IP literal while preserving the previous gateway
// behavior for non-IP host strings.
func NormalizeIP(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return value
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func setReadDeadline(value any, deadline time.Time) {
	if conn, ok := value.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = conn.SetReadDeadline(deadline)
	}
}

func setWriteDeadline(value any, deadline time.Time) {
	if conn, ok := value.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = conn.SetWriteDeadline(deadline)
	}
}
