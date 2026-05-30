package tunnel

import (
	"testing"
	"time"
)

func TestNewQUICConfigExpandsHTTPMappingCapacity(t *testing.T) {
	cfg := NewQUICConfig(30 * time.Second)
	if cfg.KeepAlivePeriod != 30*time.Second {
		t.Fatalf("KeepAlivePeriod = %s, want 30s", cfg.KeepAlivePeriod)
	}
	if cfg.MaxIdleTimeout < 5*time.Minute {
		t.Fatalf("MaxIdleTimeout = %s, want at least 5m", cfg.MaxIdleTimeout)
	}
	if cfg.MaxIncomingStreams < 1024 {
		t.Fatalf("MaxIncomingStreams = %d, want at least 1024", cfg.MaxIncomingStreams)
	}
	if cfg.MaxConnectionReceiveWindow < 64<<20 {
		t.Fatalf("MaxConnectionReceiveWindow = %d, want at least 64MiB", cfg.MaxConnectionReceiveWindow)
	}
	if cfg.InitialPacketSize != 1200 {
		t.Fatalf("InitialPacketSize = %d, want 1200", cfg.InitialPacketSize)
	}
	if !cfg.DisablePathMTUDiscovery {
		t.Fatal("DisablePathMTUDiscovery = false, want true")
	}
}

func TestNewQUICConfigDefaultsKeepAlive(t *testing.T) {
	cfg := NewQUICConfig(0)
	if cfg.KeepAlivePeriod != defaultQUICKeepAlive {
		t.Fatalf("KeepAlivePeriod = %s, want %s", cfg.KeepAlivePeriod, defaultQUICKeepAlive)
	}
}
