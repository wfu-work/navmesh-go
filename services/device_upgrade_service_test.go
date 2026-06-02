package services

import "testing"

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
