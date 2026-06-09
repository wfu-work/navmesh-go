package tunnel

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"navmesh-go/domains"
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

func TestUnregisterTCPControlIgnoresReplacedConnection(t *testing.T) {
	m := NewManager()
	device := domains.Device{Sncode: "mgrl11", Alias: "MGRL11"}
	device.Guid = "device-guid"

	oldControl, oldPeer := net.Pipe()
	defer oldControl.Close()
	defer oldPeer.Close()
	newControl, newPeer := net.Pipe()
	defer newControl.Close()
	defer newPeer.Close()

	m.RegisterTCP(device, oldControl, RoleControl)
	m.RegisterTCP(device, newControl, RoleControl)

	if m.UnregisterTCPControl(device.Guid, oldControl) {
		t.Fatal("old control connection should not unregister the replaced device")
	}
	if !m.IsOnline(device.Guid) {
		t.Fatal("device should remain online after stale unregister")
	}
	if !m.UnregisterTCPControl(device.Guid, newControl) {
		t.Fatal("current control connection should unregister the device")
	}
	if m.IsOnline(device.Guid) {
		t.Fatal("device should be offline after current control unregister")
	}
}

func TestOpenTCPOverDataConnDeadlineCoversAckWait(t *testing.T) {
	m := NewManager()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	item := &DeviceConnection{
		tcpControl: server,
		tcpData:    make(chan net.Conn, 1),
	}
	item.tcpData <- server

	readDone := make(chan error, 1)
	go func() {
		_, err := readFrameFromReader(client)
		readDone <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	conn, err := m.openTCPOverDataConn(ctx, item, "127.0.0.1", 22)

	if conn != nil {
		t.Fatal("conn is not nil")
	}
	if err == nil {
		t.Fatal("err is nil")
	}
	if time.Since(start) > 250*time.Millisecond {
		t.Fatal("ack wait should respect context deadline")
	}
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("client side did not receive open_tcp frame")
	}
}
