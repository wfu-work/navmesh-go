package services

import (
	"testing"

	"navmesh-go/domains"
)

func TestSamePlatformLinuxDistributionAliases(t *testing.T) {
	tests := []struct {
		current string
		target  string
	}{
		{current: "ubuntu", target: "linux"},
		{current: "ubuntu 20.04", target: "linux"},
		{current: "Ubuntu 22.04.4 LTS", target: "linux"},
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
