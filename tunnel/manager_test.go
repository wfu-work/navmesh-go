package tunnel

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"navmesh-go/domains"

	"github.com/quic-go/quic-go"
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

func TestCloseDeviceIfCurrentIgnoresReplacedConnection(t *testing.T) {
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
	oldItem := m.connections[device.Guid]
	m.RegisterTCP(device, newControl, RoleControl)

	if m.CloseDeviceIfCurrent(device.Guid, oldItem, "quic idle timeout") {
		t.Fatal("old connection item should not close the replaced device")
	}
	if !m.IsOnline(device.Guid) {
		t.Fatal("device should remain online after stale close")
	}
	if !m.CloseDeviceIfCurrent(device.Guid, m.connections[device.Guid], "quic idle timeout") {
		t.Fatal("current connection item should close the device")
	}
	if m.IsOnline(device.Guid) {
		t.Fatal("device should be offline after current close")
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

func TestOpenTCPStreamClosesCurrentWhenDataChannelUnavailable(t *testing.T) {
	m := NewManager()
	deviceGuid := "device-guid"
	m.connections[deviceGuid] = &DeviceConnection{tcpData: make(chan net.Conn, 1)}

	conn, err := m.OpenTCPStream(context.Background(), deviceGuid, "127.0.0.1", 22)

	if conn != nil {
		t.Fatal("conn is not nil")
	}
	if err == nil || !strings.Contains(err.Error(), "device tunnel data channel unavailable") {
		t.Fatalf("err = %v, want device tunnel data channel unavailable", err)
	}
	if m.IsOnline(deviceGuid) {
		t.Fatal("device should be offline after missing data channel")
	}
}

func TestIsQUICIdleTimeout(t *testing.T) {
	if !isQUICIdleTimeout(&quic.IdleTimeoutError{}) {
		t.Fatal("quic idle timeout should be treated as stale connection")
	}
	if !isQUICIdleTimeout(errors.New("timeout: no recent network activity")) {
		t.Fatal("idle timeout message should be treated as stale connection")
	}
	if isQUICIdleTimeout(context.DeadlineExceeded) {
		t.Fatal("context deadline should not be treated as quic idle timeout")
	}
}

func TestShouldCloseTunnelAfterOpenTCPError(t *testing.T) {
	if !shouldCloseTunnelAfterOpenTCPError(&quic.IdleTimeoutError{}) {
		t.Fatal("quic idle timeout should close stale tunnel")
	}
	if !shouldCloseTunnelAfterOpenTCPError(context.DeadlineExceeded) {
		t.Fatal("context deadline should close stale tunnel")
	}
	if !shouldCloseTunnelAfterOpenTCPError(os.ErrDeadlineExceeded) {
		t.Fatal("os deadline should close stale tunnel")
	}
	if !shouldCloseTunnelAfterOpenTCPError(errors.New("deadline exceeded")) {
		t.Fatal("quic stream deadline message should close stale tunnel")
	}
	if shouldCloseTunnelAfterOpenTCPError(errors.New("dial tcp 127.0.0.1:22: i/o timeout")) {
		t.Fatal("local target timeout message should not close the tunnel")
	}
}
