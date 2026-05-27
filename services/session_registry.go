package services

import (
	"io"
	"sync"
)

type SessionCloser interface {
	CloseSession(guid string) bool
	RegisterSession(guid string, closers ...io.Closer)
	UnregisterSession(guid string)
}

type DeviceConnectionCloser interface {
	CloseDevice(deviceGuid string, reason string) bool
}

type RuntimeSessionRegistry struct {
	mu       sync.Mutex
	sessions map[string][]io.Closer
}

type RuntimeSessionStats struct {
	Active int `json:"active"`
}

func NewRuntimeSessionRegistry() *RuntimeSessionRegistry {
	return &RuntimeSessionRegistry{sessions: make(map[string][]io.Closer)}
}

func (r *RuntimeSessionRegistry) RegisterSession(guid string, closers ...io.Closer) {
	if guid == "" || len(closers) == 0 {
		return
	}
	r.mu.Lock()
	r.sessions[guid] = append(r.sessions[guid], closers...)
	r.mu.Unlock()
}

func (r *RuntimeSessionRegistry) UnregisterSession(guid string) {
	if guid == "" {
		return
	}
	r.mu.Lock()
	delete(r.sessions, guid)
	r.mu.Unlock()
}

func (r *RuntimeSessionRegistry) CloseSession(guid string) bool {
	if guid == "" {
		return false
	}
	r.mu.Lock()
	closers := append([]io.Closer(nil), r.sessions[guid]...)
	delete(r.sessions, guid)
	r.mu.Unlock()
	for _, closer := range closers {
		_ = closer.Close()
	}
	return len(closers) > 0
}

func (r *RuntimeSessionRegistry) Stats() RuntimeSessionStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RuntimeSessionStats{Active: len(r.sessions)}
}

var DefaultSessionRegistry = NewRuntimeSessionRegistry()

var runtimeDeviceConnectionCloser DeviceConnectionCloser

func RegisterDeviceConnectionCloser(closer DeviceConnectionCloser) {
	runtimeDeviceConnectionCloser = closer
}

func closeRuntimeDeviceConnection(deviceGuid string, reason string) bool {
	if runtimeDeviceConnectionCloser == nil {
		return false
	}
	return runtimeDeviceConnectionCloser.CloseDevice(deviceGuid, reason)
}
