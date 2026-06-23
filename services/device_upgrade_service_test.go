package services

import (
	"testing"

	"navmesh-go/domains"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSamePlatformLinuxDistributionAliases(t *testing.T) {
	tests := []struct {
		current string
		target  string
	}{
		{current: "ubuntu", target: "linux"},
		{current: "ubuntu 20.04", target: "linux"},
		{current: "Ubuntu 22.04.4 LTS", target: "linux"},
		{current: "ubuntu/linux", target: "linux"},
		{current: "GNU/Linux", target: "linux"},
		{current: "linux-gnu", target: "linux"},
		{current: "debian", target: "linux"},
		{current: "debian 12.14", target: "linux"},
		{current: "centos", target: "linux"},
		{current: "centos 7", target: "linux"},
		{current: "alpine", target: "linux"},
		{current: "rocky linux 9", target: "linux"},
		{current: "almaLinux 9.4", target: "linux"},
	}

	for _, tt := range tests {
		t.Run(tt.current, func(t *testing.T) {
			if !samePlatform(tt.current, tt.target) {
				t.Fatalf("samePlatform(%q, %q) = false; want true", tt.current, tt.target)
			}
		})
	}
}

func TestSamePlatformArchitectureAliases(t *testing.T) {
	tests := []struct {
		current string
		target  string
	}{
		{current: "x86_64", target: "amd64"},
		{current: "x86-64", target: "amd64"},
		{current: "aarch64", target: "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.current, func(t *testing.T) {
			if !samePlatform(tt.current, tt.target) {
				t.Fatalf("samePlatform(%q, %q) = false; want true", tt.current, tt.target)
			}
		})
	}
}

func TestReleaseUpgradeDisabledReasonExplainsPlatformMismatch(t *testing.T) {
	release := domains.Release{ReleaseType: domains.ReleaseTypeNavmesh, DeviceType: "all", OS: "linux", Arch: "arm64"}
	device := domains.Device{DeviceType: "ssh", OS: "linux", Arch: "amd64", ClientVersion: "v0.0.3"}

	if got := releaseUpgradeDisabledReason(release, device); got != "设备系统或架构与升级包不匹配" {
		t.Fatalf("releaseUpgradeDisabledReason() = %q", got)
	}
}

func TestReleaseUpgradeDisabledReasonAllowsMatchingPlatform(t *testing.T) {
	release := domains.Release{ReleaseType: domains.ReleaseTypeNavmesh, DeviceType: "all", OS: "linux", Arch: "arm64"}
	device := domains.Device{DeviceType: "ssh", OS: "ubuntu", Arch: "arm64", ClientVersion: "v0.0.3"}

	if got := releaseUpgradeDisabledReason(release, device); got != "" {
		t.Fatalf("releaseUpgradeDisabledReason() = %q, want empty", got)
	}
}

func TestReleaseUpgradeDisabledReasonRequiresRainCapableClient(t *testing.T) {
	release := domains.Release{ReleaseType: domains.ReleaseTypeRain, DeviceType: "all", OS: "linux", Arch: "arm64"}
	device := domains.Device{DeviceType: "rain", OS: "linux", Arch: "arm64", ClientVersion: "v0.0.2"}

	if got := releaseUpgradeDisabledReason(release, device); got != "设备客户端版本不支持北斗降雨在线升级" {
		t.Fatalf("releaseUpgradeDisabledReason() = %q", got)
	}
}

func TestReleaseUpgradeDisabledReasonRequiresHipnamesCapableClient(t *testing.T) {
	release := domains.Release{ReleaseType: domains.ReleaseTypeHipnames, DeviceType: "hipnames", OS: "linux", Arch: "arm64"}
	device := domains.Device{DeviceType: "hipnames", OS: "linux", Arch: "arm64", ClientVersion: "v0.0.3"}

	if got := releaseUpgradeDisabledReason(release, device); got != "设备客户端版本不支持单机版解算在线升级" {
		t.Fatalf("releaseUpgradeDisabledReason() = %q", got)
	}

	device.ClientVersion = "v0.0.4"
	if got := releaseUpgradeDisabledReason(release, device); got != "" {
		t.Fatalf("releaseUpgradeDisabledReason() = %q, want empty", got)
	}
}

