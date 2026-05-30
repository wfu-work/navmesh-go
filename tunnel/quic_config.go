package tunnel

import (
	"time"

	"github.com/quic-go/quic-go"
)

const (
	defaultQUICKeepAlive = 30 * time.Second
	minQUICIdleTimeout   = 5 * time.Minute
)

func NewQUICConfig(keepAlive time.Duration) *quic.Config {
	if keepAlive <= 0 {
		keepAlive = defaultQUICKeepAlive
	}
	idleTimeout := keepAlive * 8
	if idleTimeout < minQUICIdleTimeout {
		idleTimeout = minQUICIdleTimeout
	}
	return &quic.Config{
		KeepAlivePeriod:                  keepAlive,
		MaxIdleTimeout:                   idleTimeout,
		MaxIncomingStreams:               2048,
		InitialStreamReceiveWindow:       1 << 20,
		MaxStreamReceiveWindow:           16 << 20,
		InitialConnectionReceiveWindow:   4 << 20,
		MaxConnectionReceiveWindow:       128 << 20,
		InitialPacketSize:                1200,
		DisablePathMTUDiscovery:          true,
		EnableStreamResetPartialDelivery: true,
	}
}
