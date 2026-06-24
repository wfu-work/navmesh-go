package services

import (
	"strings"
	"testing"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/global"
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
	stubTemplateEmailSender(t, nil)
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
	stale := domains.Device{Sncode: "stale", Alias: "stale", Status: domains.DeviceStatusOffline, LastSeenTime: now - 10*time.Minute.Milliseconds()}
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

	if err := db.Model(&domains.Device{}).
		Where("guid = ?", stale.Guid).
		Updates(map[string]any{
			"status":         domains.DeviceStatusOffline,
			"last_seen_time": domains.NowMilli() - 10*time.Minute.Milliseconds(),
		}).Error; err != nil {
		t.Fatalf("simulate device flapping offline: %v", err)
	}
	created, err = service.RecordStaleOfflineEvents(300 * time.Second)
	if err != nil {
		t.Fatalf("record offline events after flap: %v", err)
	}
	if created != 0 {
		t.Fatalf("created suppressed flap event = %d, want 0", created)
	}
	assertOfflineEventCount(t, db, stale.Guid, 1)
}

func TestRecordStaleOfflineEventsSendsDeviceOfflineEmail(t *testing.T) {
	emails := make(chan EmailTemplateInput, 1)
	stubTemplateEmailSender(t, emails)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Device{}, &domains.Event{}); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}

	service := ServiceGroupApp.DeviceService.WithDB(db)
	stale := domains.Device{
		Sncode:        "mail-offline",
		Alias:         "邮件离线设备",
		DeviceType:    "ssh",
		Status:        domains.DeviceStatusOffline,
		LastSeenTime:  domains.NowMilli() - 10*time.Minute.Milliseconds(),
		ClientVersion: "v0.0.5",
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
	input := waitForTemplateEmail(t, emails)
	if input.Code != TemplateCodeDeviceOfflineNotice {
		t.Fatalf("email template code = %q, want %q", input.Code, TemplateCodeDeviceOfflineNotice)
	}
	assertVar(t, input.Variables, "deviceAlias", "邮件离线设备")
	assertVar(t, input.Variables, "deviceSncode", "mail-offline")
	if input.Variables["lastSeenTime"] == "" || input.Variables["lastSeenTime"] == "-" {
		t.Fatalf("lastSeenTime variable = %q, want formatted time", input.Variables["lastSeenTime"])
	}

	created, err = service.RecordStaleOfflineEvents(300 * time.Second)
	if err != nil {
		t.Fatalf("record offline events again: %v", err)
	}
	if created != 0 {
		t.Fatalf("created duplicate events = %d, want 0", created)
	}
	select {
	case extra := <-emails:
		t.Fatalf("unexpected duplicate offline email: %#v", extra)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTouchOnlineRefreshesLastSeenAndStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Device{}); err != nil {
		t.Fatalf("migrate devices: %v", err)
	}

	now := domains.NowMilli()
	device := domains.Device{
		Sncode:       "device-1",
		Alias:        "device-1",
		Status:       domains.DeviceStatusOffline,
		LastSeenTime: now - time.Hour.Milliseconds(),
	}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}

	ServiceGroupApp.DeviceService.WithDB(db).TouchOnline(device.Guid)

	var updated domains.Device
	if err := db.Where("guid = ?", device.Guid).First(&updated).Error; err != nil {
		t.Fatalf("load device: %v", err)
	}
	if updated.Status != domains.DeviceStatusOnline {
		t.Fatalf("status = %d, want online", updated.Status)
	}
	if updated.LastSeenTime <= device.LastSeenTime {
		t.Fatalf("lastSeenTime = %d, want newer than %d", updated.LastSeenTime, device.LastSeenTime)
	}
}