func TestHipnamesReleaseTypeSupportsOnlineUpgrade(t *testing.T) {
	for _, value := range []string{domains.ReleaseTypeHipnames, "standalone"} {
		t.Run(value, func(t *testing.T) {
			if !isOnlineUpgradeableReleaseType(value) {
				t.Fatalf("isOnlineUpgradeableReleaseType(%q) = false, want true", value)
			}
		})
	}
}

func TestListUpgradeTasksFiltersReleaseType(t *testing.T) {
	db := setupDeviceUpgradeTestDB(t)
	service := DeviceUpgradeService{}.WithDB(db)
	deviceGuid := "device-upgrade-filter"
	tasks := []domains.DeviceUpgradeTask{
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "task-client", CreateTime: 300},
			DeviceGuid:     deviceGuid,
			ReleaseType:    domains.ReleaseTypeNavmesh,
			Version:        "v0.0.5",
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "task-rain", CreateTime: 200},
			DeviceGuid:     deviceGuid,
			ReleaseType:    domains.ReleaseTypeRain,
			Version:        "v1.0.1",
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "task-legacy-client", CreateTime: 100},
			DeviceGuid:     deviceGuid,
			Version:        "v0.0.4",
		},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatalf("create tasks: %v", err)
	}

	rainItems, rainTotal, err := service.List(deviceGuid, map[string]string{"releaseType": "rain", "page": "1", "size": "10"})
	if err != nil {
		t.Fatalf("list rain tasks: %v", err)
	}
	if rainTotal != 1 || len(rainItems) != 1 || rainItems[0].Guid != "task-rain" {
		t.Fatalf("rain tasks = total %d items %+v, want task-rain only", rainTotal, rainItems)
	}

	clientItems, clientTotal, err := service.List(deviceGuid, map[string]string{"releaseType": "navmesh", "page": "1", "size": "10"})
	if err != nil {
		t.Fatalf("list client tasks: %v", err)
	}
	if clientTotal != 2 || len(clientItems) != 2 {
		t.Fatalf("client tasks = total %d len %d, want 2", clientTotal, len(clientItems))
	}
}

func TestReportProgress(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		progress int
		previous int
		want     int
	}{
		{name: "running starts at one", status: domains.DeviceUpgradeStatusRunning, progress: 0, previous: 0, want: 1},
		{name: "running keeps previous", status: domains.DeviceUpgradeStatusRunning, progress: 0, previous: 42, want: 42},
		{name: "running caps below terminal", status: domains.DeviceUpgradeStatusRunning, progress: 120, previous: 42, want: 99},
		{name: "success completes", status: domains.DeviceUpgradeStatusSuccess, progress: 60, previous: 60, want: 100},
		{name: "failed keeps previous when omitted", status: domains.DeviceUpgradeStatusFailed, progress: 0, previous: 73, want: 73},
		{name: "failed accepts reported progress", status: domains.DeviceUpgradeStatusFailed, progress: 88, previous: 73, want: 88},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reportProgress(tt.status, tt.progress, tt.previous); got != tt.want {
				t.Fatalf("reportProgress(%d, %d, %d) = %d; want %d", tt.status, tt.progress, tt.previous, got, tt.want)
			}
		})
	}
}

func TestBatchSummaryProgressUsesTaskProgress(t *testing.T) {
	db := setupDeviceUpgradeTestDB(t)
	service := DeviceUpgradeService{}.WithDB(db)
	batch := domains.DeviceUpgradeBatch{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "batch-progress"},
		TotalCount:     2,
	}
	tasks := []domains.DeviceUpgradeTask{
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "task-running"},
			BatchGuid:      batch.Guid,
			Status:         domains.DeviceUpgradeStatusRunning,
			Progress:       60,
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "task-success"},
			BatchGuid:      batch.Guid,
			Status:         domains.DeviceUpgradeStatusSuccess,
			Progress:       100,
		},
	}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatalf("create tasks: %v", err)
	}

	summary := service.batchSummary(batch)
	if summary.Progress != 80 {
		t.Fatalf("batch progress = %d, want averaged task progress 80", summary.Progress)
	}
	if summary.FinishedCount != 1 || summary.RunningCount != 1 || summary.Status != "running" {
		t.Fatalf("summary counts/status = finished %d running %d status %q, want finished 1 running 1 status running", summary.FinishedCount, summary.RunningCount, summary.Status)
	}
}

func setupDeviceUpgradeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.DeviceUpgradeTask{}, &domains.DeviceUpgradeBatch{}); err != nil {
		t.Fatalf("migrate upgrade tasks: %v", err)
	}
	return db
}
