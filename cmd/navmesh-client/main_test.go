package main

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/quic-go/quic-go"
	gopsnet "github.com/shirou/gopsutil/v4/net"
)

func TestBridgePrefersHalfClose(t *testing.T) {
	a := newHalfCloseBufferConn("from-a")
	b := newHalfCloseBufferConn("from-b")

	bridge(a, b)

	if got := a.String(); got != "from-b" {
		t.Fatalf("a received %q, want from-b", got)
	}
	if got := b.String(); got != "from-a" {
		t.Fatalf("b received %q, want from-a", got)
	}
	if a.closeWriteCount == 0 || b.closeWriteCount == 0 {
		t.Fatalf("CloseWrite was not called: a=%d b=%d", a.closeWriteCount, b.closeWriteCount)
	}
	if a.closeReadCount == 0 || b.closeReadCount == 0 {
		t.Fatalf("CloseRead was not called: a=%d b=%d", a.closeReadCount, b.closeReadCount)
	}
	if a.closeCount == 0 || b.closeCount == 0 {
		t.Fatalf("Close was not called: a=%d b=%d", a.closeCount, b.closeCount)
	}
}

func TestCloseReadCancelsQUICReadSide(t *testing.T) {
	conn := &quicCancelReadConn{}
	closeRead(conn)
	if conn.cancelReadCount != 1 {
		t.Fatalf("CancelRead count = %d, want 1", conn.cancelReadCount)
	}
	if conn.closeCount != 0 {
		t.Fatalf("Close count = %d, want 0", conn.closeCount)
	}
}

