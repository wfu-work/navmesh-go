package tcpgateway

import (
	"testing"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecordTCPOpenFailedEventSuppressesDuplicates(t *testing.T) {
	db := setupTCPGatewayTestDB(t)

	if !recordTCPOpenFailedEvent("device-1", "device tunnel offline") {
		t.Fatal("first tcp open failed event should be recorded")
	}
	if recordTCPOpenFailedEvent("device-1", "device tunnel offline") {
		t.Fatal("duplicate tcp open failed event inside suppression window should be skipped")
	}

	var count int64
	if err := db.Model(&domains.Event{}).
		Where("device_guid = ? AND event_type = ? AND title = ?", "device-1", "open_tcp_failed", "open tcp mapping target failed").
		Count(&count).Error; err != nil {
		t.Fatalf("count tcp open failed events: %v", err)
	}
	if count != 1 {
		t.Fatalf("event count = %d, want 1", count)
	}
}

func setupTCPGatewayTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := global.NAV_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Event{}); err != nil {
		t.Fatalf("migrate events: %v", err)
	}
	global.NAV_DB = db
	t.Cleanup(func() {
		global.NAV_DB = oldDB
	})
	return db
}