func TestGetHydratesWebDomainsAndLastMetricAt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Device{}, &domains.DeviceToken{}, &domains.PortMapping{}, &domains.DeviceHeartbeat{}, &domains.DeviceTrafficState{}); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}
	device := domains.Device{
		Sncode:    "device-detail-1",
		Alias:     "device-detail-1",
		WebDomain: "fallback.example.com",
		Status:    domains.DeviceStatusOnline,
	}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
	mappings := []domains.PortMapping{
		{
			BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: 1000, UpdateTime: 1000},
			DeviceGuid:     device.Guid,
			PublicHost:     "old.example.com",
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: 3000, UpdateTime: 3000},
			DeviceGuid:     device.Guid,
			PublicHost:     "new.example.com",
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: 4000, UpdateTime: 4000},
			DeviceGuid:     device.Guid,
			PublicHost:     "disabled.example.com",
			Status:         int(domains.StatusDisabled),
		},
	}
	if err := db.Create(&mappings).Error; err != nil {
		t.Fatalf("seed mappings: %v", err)
	}
	if err := db.Create(&domains.DeviceHeartbeat{DeviceGuid: device.Guid, CreateTime: 2000}).Error; err != nil {
		t.Fatalf("seed heartbeat: %v", err)
	}
	if err := db.Create(&domains.DeviceTrafficState{DeviceGuid: device.Guid, Interface: "wwan0", SampleTime: 5000, UpdateTime: 4500}).Error; err != nil {
		t.Fatalf("seed traffic state: %v", err)
	}

	detail, err := ServiceGroupApp.DeviceService.WithDB(db).Get(device.Guid)
	if err != nil {
		t.Fatalf("get device detail: %v", err)
	}
	if detail.Device.WebDomain != "new.example.com, old.example.com" {
		t.Fatalf("webDomain = %q, want enabled mapping domains ordered by update time", detail.Device.WebDomain)
	}
	if got := strings.Join(detail.Device.WebDomains, ","); got != "new.example.com,old.example.com" {
		t.Fatalf("webDomains = %q", got)
	}
	if detail.Device.LastMetricAt != 5000 {
		t.Fatalf("lastMetricAt = %d, want latest metric sample 5000", detail.Device.LastMetricAt)
	}
}

func TestHeartbeatRecordsNetworkQualitySnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Device{}, &domains.DeviceGroup{}, &domains.DeviceToken{}, &domains.DeviceHeartbeat{}, &domains.DeviceTrafficState{}, &domains.DeviceTrafficDaily{}, &domains.Event{}); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}
	oldDB := global.NAV_DB
	global.NAV_DB = db
	t.Cleanup(func() {
		global.NAV_DB = oldDB
	})
	if err := db.Create(&domains.DeviceGroup{
		Key:    "ssh",
		Name:   "SSH",
		Status: int(domains.StatusEnabled),
	}).Error; err != nil {
		t.Fatalf("seed device group: %v", err)
	}

	device := domains.Device{
		Sncode:     "device-network-1",
		Alias:      "device-network-1",
		DeviceType: "ssh",
		Status:     domains.DeviceStatusOnline,
	}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
	token := "device-token"
	if err := db.Create(&domains.DeviceToken{
		DeviceGuid: device.Guid,
		Token:      token,
		TokenHash:  utils.HashToken(token),
		Name:       "test",
		Status:     domains.DeviceTokenStatusEnabled,
	}).Error; err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := HeartbeatRequest{
		Guid:          device.Guid,
		Token:         token,
		NetworkType:   "4g",
		NetworkIface:  "wwan0",
		SignalDBM:     -82,
		SignalPct:     120,
		CellularRSRP:  -95,
		CellularRSRQ:  -11,
		CellularSINR:  18,
		PingTarget:    "https://api.example.com/api/device/heartbeat",
		PingLatencyMs: 37,
		PingLossPct:   -1,
		RXRateBps:     120000,
		TXRateBps:     34000,
	}
	if _, err := ServiceGroupApp.DeviceService.WithDB(db).Heartbeat(req, "198.51.100.10"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	var updated domains.Device
	if err := db.Where("guid = ?", device.Guid).First(&updated).Error; err != nil {
		t.Fatalf("load device: %v", err)
	}
	if updated.NetworkType != "cellular" || updated.NetworkIface != "wwan0" || updated.SignalDBM != -82 || updated.SignalPct != 100 {
		t.Fatalf("device network snapshot = %+v, want normalized cellular wwan0 signal", updated)
	}
	if updated.CellularRSRP != -95 || updated.CellularRSRQ != -11 || updated.CellularSINR != 18 {
		t.Fatalf("device cellular metrics = rsrp=%d rsrq=%d sinr=%d", updated.CellularRSRP, updated.CellularRSRQ, updated.CellularSINR)
	}
	if updated.PingLatencyMs != 37 || updated.PingLossPct != 0 || updated.RXRateBps != 120000 || updated.TXRateBps != 34000 {
		t.Fatalf("device link metrics = latency=%d loss=%.1f rx=%d tx=%d", updated.PingLatencyMs, updated.PingLossPct, updated.RXRateBps, updated.TXRateBps)
	}

	var heartbeat domains.DeviceHeartbeat
	if err := db.Where("device_guid = ?", device.Guid).First(&heartbeat).Error; err != nil {
		t.Fatalf("load heartbeat: %v", err)
	}
	if heartbeat.NetworkType != "cellular" || heartbeat.SignalPct != 100 || heartbeat.RXRateBps != 120000 {
		t.Fatalf("heartbeat network snapshot = %+v", heartbeat)
	}
}

