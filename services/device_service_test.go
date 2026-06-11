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
