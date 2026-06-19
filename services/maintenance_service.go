package services

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"navmesh-go/domains"

	"github.com/robfig/cron/v3"
	"github.com/wfu-work/nav-common-go-lib/global"
	"github.com/wfu-work/nav-common-go-lib/scheduleds"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type MaintenanceService struct{}

const (
	retentionCleanerCronName = "NavMeshRetention"
	retentionCleanerTaskName = "retention_cleanup"
	retentionCleanerSpec     = "@every 1m"
	retentionMinimumInterval = time.Minute
)

var (
	retentionCleanerLastRun atomic.Int64
	retentionCleanerRunning atomic.Bool
)

type RetentionCleanupResult struct {
	AuditLogs         int64 `json:"auditLogs"`
	HTTPAccessLogs    int64 `json:"httpAccessLogs"`
	TunnelSessions    int64 `json:"tunnelSessions"`
	DeviceHeartbeats  int64 `json:"deviceHeartbeats"`
	DeviceTrafficDays int64 `json:"deviceTrafficDays"`
	DeviceConnections int64 `json:"deviceConnections"`
}

func (s MaintenanceService) RegisterRetentionCleaner(timer scheduleds.Timer, options []cron.Option) {
	if timer == nil {
		global.NAV_LOG.Warn("navmesh retention cleaner timer is nil")
		return
	}
	timer.RemoveTaskByName(retentionCleanerCronName, retentionCleanerTaskName)
	if _, err := timer.AddTaskByFunc(retentionCleanerCronName, retentionCleanerSpec, func() {
		s.cleanupDue("scheduled")
	}, retentionCleanerTaskName, options...); err != nil {
		global.NAV_LOG.Warn("register navmesh retention cleaner failed", zap.Error(err))
		return
	}
	global.NAV_LOG.Info("register navmesh retention cleaner", zap.String("spec", retentionCleanerSpec))
	s.cleanupDue("startup")
}

func (s MaintenanceService) StopRetentionCleaner(timer scheduleds.Timer) {
	if timer == nil {
		return
	}
	timer.Clear(retentionCleanerCronName)
}

func (s MaintenanceService) CleanupRetention() RetentionCleanupResult {
	now := domains.NowMilli()
	return RetentionCleanupResult{
		AuditLogs:         deleteBefore(&domains.AuditLog{}, "create_time", now, settingInt("audit_retention_days", 90)),
		HTTPAccessLogs:    deleteBefore(&domains.HTTPAccessLog{}, "create_time", now, settingInt("http_access_retention_days", 30)),
		TunnelSessions:    deleteBefore(&domains.TunnelSession{}, "end_time", now, settingInt("session_retention_days", 90), closedRows),
		DeviceHeartbeats:  deleteBefore(&domains.DeviceHeartbeat{}, "create_time", now, settingInt("heartbeat_retention_days", 7)),
		DeviceTrafficDays: deleteBefore(&domains.DeviceTrafficDaily{}, "last_seen_time", now, settingInt("traffic_daily_retention_days", 370)),
		DeviceConnections: deleteBefore(&domains.DeviceConnection{}, "update_time", now, settingInt("device_connection_retention_days", 30), closedRows),
	}
}

func (s MaintenanceService) cleanupDue(reason string) {
	if !settingBool("retention_cleanup_enabled", true) {
		return
	}
	interval := retentionCleanupInterval()
	now := domains.NowMilli()
	lastRun := retentionCleanerLastRun.Load()
	if lastRun > 0 && now-lastRun < interval.Milliseconds() {
		return
	}
	if !retentionCleanerRunning.CompareAndSwap(false, true) {
		return
	}
	defer retentionCleanerRunning.Store(false)
	now = domains.NowMilli()
	lastRun = retentionCleanerLastRun.Load()
	if lastRun > 0 && now-lastRun < interval.Milliseconds() {
		return
	}
	s.cleanupOnce(reason)
	retentionCleanerLastRun.Store(now)
}

func (s MaintenanceService) cleanupOnce(reason string) {
	result := s.CleanupRetention()
	global.NAV_LOG.Info("navmesh retention cleanup finished",
		zap.String("reason", reason),
		zap.Int64("auditLogs", result.AuditLogs),
		zap.Int64("httpAccessLogs", result.HTTPAccessLogs),
		zap.Int64("tunnelSessions", result.TunnelSessions),
		zap.Int64("deviceHeartbeats", result.DeviceHeartbeats),
		zap.Int64("deviceTrafficDays", result.DeviceTrafficDays),
		zap.Int64("deviceConnections", result.DeviceConnections),
	)
}

func deleteBefore(model any, column string, now int64, days int, scopes ...func(*gorm.DB) *gorm.DB) int64 {
	if days <= 0 {
		return 0
	}
	cutoff := now - int64(days)*24*60*60*1000
	db := global.NAV_DB.Where(column+" < ?", cutoff)
	for _, scope := range scopes {
		db = scope(db)
	}
	tx := db.Delete(model)
	if tx.Error != nil {
		global.NAV_LOG.Warn("navmesh retention delete failed", zap.String("column", column), zap.Error(tx.Error))
		return 0
	}
	return tx.RowsAffected
}

func closedRows(db *gorm.DB) *gorm.DB {
	return db.Where("status = ?", int(domains.StatusDisabled))
}

func retentionCleanupInterval() time.Duration {
	interval := settingDuration("retention_cleanup_interval", 24*time.Hour)
	if interval < retentionMinimumInterval {
		return retentionMinimumInterval
	}
	return interval
}

func settingBool(key string, def bool) bool {
	value := strings.ToLower(strings.TrimSpace(settingValue(key, "")))
	if value == "" {
		return def
	}
	return value == "true" || value == "1" || value == "yes" || value == "on"
}

func settingInt(key string, def int) int {
	value := strings.TrimSpace(settingValue(key, ""))
	if value == "" {
		return def
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return def
	}
	return n
}

func settingDuration(key string, def time.Duration) time.Duration {
	value := strings.TrimSpace(settingValue(key, ""))
	if value == "" {
		return def
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return def
	}
	return duration
}

func settingValue(key, def string) string {
	var row domains.Setting
	if err := global.NAV_DB.Where("key = ?", key).First(&row).Error; err == nil && strings.TrimSpace(row.Value) != "" {
		return strings.TrimSpace(row.Value)
	}
	return def
}