func TestNetworkSnapshotDerivesSignalFromWifiRSSI(t *testing.T) {
	snapshot := networkSnapshotFromHeartbeat(HeartbeatRequest{
		NetworkType:  "wifi",
		NetworkIface: "wlan0",
		WifiRSSI:     -70,
	})
	if snapshot.SignalDBM != -70 {
		t.Fatalf("signalDbm = %d, want wifi rssi -70", snapshot.SignalDBM)
	}
	if snapshot.SignalPct != 66 {
		t.Fatalf("signalPct = %d, want derived percent 66", snapshot.SignalPct)
	}
}

func TestCarrierLocationReconcilesWlanWifiSnapshotToCellular(t *testing.T) {
	snapshot := reconcileNetworkSnapshotWithLocation(networkSnapshotFromHeartbeat(HeartbeatRequest{
		NetworkType:  "wifi",
		NetworkIface: "wlan0",
		RXRateBps:    3000,
		TXRateBps:    1400,
	}), "中国 移动")

	if snapshot.NetworkType != "cellular" {
		t.Fatalf("networkType = %q, want cellular for mobile carrier wlan snapshot", snapshot.NetworkType)
	}
	if snapshot.NetworkIface != "wlan0" || snapshot.RXRateBps != 3000 || snapshot.TXRateBps != 1400 {
		t.Fatalf("snapshot = %+v, want iface and rate preserved", snapshot)
	}
}

func TestCarrierLocationKeepsWifiSnapshotWithWifiMetrics(t *testing.T) {
	snapshot := reconcileNetworkSnapshotWithLocation(networkSnapshotFromHeartbeat(HeartbeatRequest{
		NetworkType:  "wifi",
		NetworkIface: "wlan0",
		WifiSSID:     "site-wifi",
		WifiRSSI:     -68,
	}), "中国 移动")

	if snapshot.NetworkType != "wifi" {
		t.Fatalf("networkType = %q, want wifi when wifi metrics exist", snapshot.NetworkType)
	}
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

func TestDeviceStatsRespectsStatusFilter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Device{}); err != nil {
		t.Fatalf("migrate devices: %v", err)
	}

	devices := []domains.Device{
		{Sncode: "online-1", Alias: "online-1", Status: domains.DeviceStatusOnline, DeviceType: "ssh"},
		{Sncode: "offline-1", Alias: "offline-1", Status: domains.DeviceStatusOffline, DeviceType: "ssh"},
		{Sncode: "offline-2", Alias: "offline-2", Status: domains.DeviceStatusOffline, DeviceType: "ssh"},
	}
	if err := db.Create(&devices).Error; err != nil {
		t.Fatalf("seed devices: %v", err)
	}

	stats, err := ServiceGroupApp.DeviceService.WithDB(db).Stats(map[string]string{
		"status": "3",
		"type":   "ssh",
	})
	if err != nil {
		t.Fatalf("device stats: %v", err)
	}
	if stats.Total != 2 || stats.Online != 0 || stats.Offline != 2 {
		t.Fatalf("stats = %+v, want total=2 online=0 offline=2", stats)
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

func stubTemplateEmailSender(t *testing.T, emails chan<- EmailTemplateInput) {
	t.Helper()
	previous := sendTemplateEmail
	sendTemplateEmail = func(_ EmailService, input EmailTemplateInput) (*EmailSendResult, error) {
		if emails != nil {
			emails <- input
		}
		return &EmailSendResult{Recipients: 1, Successes: 1, Subject: input.Title}, nil
	}
	t.Cleanup(func() {
		sendTemplateEmail = previous
	})
}

func waitForTemplateEmail(t *testing.T, emails <-chan EmailTemplateInput) EmailTemplateInput {
	t.Helper()
	select {
	case input := <-emails:
		return input
	case <-time.After(time.Second):
		t.Fatal("expected offline email")
		return EmailTemplateInput{}
	}
}
