package services

import (
	"io"
	"sync"
	"time"
)

const forcedSessionMarkerTTL = time.Hour

type SessionCloser interface {
	CloseSession(guid string) bool
	RegisterSession(guid string, closers ...io.Closer)
	UnregisterSession(guid string)
}

type DeviceConnectionCloser interface {
	CloseDevice(deviceGuid string, reason string) bool
}

type RuntimeSessionRegistry struct {
	mu            sync.Mutex
	sessions      map[string][]io.Closer
	forcedSession map[string]time.Time
}

type RuntimeSessionStats struct {
	Active int `json:"active"`
}

func NewRuntimeSessionRegistry() *RuntimeSessionRegistry {
	return &RuntimeSessionRegistry{
		sessions:      make(map[string][]io.Closer),
		forcedSession: make(map[string]time.Time),
	}
}

func (r *RuntimeSessionRegistry) RegisterSession(guid string, closers ...io.Closer) {
	if guid == "" || len(closers) == 0 {
		return
	}
	r.mu.Lock()
	r.pruneForcedSessionsLocked(time.Now())
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
	if len(closers) > 0 {
		now := time.Now()
		r.pruneForcedSessionsLocked(now)
		r.forcedSession[guid] = now
	}
	r.mu.Unlock()
	for _, closer := range closers {
		_ = closer.Close()
	}
	return len(closers) > 0
}

// ConsumeForceClosed returns whether an active runtime session was explicitly
// closed by the control plane. The marker is consumed once by the gateway's
// session finalizer, avoiding a database read on every normal session close.
func (r *RuntimeSessionRegistry) ConsumeForceClosed(guid string) bool {
	if guid == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneForcedSessionsLocked(time.Now())
	_, forced := r.forcedSession[guid]
	delete(r.forcedSession, guid)
	return forced
}

func (r *RuntimeSessionRegistry) pruneForcedSessionsLocked(now time.Time) {
	cutoff := now.Add(-forcedSessionMarkerTTL)
	for guid, markedAt := range r.forcedSession {
		if markedAt.Before(cutoff) {
			delete(r.forcedSession, guid)
		}
	}
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
