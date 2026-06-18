package services

import (
	"testing"
	"time"

	"navmesh-go/domains"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeviceTrafficRecordHeartbeatSample(t *testing.T) {
	db := setupDeviceTrafficTestDB(t)
	service := DeviceTrafficService{}.WithDB(db)
	deviceGuid := "device-traffic-1"
	base := time.Date(2026, 6, 18, 9, 0, 0, 0, time.Local).UnixMilli()

	req := HeartbeatRequest{
		TrafficIface:      "wwan0",
		TrafficRXBytes:    1000,
		TrafficTXBytes:    2000,
		TrafficSampleTime: base,
		TrafficBootID:     "boot-1",
	}
	if err := service.RecordHeartbeatSample(deviceGuid, req, base); err != nil {
		t.Fatalf("record first sample: %v", err)
	}
	assertDeviceTrafficDaily(t, db, deviceGuid, "wwan0", 0, 0, 0, 0)

	req.TrafficRXBytes = 1500
	req.TrafficTXBytes = 2600
	req.TrafficSampleTime = base + time.Minute.Milliseconds()
	if err := service.RecordHeartbeatSample(deviceGuid, req, req.TrafficSampleTime); err != nil {
		t.Fatalf("record second sample: %v", err)
	}
	assertDeviceTrafficDaily(t, db, deviceGuid, "wwan0", 500, 600, 1, 0)

	req.TrafficRXBytes = 100
	req.TrafficTXBytes = 200
	req.TrafficBootID = "boot-2"
	req.TrafficSampleTime = base + 2*time.Minute.Milliseconds()
	if err := service.RecordHeartbeatSample(deviceGuid, req, req.TrafficSampleTime); err != nil {
		t.Fatalf("record reset sample: %v", err)
	}
	assertDeviceTrafficDaily(t, db, deviceGuid, "wwan0", 500, 600, 2, 1)

	req.TrafficRXBytes = 180
	req.TrafficTXBytes = 250
	req.TrafficSampleTime = base + 3*time.Minute.Milliseconds()
	if err := service.RecordHeartbeatSample(deviceGuid, req, req.TrafficSampleTime); err != nil {
		t.Fatalf("record post-reset sample: %v", err)
	}
	assertDeviceTrafficDaily(t, db, deviceGuid, "wwan0", 580, 650, 3, 1)

	items, summary, err := service.Daily(map[string]string{
		"deviceGuid": deviceGuid,
		"to":         time.UnixMilli(base).In(time.Local).Format("2006-01-02"),
		"days":       "1",
	})
	if err != nil {
		t.Fatalf("query daily traffic: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(items))
	}
	if summary.RXBytes != 580 || summary.TXBytes != 650 || summary.TotalBytes != 1230 {
		t.Fatalf("summary = %+v, want rx=580 tx=650 total=1230", summary)
	}
}

func setupDeviceTrafficTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.DeviceTrafficState{}, &domains.DeviceTrafficDaily{}); err != nil {
		t.Fatalf("migrate device traffic tables: %v", err)
	}
	return db
}

func assertDeviceTrafficDaily(t *testing.T, db *gorm.DB, deviceGuid, iface string, wantRX, wantTX, wantSamples, wantResets int64) {
	t.Helper()
	var row domains.DeviceTrafficDaily
	err := db.Where("device_guid = ? AND iface = ?", deviceGuid, iface).First(&row).Error
	if wantSamples == 0 {
		if err == nil {
			t.Fatalf("daily row exists before first delta: %+v", row)
		}
		return
	}
	if err != nil {
		t.Fatalf("load daily traffic: %v", err)
	}
	if row.RXBytes != wantRX || row.TXBytes != wantTX || row.TotalBytes != wantRX+wantTX {
		t.Fatalf("traffic row bytes = rx=%d tx=%d total=%d, want rx=%d tx=%d total=%d", row.RXBytes, row.TXBytes, row.TotalBytes, wantRX, wantTX, wantRX+wantTX)
	}
	if row.SampleCount != wantSamples || row.ResetCount != wantResets {
		t.Fatalf("traffic row counts = samples=%d resets=%d, want samples=%d resets=%d", row.SampleCount, row.ResetCount, wantSamples, wantResets)
	}
}
