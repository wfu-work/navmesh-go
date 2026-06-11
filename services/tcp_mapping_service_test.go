package services

import (
	"strings"
	"testing"

	"navmesh-go/domains"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTCPMappingSaveAllocatesPublicPortAndDefaultHost(t *testing.T) {
	db := setupTCPMappingTestDB(t)
	seedTCPMappingDevice(t, db, "device-1")
	seedTCPMappingSetting(t, db, "tcp_public_port_min", "25000")
	seedTCPMappingSetting(t, db, "tcp_public_port_max", "25002")
	seedTCPMappingSetting(t, db, "tcp_gateway_domain", "tcpd.example.com")

	service := ServiceGroupApp.TCPMappingService.WithDB(db)
	first, err := service.Save(SaveTCPMappingRequest{
		DeviceGuid: "device-1",
		Name:       "Postgres 主库!",
		TargetHost: "127.0.0.1",
		TargetPort: 5432,
	})
	if err != nil {
		t.Fatalf("save first mapping: %v", err)
	}
	if first.PublicPort != 25000 {
		t.Fatalf("first public port = %d, want 25000", first.PublicPort)
	}
	if first.PublicHost != "postgres.tcpd.example.com" {
		t.Fatalf("first public host = %q, want postgres.tcpd.example.com", first.PublicHost)
	}

	second, err := service.Save(SaveTCPMappingRequest{
		DeviceGuid: "device-1",
		Name:       "redis",
		TargetPort: 6379,
	})
	if err != nil {
		t.Fatalf("save second mapping: %v", err)
	}
	if second.PublicPort != 25001 {
		t.Fatalf("second public port = %d, want 25001", second.PublicPort)
	}
	if second.TargetHost != "127.0.0.1" {
		t.Fatalf("second target host = %q, want 127.0.0.1", second.TargetHost)
	}
}

func TestTCPMappingSaveRejectsDuplicateActivePublicPort(t *testing.T) {
	db := setupTCPMappingTestDB(t)
	seedTCPMappingDevice(t, db, "device-1")
	seedTCPMappingSetting(t, db, "tcp_public_port_min", "25000")
	seedTCPMappingSetting(t, db, "tcp_public_port_max", "25002")

	service := ServiceGroupApp.TCPMappingService.WithDB(db)
	if _, err := service.Save(SaveTCPMappingRequest{
		DeviceGuid: "device-1",
		Name:       "postgres",
		PublicPort: 25000,
		TargetPort: 5432,
	}); err != nil {
		t.Fatalf("save first mapping: %v", err)
	}
	_, err := service.Save(SaveTCPMappingRequest{
		DeviceGuid: "device-1",
		Name:       "redis",
		PublicPort: 25000,
		TargetPort: 6379,
	})
	if err == nil || !strings.Contains(err.Error(), "publicPort already exists") {
		t.Fatalf("duplicate port err = %v, want publicPort already exists", err)
	}
}

func TestTCPMappingEnabledHonorsGlobalSwitch(t *testing.T) {
	db := setupTCPMappingTestDB(t)
	seedTCPMappingSetting(t, db, "tcp_mapping_enabled", "false")
	if err := db.Create(&domains.TCPMapping{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "mapping-1"},
		DeviceGuid:     "device-1",
		Name:           "postgres",
		PublicHost:     "postgres.tcpd.example.com",
		PublicPort:     25000,
		TargetHost:     "127.0.0.1",
		TargetPort:     5432,
		Status:         int(domains.StatusEnabled),
	}).Error; err != nil {
		t.Fatalf("seed tcp mapping: %v", err)
	}

	items, err := ServiceGroupApp.TCPMappingService.WithDB(db).Enabled()
	if err != nil {
		t.Fatalf("enabled mappings: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("enabled mappings = %d, want 0 when tcp mapping is disabled", len(items))
	}
}

func TestValidateTCPPublicPortUsesConfiguredRange(t *testing.T) {
	portRange := TCPPortRange{Min: 25000, Max: 25002}
	if err := validateTCPPublicPort(25001, portRange); err != nil {
		t.Fatalf("valid public port rejected: %v", err)
	}
	if err := validateTCPPublicPort(24999, portRange); err == nil {
		t.Fatal("port below configured range should be rejected")
	}
	if err := validateTCPPublicPort(65536, portRange); err == nil {
		t.Fatal("port above 65535 should be rejected")
	}
}

func setupTCPMappingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := global.NAV_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Device{}, &domains.TCPMapping{}, &domains.Setting{}); err != nil {
		t.Fatalf("migrate tcp mapping tables: %v", err)
	}
	global.NAV_DB = db
	t.Cleanup(func() {
		global.NAV_DB = oldDB
	})
	return db
}

func seedTCPMappingDevice(t *testing.T, db *gorm.DB, guid string) {
	t.Helper()
	device := domains.Device{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: guid},
		Sncode:         guid,
		Alias:          guid,
		Status:         domains.DeviceStatusRegistered,
	}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}
}

func seedTCPMappingSetting(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	if err := db.Create(&domains.Setting{Key: key, Value: value}).Error; err != nil {
		t.Fatalf("seed setting %s: %v", key, err)
	}
}
