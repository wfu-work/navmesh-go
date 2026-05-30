package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"navmesh-go/domains"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
)

type DeviceConnectionInfo struct {
	DeviceGuid     string `json:"deviceGuid"`
	SnCode         string `json:"sncode"`
	Alias          string `json:"alias"`
	RemoteAddr     string `json:"remoteAddr"`
	ConnectedTime  int64  `json:"connectedTime"`
	LastActiveTime int64  `json:"lastActiveTime"`
}

type DeviceConnection struct {
	info       DeviceConnectionInfo
	control    *quic.Conn
	data       *quic.Conn
	tcpControl net.Conn
	tcpData    chan net.Conn
}

type Manager struct {
	mu          sync.RWMutex
	connections map[string]*DeviceConnection
}

var DefaultManager = NewManager()

func NewManager() *Manager {
	return &Manager{connections: make(map[string]*DeviceConnection)}
}

func (m *Manager) Register(device domains.Device, conn *quic.Conn) {
	m.RegisterQUIC(device, conn, RoleLegacy)
}

func (m *Manager) RegisterQUIC(device domains.Device, conn *quic.Conn, role string) {
	now := domains.NowMilli()
	info := DeviceConnectionInfo{
		DeviceGuid:     device.Guid,
		SnCode:         device.Sncode,
		Alias:          device.Alias,
		RemoteAddr:     conn.RemoteAddr().String(),
		ConnectedTime:  now,
		LastActiveTime: now,
	}
	m.mu.Lock()
	item := m.connections[device.Guid]
	if item == nil {
		item = &DeviceConnection{info: info, tcpData: make(chan net.Conn, 64)}
		m.connections[device.Guid] = item
	}
	item.info = info
	if role == RoleData {
		item.data = conn
	} else {
		item.control = conn
		if role == RoleLegacy {
			item.data = conn
		}
	}
	m.mu.Unlock()
	if role == RoleData {
		return
	}
	_ = global.NAV_DB.Create(&domains.DeviceConnection{
		DeviceGuid:     device.Guid,
		ConnectionID:   uuid.NewString(),
		Protocol:       TransportQUIC,
		RemoteAddr:     info.RemoteAddr,
		Status:         int(domains.StatusEnabled),
		ConnectedTime:  now,
		LastActiveTime: now,
		CreateTime:     now,
		UpdateTime:     now,
	}).Error
}

func (m *Manager) RegisterTCP(device domains.Device, conn net.Conn, role string) {
	now := domains.NowMilli()
	info := DeviceConnectionInfo{
		DeviceGuid:     device.Guid,
		SnCode:         device.Sncode,
		Alias:          device.Alias,
		RemoteAddr:     conn.RemoteAddr().String(),
		ConnectedTime:  now,
		LastActiveTime: now,
	}
	m.mu.Lock()
	item := m.connections[device.Guid]
	if item == nil {
		item = &DeviceConnection{info: info, tcpData: make(chan net.Conn, 64)}
		m.connections[device.Guid] = item
	}
	item.info = info
	if role == RoleData {
		select {
		case item.tcpData <- conn:
		default:
			_ = conn.Close()
		}
		m.mu.Unlock()
		return
	}
	item.tcpControl = conn
	m.mu.Unlock()
	_ = global.NAV_DB.Create(&domains.DeviceConnection{
		DeviceGuid:     device.Guid,
		ConnectionID:   uuid.NewString(),
		Protocol:       TransportTCP,
		RemoteAddr:     info.RemoteAddr,
		Status:         int(domains.StatusEnabled),
		ConnectedTime:  now,
		LastActiveTime: now,
		CreateTime:     now,
		UpdateTime:     now,
	}).Error
}

func (m *Manager) Unregister(deviceGuid string) {
	m.mu.Lock()
	item := m.connections[deviceGuid]
	delete(m.connections, deviceGuid)
	m.mu.Unlock()
	closeDeviceItem(item, "device unregister")
	now := domains.NowMilli()
	_ = global.NAV_DB.Model(&domains.DeviceConnection{}).
		Where("device_guid = ? AND status = ?", deviceGuid, int(domains.StatusEnabled)).
		Updates(map[string]any{"status": domains.StatusDisabled, "update_time": now}).Error
}

func (m *Manager) UnregisterQUICData(deviceGuid string, conn *quic.Conn) {
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" || conn == nil {
		return
	}
	m.mu.Lock()
	if item := m.connections[deviceGuid]; item != nil && item.data == conn {
		item.data = nil
	}
	m.mu.Unlock()
}

func (m *Manager) Touch(deviceGuid string) {
	now := domains.NowMilli()
	m.mu.Lock()
	if item := m.connections[deviceGuid]; item != nil {
		item.info.LastActiveTime = now
	}
	m.mu.Unlock()
	_ = global.NAV_DB.Model(&domains.DeviceConnection{}).
		Where("device_guid = ? AND status = ?", deviceGuid, int(domains.StatusEnabled)).
		Updates(map[string]any{"last_active_time": now, "update_time": now}).Error
}

