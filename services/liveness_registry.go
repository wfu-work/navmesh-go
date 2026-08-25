package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	defaultLivenessFlushInterval = 30 * time.Second
	livenessFlushBatchSize       = 300
)

// LivenessRegistry coalesces frequent tunnel activity into bounded database
// writes. The latest timestamp remains dirty until a successful flush acks it.
type LivenessRegistry struct {
	mu      sync.Mutex
	flushMu sync.Mutex
	dirty   map[string]int64
	db      *gorm.DB
}

func NewLivenessRegistry(db *gorm.DB) *LivenessRegistry {
	return &LivenessRegistry{dirty: make(map[string]int64), db: db}
}

func (r *LivenessRegistry) WithDB(db *gorm.DB) *LivenessRegistry {
	if r == nil {
		return NewLivenessRegistry(db)
	}
	r.mu.Lock()
	r.db = db
	r.mu.Unlock()
	return r
}

func (r *LivenessRegistry) Touch(deviceGuid string, at int64) {
	if r == nil {
		return
	}
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return
	}
	if at <= 0 {
		at = domains.NowMilli()
	}
	r.mu.Lock()
	if at > r.dirty[deviceGuid] {
		r.dirty[deviceGuid] = at
	}
	r.mu.Unlock()
}

func (r *LivenessRegistry) LastSeen(deviceGuid string) int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dirty[strings.TrimSpace(deviceGuid)]
}

func (r *LivenessRegistry) DirtyCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.dirty)
}

func (r *LivenessRegistry) snapshot() (map[string]int64, *gorm.DB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make(map[string]int64, len(r.dirty))
	for guid, at := range r.dirty {
		items[guid] = at
	}
	db := r.db
	if db == nil {
		db = global.NAV_DB
	}
	return items, db
}

func (r *LivenessRegistry) ack(snapshot map[string]int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for guid, flushedAt := range snapshot {
		if r.dirty[guid] <= flushedAt {
			delete(r.dirty, guid)
		}
	}
}

func (r *LivenessRegistry) MarkPersisted(deviceGuid string, at int64) {
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return
	}
	r.ack(map[string]int64{deviceGuid: at})
}

func (r *LivenessRegistry) ForgetDevice(deviceGuid string) {
	if r == nil {
		return
	}
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return
	}
	r.mu.Lock()
	delete(r.dirty, deviceGuid)
	r.mu.Unlock()
}

func (r *LivenessRegistry) Flush() error {
	if r == nil {
		return nil
	}
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	snapshot, db := r.snapshot()
	if len(snapshot) == 0 {
		return nil
	}
	if db == nil {
		return errors.New("liveness database unavailable")
	}
	guids := make([]string, 0, len(snapshot))
	for guid := range snapshot {
		guids = append(guids, guid)
	}
	now := domains.NowMilli()
	err := db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(guids); start += livenessFlushBatchSize {
			end := start + livenessFlushBatchSize
			if end > len(guids) {
				end = len(guids)
			}
			batch := guids[start:end]
			if err := tx.Model(&domains.Device{}).
				Where("guid IN ? AND status <> ?", batch, domains.DeviceStatusDisabled).
				Updates(map[string]any{
					"status":         domains.DeviceStatusOnline,
					"last_seen_time": now,
					"update_time":    now,
				}).Error; err != nil {
				return err
			}
			if err := tx.Model(&domains.DeviceConnection{}).
				Where("device_guid IN ? AND status = ?", batch, int(domains.StatusEnabled)).
				Updates(map[string]any{
					"last_active_time": now,
					"update_time":      now,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.ack(snapshot)
	return nil
}

func (r *LivenessRegistry) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultLivenessFlushInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := r.Flush(); err != nil && global.NAV_LOG != nil {
				global.NAV_LOG.Warn("flush device liveness during shutdown failed", zap.Error(err))
			}
			return
		case <-ticker.C:
			if err := r.Flush(); err != nil && global.NAV_LOG != nil {
				global.NAV_LOG.Warn("flush device liveness failed", zap.Error(err))
			}
		}
	}
}

var DefaultLivenessRegistry = NewLivenessRegistry(nil)

type TelemetrySampler struct {
	mu       sync.Mutex
	lastSave map[string]int64
}

func NewTelemetrySampler() *TelemetrySampler {
	return &TelemetrySampler{lastSave: make(map[string]int64)}
}

func (s *TelemetrySampler) ShouldPersist(deviceGuid string, now int64, interval time.Duration) bool {
	if s == nil {
		return true
	}
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" || interval <= 0 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	last := s.lastSave[deviceGuid]
	if last > 0 && now-last < interval.Milliseconds() {
		return false
	}
	s.lastSave[deviceGuid] = now
	return true
}

func (s *TelemetrySampler) MarkPersisted(deviceGuid string, now int64) {
	if s == nil || strings.TrimSpace(deviceGuid) == "" {
		return
	}
	s.mu.Lock()
	s.lastSave[strings.TrimSpace(deviceGuid)] = now
	s.mu.Unlock()
}

func (s *TelemetrySampler) Forget(deviceGuid string, reservedAt int64) {
	if s == nil || strings.TrimSpace(deviceGuid) == "" {
		return
	}
	deviceGuid = strings.TrimSpace(deviceGuid)
	s.mu.Lock()
	if s.lastSave[deviceGuid] == reservedAt {
		delete(s.lastSave, deviceGuid)
	}
	s.mu.Unlock()
}

func (s *TelemetrySampler) ForgetDevice(deviceGuid string) {
	if s == nil {
		return
	}
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return
	}
	s.mu.Lock()
	delete(s.lastSave, deviceGuid)
	s.mu.Unlock()
}

var DefaultTelemetrySampler = NewTelemetrySampler()
