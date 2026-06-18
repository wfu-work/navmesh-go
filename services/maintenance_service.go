package services

import (
	"context"
	"strconv"
	"strings"
	"time"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
)

type MaintenanceService struct{}

type RetentionCleanupResult struct {
	AuditLogs         int64 `json:"auditLogs"`
	HTTPAccessLogs    int64 `json:"httpAccessLogs"`
	TunnelSessions    int64 `json:"tunnelSessions"`
	DeviceHeartbeats  int64 `json:"deviceHeartbeats"`
	DeviceTrafficDays int64 `json:"deviceTrafficDays"`
	DeviceConnections int64 `json:"deviceConnections"`
}

func (s MaintenanceService) StartRetentionCleaner(ctx context.Context) {
	if !settingBool("retention_cleanup_enabled", true) {
		global.NAV_LOG.Info("navmesh retention cleaner disabled")
		return
	}
	interval := settingDuration("retention_cleanup_interval", 24*time.Hour)
	if interval < time.Minute {
		interval = time.Minute
	}
	go func() {
		s.cleanupOnce("startup")
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanupOnce("ticker")
			}
		}
	}()
}

func (s MaintenanceService) CleanupRetention() RetentionCleanupResult {
	now := domains.NowMilli()
	return RetentionCleanupResult{
		AuditLogs:         deleteBefore(&domains.AuditLog{}, "create_time", now, settingInt("audit_retention_days", 90)),
		HTTPAccessLogs:    deleteBefore(&domains.HTTPAccessLog{}, "create_time", now, settingInt("http_access_retention_days", 30)),
		TunnelSessions:    deleteBefore(&domains.TunnelSession{}, "start_time", now, settingInt("session_retention_days", 90)),
		DeviceHeartbeats:  deleteBefore(&domains.DeviceHeartbeat{}, "create_time", now, settingInt("heartbeat_retention_days", 7)),
		DeviceTrafficDays: deleteBefore(&domains.DeviceTrafficDaily{}, "last_seen_time", now, settingInt("traffic_daily_retention_days", 370)),
		DeviceConnections: deleteBefore(&domains.DeviceConnection{}, "create_time", now, settingInt("device_connection_retention_days", 30)),
	}
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

func deleteBefore(model any, column string, now int64, days int) int64 {
	if days <= 0 {
		return 0
	}
	cutoff := now - int64(days)*24*60*60*1000
	tx := global.NAV_DB.Where(column+" < ?", cutoff).Delete(model)
	if tx.Error != nil {
		global.NAV_LOG.Warn("navmesh retention delete failed", zap.String("column", column), zap.Error(tx.Error))
		return 0
	}
	return tx.RowsAffected
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