func (m *Manager) List() []DeviceConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]DeviceConnectionInfo, 0, len(m.connections))
	for _, item := range m.connections {
		items = append(items, item.info)
	}
	return items
}

func (m *Manager) IsOnline(deviceGuid string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[deviceGuid] != nil
}

func (m *Manager) CloseDevice(deviceGuid string, reason string) bool {
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return false
	}
	if reason == "" {
		reason = "device connection closed"
	}
	m.mu.Lock()
	item := m.connections[deviceGuid]
	delete(m.connections, deviceGuid)
	m.mu.Unlock()
	if item == nil {
		return false
	}
	closeDeviceItem(item, reason)
	now := domains.NowMilli()
	_ = global.NAV_DB.Model(&domains.DeviceConnection{}).
		Where("device_guid = ? AND status = ?", deviceGuid, int(domains.StatusEnabled)).
		Updates(map[string]any{"status": domains.StatusDisabled, "update_time": now}).Error
	return true
}

func (m *Manager) OpenTCPStream(ctx context.Context, deviceGuid, targetHost string, targetPort int) (io.ReadWriteCloser, error) {
	m.mu.RLock()
	item := m.connections[deviceGuid]
	m.mu.RUnlock()
	if item == nil {
		return nil, errors.New("device tunnel offline")
	}
	if conn, err := m.openTCPOverDataConn(ctx, item, targetHost, targetPort); err == nil {
		m.Touch(deviceGuid)
		return conn, nil
	}
	conn := item.data
	if conn != nil {
		stream, err := m.openTCPOverQUICConn(ctx, conn, targetHost, targetPort)
		if err == nil {
			m.Touch(deviceGuid)
			return stream, nil
		}
		return nil, err
	}
	return nil, errors.New("device tunnel data channel unavailable")
}

func (m *Manager) openTCPOverQUICConn(ctx context.Context, conn *quic.Conn, targetHost string, targetPort int) (io.ReadWriteCloser, error) {
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	frame := Frame{Type: FrameTypeOpenTCP, RequestID: uuid.NewString(), TargetHost: targetHost, TargetPort: targetPort}
	payload, err := EncodeFrame(frame)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	if _, err := stream.Write(payload); err != nil {
		_ = stream.Close()
		return nil, err
	}
	ack, err := readFrame(stream)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	if ack.Type == FrameTypeError || !ack.OK {
		_ = stream.Close()
		if ack.Message == "" {
			ack.Message = "open tcp rejected"
		}
		return nil, errors.New(ack.Message)
	}
	if ack.Type != FrameTypeOpenTCPAck {
		_ = stream.Close()
		return nil, errors.New("invalid open tcp ack")
	}
	return stream, nil
}

func (m *Manager) openTCPOverDataConn(ctx context.Context, item *DeviceConnection, targetHost string, targetPort int) (io.ReadWriteCloser, error) {
	if item == nil || item.tcpData == nil {
		return nil, errors.New("tcp data channel unavailable")
	}
	if item.tcpControl == nil && len(item.tcpData) == 0 {
		return nil, errors.New("tcp data channel unavailable")
	}
	select {
	case conn := <-item.tcpData:
		frame := Frame{Type: FrameTypeOpenTCP, RequestID: uuid.NewString(), TargetHost: targetHost, TargetPort: targetPort}
		payload, err := EncodeFrame(frame)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		if _, err := conn.Write(payload); err != nil {
			_ = conn.Close()
			return nil, err
		}
		ack, err := readFrameFromReader(conn)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		if ack.Type == FrameTypeError || !ack.OK {
			_ = conn.Close()
			if ack.Message == "" {
				ack.Message = "open tcp rejected"
			}
			return nil, errors.New(ack.Message)
		}
		if ack.Type != FrameTypeOpenTCPAck {
			_ = conn.Close()
			return nil, errors.New("invalid open tcp ack")
		}
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *Manager) SetOffline(deviceGuid string) {
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return
	}
	now := domains.NowMilli()
	_ = global.NAV_DB.Model(&domains.Device{}).
		Where("guid = ? AND status != ?", deviceGuid, domains.DeviceStatusDisabled).
		Updates(map[string]any{"status": domains.DeviceStatusOffline, "update_time": now}).Error
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for guid, item := range m.connections {
		closeDeviceItem(item, "server shutting down")
		global.NAV_LOG.Debug("close tunnel connection", zap.String("deviceGuid", guid))
	}
	m.connections = make(map[string]*DeviceConnection)
}

func closeDeviceItem(item *DeviceConnection, reason string) {
	if item == nil {
		return
	}
	if item.control != nil {
		_ = item.control.CloseWithError(0, reason)
	}
	if item.data != nil && item.data != item.control {
		_ = item.data.CloseWithError(0, reason)
	}
	if item.tcpControl != nil {
		_ = item.tcpControl.Close()
	}
	if item.tcpData != nil {
		for {
			select {
			case conn := <-item.tcpData:
				_ = conn.Close()
			default:
				return
			}
		}
	}
}

func contextWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return context.WithTimeout(parent, timeout)
}
