package tunnel

import (
	"context"
	"errors"
	"io"
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
	info DeviceConnectionInfo
	conn *quic.Conn
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
	now := domains.NowMilli()
	info := DeviceConnectionInfo{
		DeviceGuid:     device.Guid,
		SnCode:         device.SnCode,
		Alias:          device.Alias,
		RemoteAddr:     conn.RemoteAddr().String(),
		ConnectedTime:  now,
		LastActiveTime: now,
	}
	m.mu.Lock()
	m.connections[device.Guid] = &DeviceConnection{info: info, conn: conn}
	m.mu.Unlock()
	_ = global.NAV_DB.Create(&domains.DeviceConnection{
		DeviceGuid:     device.Guid,
		ConnectionID:   uuid.NewString(),
		Protocol:       "quic",
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
	delete(m.connections, deviceGuid)
	m.mu.Unlock()
	now := domains.NowMilli()
	_ = global.NAV_DB.Model(&domains.DeviceConnection{}).
		Where("device_guid = ? AND status = ?", deviceGuid, int(domains.StatusEnabled)).
		Updates(map[string]any{"status": domains.StatusDisabled, "update_time": now}).Error
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

func (m *Manager) OpenTCPStream(ctx context.Context, deviceGuid, targetHost string, targetPort int) (io.ReadWriteCloser, error) {
	m.mu.RLock()
	item := m.connections[deviceGuid]
	m.mu.RUnlock()
	if item == nil {
		return nil, errors.New("device tunnel offline")
	}
	stream, err := item.conn.OpenStreamSync(ctx)
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
	m.Touch(deviceGuid)
	return stream, nil
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
		if err := item.conn.CloseWithError(0, "server shutting down"); err != nil {
			global.NAV_LOG.Warn("close tunnel connection failed", zap.String("deviceGuid", guid), zap.Error(err))
		}
	}
	m.connections = make(map[string]*DeviceConnection)
}

func contextWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return context.WithTimeout(parent, timeout)
}
