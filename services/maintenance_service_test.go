package services

import (
	"testing"
	"time"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCleanupRetentionDeletesExpiredHistoryAndKeepsActiveRows(t *testing.T) {
	db := setupMaintenanceTestDB(t)
	now := domains.NowMilli()
	old := now - 48*time.Hour.Milliseconds()
	recent := now - 6*time.Hour.Milliseconds()
	seedRetentionSettings(t, db, 1)

	rows := []any{
		&domains.AuditLog{Actor: "old", CreateTime: old},
		&domains.AuditLog{Actor: "recent", CreateTime: recent},
		&domains.HTTPAccessLog{Host: "old.example.com", CreateTime: old},
		&domains.HTTPAccessLog{Host: "recent.example.com", CreateTime: recent},
		&domains.DeviceHeartbeat{DeviceGuid: "old-heartbeat", CreateTime: old},
		&domains.DeviceHeartbeat{DeviceGuid: "recent-heartbeat", CreateTime: recent},
		&domains.DeviceTrafficDaily{DeviceGuid: "old-traffic", Interface: "wwan0", Day: "2026-06-17", LastSeenTime: old},
		&domains.DeviceTrafficDaily{DeviceGuid: "recent-traffic", Interface: "wwan0", Day: "2026-06-19", LastSeenTime: recent},
		&domains.TunnelSession{Guid: "old-closed-session", Status: int(domains.StatusDisabled), StartTime: old, EndTime: old},
		&domains.TunnelSession{Guid: "recent-closed-session", Status: int(domains.StatusDisabled), StartTime: old, EndTime: recent},
		&domains.TunnelSession{Guid: "old-active-session", Status: int(domains.StatusEnabled), StartTime: old, EndTime: 0},
		&domains.DeviceConnection{ConnectionID: "old-closed-connection", Status: int(domains.StatusDisabled), CreateTime: old, UpdateTime: old},
		&domains.DeviceConnection{ConnectionID: "recent-closed-connection", Status: int(domains.StatusDisabled), CreateTime: old, UpdateTime: recent},
		&domains.DeviceConnection{ConnectionID: "old-active-connection", Status: int(domains.StatusEnabled), CreateTime: old, UpdateTime: old},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed row %T: %v", row, err)
		}
	}

	result := ServiceGroupApp.MaintenanceService.CleanupRetention()

	if result.AuditLogs != 1 {
		t.Fatalf("audit deleted = %d, want 1", result.AuditLogs)
	}
	if result.HTTPAccessLogs != 1 {
		t.Fatalf("http access deleted = %d, want 1", result.HTTPAccessLogs)
	}
	if result.DeviceHeartbeats != 1 {
		t.Fatalf("heartbeats deleted = %d, want 1", result.DeviceHeartbeats)
	}
	if result.DeviceTrafficDays != 1 {
		t.Fatalf("traffic days deleted = %d, want 1", result.DeviceTrafficDays)
	}
	if result.TunnelSessions != 1 {
		t.Fatalf("sessions deleted = %d, want 1", result.TunnelSessions)
	}
	if result.DeviceConnections != 1 {
		t.Fatalf("connections deleted = %d, want 1", result.DeviceConnections)
	}

	assertMaintenanceCount(t, db, &domains.AuditLog{}, 1)
	assertMaintenanceCount(t, db, &domains.HTTPAccessLog{}, 1)
	assertMaintenanceCount(t, db, &domains.DeviceHeartbeat{}, 1)
	assertMaintenanceCount(t, db, &domains.DeviceTrafficDaily{}, 1)
	assertMaintenanceCount(t, db, &domains.TunnelSession{}, 2)
	assertMaintenanceCount(t, db, &domains.DeviceConnection{}, 2)
}

func TestCleanupRetentionSkipsDisabledBuckets(t *testing.T) {
	db := setupMaintenanceTestDB(t)
	now := domains.NowMilli()
	old := now - 48*time.Hour.Milliseconds()
	seedRetentionSettings(t, db, 0)
	if err := db.Create(&domains.AuditLog{Actor: "old", CreateTime: old}).Error; err != nil {
		t.Fatalf("seed audit log: %v", err)
	}

	result := ServiceGroupApp.MaintenanceService.CleanupRetention()

	if result.AuditLogs != 0 {
		t.Fatalf("audit deleted = %d, want 0", result.AuditLogs)
	}
	assertMaintenanceCount(t, db, &domains.AuditLog{}, 1)
}

func setupMaintenanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&domains.Setting{},
		&domains.AuditLog{},
		&domains.HTTPAccessLog{},
		&domains.TunnelSession{},
		&domains.DeviceHeartbeat{},
		&domains.DeviceTrafficDaily{},
		&domains.DeviceConnection{},
	); err != nil {
		t.Fatalf("migrate retention tables: %v", err)
	}
	oldDB := global.NAV_DB
	oldLog := global.NAV_LOG
	global.NAV_DB = db
	global.NAV_LOG = zap.NewNop()
	t.Cleanup(func() {
		global.NAV_DB = oldDB
		global.NAV_LOG = oldLog
	})
	return db
}

func seedRetentionSettings(t *testing.T, db *gorm.DB, days int) {
	t.Helper()
	value := "1"
	if days <= 0 {
		value = "0"
	}
	now := domains.NowMilli()
	keys := []string{
		"audit_retention_days",
		"http_access_retention_days",
		"session_retention_days",
		"heartbeat_retention_days",
		"traffic_daily_retention_days",
		"device_connection_retention_days",
	}
	for _, key := range keys {
		if err := db.Create(&domains.Setting{Key: key, Value: value, CreateTime: now, UpdateTime: now}).Error; err != nil {
			t.Fatalf("seed setting %s: %v", key, err)
		}
	}
}

func assertMaintenanceCount(t *testing.T, db *gorm.DB, model any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		t.Fatalf("count %T: %v", model, err)
	}
	if count != want {
		t.Fatalf("%T count = %d, want %d", model, count, want)
	}
}