func TestHeartbeatFailedRequiresTunnelHeartbeat(t *testing.T) {
	err := errors.New("deadline exceeded")
	tests := []struct {
		name    string
		quicErr error
		httpErr error
		want    bool
	}{
		{name: "both ok", want: false},
		{name: "quic failed http ok", quicErr: err, want: true},
		{name: "quic ok http failed", httpErr: err, want: false},
		{name: "both failed", quicErr: err, httpErr: err, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := heartbeatFailed(tt.quicErr, tt.httpErr); got != tt.want {
				t.Fatalf("heartbeatFailed() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestNormalizeTransport(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "", want: clientTransportAuto},
		{value: "AUTO", want: clientTransportAuto},
		{value: "quic", want: "quic"},
		{value: "tcp", want: "tcp"},
		{value: "bad", want: clientTransportAuto},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := normalizeTransport(tt.value); got != tt.want {
				t.Fatalf("normalizeTransport(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestNormalizeUpgradeReleaseTypeSupportsHipnamesAliases(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "hipnames", want: releaseTypeHipnames},
		{value: "standalone", want: releaseTypeHipnames},
		{value: "device_software", want: releaseTypeRain},
		{value: "navmesh_client", want: releaseTypeNavmesh},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := normalizeUpgradeReleaseType(tt.value); got != tt.want {
				t.Fatalf("normalizeUpgradeReleaseType(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestHipnamesUpgradeMessages(t *testing.T) {
	upgrade := clientUpgradeCommand{ReleaseType: releaseTypeHipnames}
	if got := upgradeDownloadMessage(upgrade); got != "正在下载单机版解算应用" {
		t.Fatalf("upgradeDownloadMessage() = %q", got)
	}
	if got := upgradeVerifyMessage(upgrade); got != "正在校验单机版解算应用" {
		t.Fatalf("upgradeVerifyMessage() = %q", got)
	}
	if got := upgradeVerifiedMessage(upgrade); got != "单机版解算应用校验完成" {
		t.Fatalf("upgradeVerifiedMessage() = %q", got)
	}
}

func TestNormalizeIPv4RejectsIPv6(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "plain ipv4", value: " 203.0.113.10 ", want: "203.0.113.10"},
		{name: "ipv4 with port", value: "203.0.113.10:8080", want: "203.0.113.10"},
		{name: "ipv6", value: "240e:36d:389:5a0::1", want: ""},
		{name: "ipv6 with brackets", value: "[240e:36d:389:5a0::1]", want: ""},
		{name: "invalid", value: "not-an-ip", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeIPv4(tt.value); got != tt.want {
				t.Fatalf("normalizeIPv4(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestIsPublicIPv4RejectsIPv6AndPrivateIP(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "8.8.8.8", want: true},
		{value: "192.168.1.10", want: false},
		{value: "240e:36d:389:5a0::1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := isPublicIPv4(tt.value); got != tt.want {
				t.Fatalf("isPublicIPv4(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}

func TestDefaultTCPDataChannelsSupportsResourceHeavyPages(t *testing.T) {
	if defaultTCPDataChannels < 32 {
		t.Fatalf("defaultTCPDataChannels = %d, want at least 32", defaultTCPDataChannels)
	}
}

func TestSelectTrafficInterfaceFromCountersPrefersSingleActivePhysicalWithTraffic(t *testing.T) {
	counters := []gopsnet.IOCountersStat{
		{Name: "lo", BytesRecv: 100, BytesSent: 100},
		{Name: "eth0"},
		{Name: "eth1", BytesRecv: 1607687512, BytesSent: 3818216370},
		{Name: "docker0"},
	}
	active := map[string]bool{
		"lo":      true,
		"eth0":    true,
		"eth1":    true,
		"docker0": true,
	}
	if got := selectTrafficInterfaceFromCounters(counters, active); got != "eth1" {
		t.Fatalf("selectTrafficInterfaceFromCounters() = %q, want eth1", got)
	}
}

func TestSelectTrafficInterfaceFromCountersPrefersCellularName(t *testing.T) {
	counters := []gopsnet.IOCountersStat{
		{Name: "eth1", BytesRecv: 1607687512, BytesSent: 3818216370},
		{Name: "wwan0"},
	}
	active := map[string]bool{"eth1": true, "wwan0": true}
	if got := selectTrafficInterfaceFromCounters(counters, active); got != "wwan0" {
		t.Fatalf("selectTrafficInterfaceFromCounters() = %q, want wwan0", got)
	}
}

func TestSelectTrafficInterfaceFromCountersKeepsAmbiguousPhysicalInterfacesDisabled(t *testing.T) {
	counters := []gopsnet.IOCountersStat{
		{Name: "eth0", BytesRecv: 10, BytesSent: 10},
		{Name: "eth1", BytesRecv: 20, BytesSent: 20},
	}
	active := map[string]bool{"eth0": true, "eth1": true}
	if got := selectTrafficInterfaceFromCounters(counters, active); got != "" {
		t.Fatalf("selectTrafficInterfaceFromCounters() = %q, want empty", got)
	}
}

func TestInferNetworkTypeSupportsCellularWifiAndEthernet(t *testing.T) {
	tests := []struct {
		iface string
		want  string
	}{
		{iface: "wwan0", want: "cellular"},
		{iface: "wlan0", want: "wifi"},
		{iface: "eth0", want: "ethernet"},
		{iface: "tun0", want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.iface, func(t *testing.T) {
			if got := inferNetworkType(tt.iface); got != tt.want {
				t.Fatalf("inferNetworkType(%q) = %q, want %q", tt.iface, got, tt.want)
			}
		})
	}
}

func TestNetworkSnapshotNormalizesSignalAndLinkMetrics(t *testing.T) {
	snapshot := networkSnapshot{
		NetworkType:   "4g",
		NetworkIface:  " wwan0 ",
		SignalPct:     150,
		PingLatencyMs: -1,
		PingLossPct:   120,
		RXRateBps:     -1,
		TXRateBps:     2048,
	}.normalized()

	if snapshot.NetworkType != "cellular" || snapshot.NetworkIface != "wwan0" {
		t.Fatalf("normalized network identity = type=%q iface=%q", snapshot.NetworkType, snapshot.NetworkIface)
	}
	if snapshot.SignalPct != 100 || snapshot.PingLatencyMs != 0 || snapshot.PingLossPct != 100 {
		t.Fatalf("normalized quality = signal=%d latency=%d loss=%.1f", snapshot.SignalPct, snapshot.PingLatencyMs, snapshot.PingLossPct)
	}
	if snapshot.RXRateBps != 0 || snapshot.TXRateBps != 2048 {
		t.Fatalf("normalized rates = rx=%d tx=%d", snapshot.RXRateBps, snapshot.TXRateBps)
	}
}

func TestUpdateTrafficRatesCalculatesBpsAndHandlesReset(t *testing.T) {
	networkQualityState.Lock()
	networkQualityState.traffic = trafficRateState{}
	networkQualityState.Unlock()

	rx, tx := updateTrafficRates(trafficSnapshot{Interface: "wwan0", RXBytes: 1000, TXBytes: 2000, SampleTime: 1000, BootID: "boot-1"})
	if rx != 0 || tx != 0 {
		t.Fatalf("first traffic rate = rx=%d tx=%d, want zero baseline", rx, tx)
	}
	rx, tx = updateTrafficRates(trafficSnapshot{Interface: "wwan0", RXBytes: 1600, TXBytes: 2300, SampleTime: 2000, BootID: "boot-1"})
	if rx != 4800 || tx != 2400 {
		t.Fatalf("traffic rate = rx=%d tx=%d, want rx=4800 tx=2400", rx, tx)
	}
	rx, tx = updateTrafficRates(trafficSnapshot{Interface: "wwan0", RXBytes: 100, TXBytes: 100, SampleTime: 3000, BootID: "boot-2"})
	if rx != 0 || tx != 0 {
		t.Fatalf("reset traffic rate = rx=%d tx=%d, want zero", rx, tx)
	}
}

func TestHeartbeatProbeTargetUsesDeviceHeartbeatEndpoint(t *testing.T) {
	got := heartbeatProbeTarget(clientConfig{API: "https://example.com/base?x=1"})
	want := "https://example.com/api/device/heartbeat"
	if got != want {
		t.Fatalf("heartbeatProbeTarget() = %q, want %q", got, want)
	}
}

type halfCloseBufferConn struct {
	mu              sync.Mutex
	readBuffer      *bytes.Buffer
	writeBuffer     bytes.Buffer
	closeWriteCount int
	closeReadCount  int
	closeCount      int
}

func newHalfCloseBufferConn(read string) *halfCloseBufferConn {
	return &halfCloseBufferConn{readBuffer: bytes.NewBufferString(read)}
}

func (c *halfCloseBufferConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readBuffer.Len() == 0 {
		return 0, io.EOF
	}
	return c.readBuffer.Read(p)
}

func (c *halfCloseBufferConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeBuffer.Write(p)
}

func (c *halfCloseBufferConn) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	return nil
}

func (c *halfCloseBufferConn) CloseWrite() error {
	c.mu.Lock()
	c.closeWriteCount++
	c.mu.Unlock()
	return nil
}

func (c *halfCloseBufferConn) CloseRead() error {
	c.mu.Lock()
	c.closeReadCount++
	c.mu.Unlock()
	return nil
}

func (c *halfCloseBufferConn) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeBuffer.String()
}

type quicCancelReadConn struct {
	closeCount      int
	cancelReadCount int
}

func (c *quicCancelReadConn) Close() error {
	c.closeCount++
	return nil
}

func (c *quicCancelReadConn) CancelRead(quic.StreamErrorCode) {
	c.cancelReadCount++
}
