package tunnel

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestOpenTCPOverDataConnSkipsEmptyPoolForQUIC(t *testing.T) {
	m := NewManager()
	item := &DeviceConnection{tcpData: make(chan net.Conn, 1)}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	conn, err := m.openTCPOverDataConn(ctx, item, "127.0.0.1", 80)

	if conn != nil {
		t.Fatal("conn is not nil")
	}
	if err == nil || !strings.Contains(err.Error(), "tcp data channel unavailable") {
		t.Fatalf("err = %v, want tcp data channel unavailable", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("empty QUIC-side tcp data pool should not block")
	}
}

func TestOpenTCPOverDataConnWaitsForTCPPool(t *testing.T) {
	m := NewManager()
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	item := &DeviceConnection{
		tcpControl: left,
		tcpData:    make(chan net.Conn, 1),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	conn, err := m.openTCPOverDataConn(ctx, item, "127.0.0.1", 80)

	if conn != nil {
		t.Fatal("conn is not nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context deadline exceeded", err)
	}
}
