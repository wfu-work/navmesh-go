package services

import (
	"testing"
	"time"

	"navmesh-go/domains"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecordDailyDiskUsageEventLimitsOnePerDay(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Event{}); err != nil {
		t.Fatalf("migrate events: %v", err)
	}

	service := ServiceGroupApp.DeviceService.WithDB(db)
	deviceGuid := "device-1"
	first := time.Date(2026, 6, 10, 8, 0, 0, 0, time.Local).UnixMilli()
	second := time.Date(2026, 6, 10, 18, 0, 0, 0, time.Local).UnixMilli()
	nextDay := time.Date(2026, 6, 11, 8, 0, 0, 0, time.Local).UnixMilli()

	req := HeartbeatRequest{DiskUsedPct: 91.2, DiskFree: 10 * 1024 * 1024 * 1024, DiskTotal: 100 * 1024 * 1024 * 1024}
	service.recordDailyDiskUsageEvent(deviceGuid, req, first)
	service.recordDailyDiskUsageEvent(deviceGuid, req, second)
	assertDiskUsageEventCount(t, db, deviceGuid, 1)

	service.recordDailyDiskUsageEvent(deviceGuid, HeartbeatRequest{DiskUsedPct: 89.9}, second)
	assertDiskUsageEventCount(t, db, deviceGuid, 1)

	service.recordDailyDiskUsageEvent(deviceGuid, req, nextDay)
	assertDiskUsageEventCount(t, db, deviceGuid, 2)
}

func TestRecordStaleOfflineEventsWaitsForDelayAndDedupes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Device{}, &domains.Event{}); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}

	service := ServiceGroupApp.DeviceService.WithDB(db)
	now := domains.NowMilli()
	recent := domains.Device{Sncode: "recent", Alias: "recent", Status: domains.DeviceStatusOffline, LastSeenTime: now - 299*time.Second.Milliseconds()}
	stale := domains.Device{Sncode: "stale", Alias: "stale", Status: domains.DeviceStatusOffline, LastSeenTime: now - 301*time.Second.Milliseconds()}
	if err := db.Create(&recent).Error; err != nil {
		t.Fatalf("seed recent device: %v", err)
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatalf("seed stale device: %v", err)
	}

	created, err := service.RecordStaleOfflineEvents(300 * time.Second)
	if err != nil {
		t.Fatalf("record offline events: %v", err)
	}
	if created != 1 {
		t.Fatalf("created events = %d, want 1", created)
	}
	assertOfflineEventCount(t, db, stale.Guid, 1)
	assertOfflineEventCount(t, db, recent.Guid, 0)

	created, err = service.RecordStaleOfflineEvents(300 * time.Second)
	if err != nil {
		t.Fatalf("record offline events again: %v", err)
	}
	if created != 0 {
		t.Fatalf("created duplicate events = %d, want 0", created)
	}
	assertOfflineEventCount(t, db, stale.Guid, 1)

	if err := db.Model(&domains.Event{}).
		Where("device_guid = ? AND event_type = ?", stale.Guid, deviceOfflineEventType).
		Update("status", int(domains.StatusDisabled)).Error; err != nil {
		t.Fatalf("ack offline event: %v", err)
	}
	created, err = service.RecordStaleOfflineEvents(300 * time.Second)
	if err != nil {
		t.Fatalf("record offline events after ack: %v", err)
	}
	if created != 0 {
		t.Fatalf("created acked duplicate events = %d, want 0", created)
	}
	assertOfflineEventCount(t, db, stale.Guid, 1)
}

func TestResolveDeviceWanIPOnlyReturnsIPv4(t *testing.T) {
	tests := []struct {
		name     string
		wanIP    string
		sourceIP string
		want     string
	}{
		{name: "reported ipv4 wins", wanIP: " 203.0.113.10 ", sourceIP: "198.51.100.20", want: "203.0.113.10"},
		{name: "reported ipv6 ignored source ipv4 used", wanIP: "240e:36d:389:5a0::1", sourceIP: "198.51.100.20", want: "198.51.100.20"},
		{name: "source ipv6 ignored", wanIP: "", sourceIP: "240e:36d:389:5a0::1", want: ""},
		{name: "both ipv6 ignored", wanIP: "240e:36d:389:5a0::1", sourceIP: "2001:db8::1", want: ""},
		{name: "ipv4 with port normalized", wanIP: "203.0.113.10:443", sourceIP: "", want: "203.0.113.10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveDeviceWanIP(tt.wanIP, tt.sourceIP); got != tt.want {
				t.Fatalf("resolveDeviceWanIP(%q, %q) = %q, want %q", tt.wanIP, tt.sourceIP, got, tt.want)
			}
		})
	}
}

func assertOfflineEventCount(t *testing.T, db *gorm.DB, deviceGuid string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&domains.Event{}).
		Where("device_guid = ? AND event_type = ?", deviceGuid, deviceOfflineEventType).
		Count(&count).Error; err != nil {
		t.Fatalf("count offline events: %v", err)
	}
	if count != want {
		t.Fatalf("offline event count = %d, want %d", count, want)
	}
}

func assertDiskUsageEventCount(t *testing.T, db *gorm.DB, deviceGuid string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&domains.Event{}).
		Where("device_guid = ? AND event_type = ?", deviceGuid, diskUsageHighEventType).
		Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != want {
		t.Fatalf("disk usage event count = %d, want %d", count, want)
	}
}
