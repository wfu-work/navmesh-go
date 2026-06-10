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
		{current: "debian", target: "linux"},
		{current: "centos", target: "linux"},
		{current: "alpine", target: "linux"},
	}

	for _, tt := range tests {
		t.Run(tt.current, func(t *testing.T) {
			if !samePlatform(tt.current, tt.target) {
				t.Fatalf("samePlatform(%q, %q) = false; want true", tt.current, tt.target)
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
