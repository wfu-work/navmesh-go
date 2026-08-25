package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestDownloadUpgradeBinaryResumesAfterInterruptedResponse(t *testing.T) {
	data := []byte("0123456789")
	digest := sha256.Sum256(data)
	var requests atomic.Int32
	var requestedRange atomic.Value
	withUpgradeRoundTripper(t, upgradeRoundTripper(func(r *http.Request) (*http.Response, error) {
		header := make(http.Header)
		switch requests.Add(1) {
		case 1:
			header.Set("Content-Length", strconv.Itoa(len(data)))
			return &http.Response{StatusCode: http.StatusOK, Header: header, ContentLength: int64(len(data)), Body: &failingUpgradeBody{reader: bytes.NewReader(data[:4])}}, nil
		case 2:
			requestedRange.Store(r.Header.Get("Range"))
			header.Set("Content-Range", "bytes 4-9/10")
			header.Set("Content-Length", "6")
			return &http.Response{StatusCode: http.StatusPartialContent, Header: header, ContentLength: 6, Body: io.NopCloser(bytes.NewReader(data[4:]))}, nil
		default:
			return &http.Response{StatusCode: http.StatusInternalServerError, Header: header, ContentLength: 0, Body: io.NopCloser(strings.NewReader("unexpected request"))}, nil
		}
	}))

	target := filepath.Join(t.TempDir(), "upgrade.bin")
	upgrade := clientUpgradeCommand{
		DownloadURL: "http://upgrade.test/package",
		Sha256:      fmt.Sprintf("%x", digest[:]),
		Size:        int64(len(data)),
	}
	err := downloadUpgradeBinary(clientConfig{}, upgrade, target, nil)
	if err != nil {
		t.Fatalf("download with resume: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read resumed target: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("resumed data = %q, want %q", got, data)
	}
	if got := requestedRange.Load(); got != "bytes=4-" {
		t.Fatalf("resume Range = %q, want bytes=4-", got)
	}
	if _, err := os.Stat(upgradePartialPath(target, upgrade)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial file remains after successful promotion: %v", err)
	}
}

func TestDownloadUpgradeBinaryRestartsWhenRangeIsIgnored(t *testing.T) {
	data := []byte("abcdef")
	var requestedRange atomic.Value
	withUpgradeRoundTripper(t, upgradeRoundTripper(func(r *http.Request) (*http.Response, error) {
		requestedRange.Store(r.Header.Get("Range"))
		header := make(http.Header)
		header.Set("Content-Length", strconv.Itoa(len(data)))
		return &http.Response{StatusCode: http.StatusOK, Header: header, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}, nil
	}))

	target := filepath.Join(t.TempDir(), "upgrade.bin")
	upgrade := clientUpgradeCommand{
		DownloadURL: "http://upgrade.test/package",
		Sha256:      fmt.Sprintf("%x", sha256.Sum256(data)),
		Size:        int64(len(data)),
	}
	if err := os.WriteFile(upgradePartialPath(target, upgrade), data[:3], 0o700); err != nil {
		t.Fatalf("seed partial package: %v", err)
	}
	err := downloadUpgradeBinary(clientConfig{}, upgrade, target, nil)
	if err != nil {
		t.Fatalf("download after ignored Range: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restarted target: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("restarted data = %q, want %q", got, data)
	}
	if got := requestedRange.Load(); got != "bytes=3-" {
		t.Fatalf("initial resume Range = %q, want bytes=3-", got)
	}
}

type upgradeRoundTripper func(*http.Request) (*http.Response, error)

func (f upgradeRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type failingUpgradeBody struct {
	reader *bytes.Reader
	failed bool
}

func (b *failingUpgradeBody) Read(data []byte) (int, error) {
	if b.failed {
		return 0, io.EOF
	}
	b.failed = true
	n, _ := b.reader.Read(data)
	return n, io.ErrUnexpectedEOF
}

func (b *failingUpgradeBody) Close() error { return nil }

func withUpgradeRoundTripper(t *testing.T, roundTripper http.RoundTripper) {
	t.Helper()
	previous := http.DefaultClient
	client := *previous
	client.Transport = roundTripper
	http.DefaultClient = &client
	t.Cleanup(func() { http.DefaultClient = previous })
}

func TestWriteRainSystemdServiceCreatesInstallService(t *testing.T) {
	serviceDir := t.TempDir()
	oldDir := systemdServiceDir
	systemdServiceDir = serviceDir
	t.Cleanup(func() {
		systemdServiceDir = oldDir
	})

	if err := writeRainSystemdService("raind", "/mnt/navfirst/nav-rain-go", "navRainApp"); err != nil {
		t.Fatalf("write rain systemd service: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(serviceDir, "raind.service"))
	if err != nil {
		t.Fatalf("read rain service: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"Description=navfirst rain predict for go",
		"Type=idle",
		"ExecStartPre=/bin/sleep 5",
		"ExecStart=/mnt/navfirst/nav-rain-go/navRainApp",
		"WorkingDirectory=/mnt/navfirst/nav-rain-go",
		"RestartSec=10s",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("service content missing %q:\n%s", want, text)
		}
	}
	servicePath := filepath.Join(serviceDir, "raind.service")
	if err := os.WriteFile(servicePath, []byte("custom service\n"), 0o600); err != nil {
		t.Fatalf("replace rain service fixture: %v", err)
	}
	if err := writeRainSystemdService("raind", "/different", "different-app"); err != nil {
		t.Fatalf("preserve existing rain service: %v", err)
	}
	preserved, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("read preserved rain service: %v", err)
	}
	if string(preserved) != "custom service\n" {
		t.Fatalf("existing rain service was overwritten: %q", preserved)
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

func TestDefaultTCPDataChannelsKeepsConnectionPoolBounded(t *testing.T) {
	if defaultTCPDataChannels < 4 || defaultTCPDataChannels > 8 {
		t.Fatalf("defaultTCPDataChannels = %d, want between 4 and 8", defaultTCPDataChannels)
	}
}

func TestCollectHeartbeatTelemetryReusesSnapshotUntilInterval(t *testing.T) {
	heartbeatTelemetryState.Lock()
	previous := heartbeatTelemetryState.item
	heartbeatTelemetryState.item = heartbeatTelemetry{}
	heartbeatTelemetryState.Unlock()
	t.Cleanup(func() {
		heartbeatTelemetryState.Lock()
		heartbeatTelemetryState.item = previous
		heartbeatTelemetryState.Unlock()
	})

	calls := 0
	restoreOutboundInterfaceDetector(t, func(string) string { return "" })
	restoreNetworkSignalDetector(t, func(iface string) networkSnapshot {
		calls++
		return networkSnapshot{NetworkType: "ethernet", NetworkIface: iface}
	})
	cfg := clientConfig{API: "https://example.com", TrafficIface: "none", Telemetry: 5 * time.Minute}
	start := time.Unix(1000, 0)

	first := collectHeartbeatTelemetry(context.Background(), cfg, start)
	second := collectHeartbeatTelemetry(context.Background(), cfg, start.Add(time.Minute))
	third := collectHeartbeatTelemetry(context.Background(), cfg, start.Add(5*time.Minute))

	if calls != 2 {
		t.Fatalf("network collection calls = %d, want 2", calls)
	}
	if !first.sampledAt.Equal(second.sampledAt) {
		t.Fatalf("cached sample time changed: first=%s second=%s", first.sampledAt, second.sampledAt)
	}
	if !third.sampledAt.Equal(start.Add(5 * time.Minute)) {
		t.Fatalf("refreshed sample time = %s", third.sampledAt)
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

func TestDetectNetworkSignalTreatsWlanIfaceAsCellularWhenOnlyCellularSignalExists(t *testing.T) {
	restoreWirelessCapabilityProbe(t, func(string) bool { return true })
	snapshot := detectNetworkSignalWithCollectors(
		"wlan0",
		func(iface string) (networkSnapshot, bool) {
			return networkSnapshot{
				NetworkType:  "cellular",
				NetworkIface: iface,
				CellularRSRP: -92,
				CellularRSRQ: -10,
				CellularSINR: 16,
			}, true
		},
		func(string) (networkSnapshot, bool) {
			return networkSnapshot{}, false
		},
	)

	if snapshot.NetworkType != "cellular" || snapshot.NetworkIface != "wlan0" {
		t.Fatalf("detectNetworkSignalWithCollectors() = type=%q iface=%q, want cellular wlan0", snapshot.NetworkType, snapshot.NetworkIface)
	}
	if snapshot.CellularRSRP != -92 || snapshot.SignalDBM != -92 || snapshot.SignalPct == 0 {
		t.Fatalf("detectNetworkSignalWithCollectors() cellular metrics = rsrp=%d signal=%d pct=%d, want derived cellular signal", snapshot.CellularRSRP, snapshot.SignalDBM, snapshot.SignalPct)
	}
}

func TestDetectNetworkSignalPrefersCellularWhenWlanWifiHasNoAssociation(t *testing.T) {
	restoreWirelessCapabilityProbe(t, func(string) bool { return true })
	snapshot := detectNetworkSignalWithCollectors(
		"wlan0",
		func(iface string) (networkSnapshot, bool) {
			return networkSnapshot{
				NetworkType:  "cellular",
				NetworkIface: iface,
			}, true
		},
		func(iface string) (networkSnapshot, bool) {
			return networkSnapshot{
				NetworkType:  "wifi",
				NetworkIface: iface,
				SignalDBM:    -67,
				SignalPct:    72,
				WifiRSSI:     -67,
			}, true
		},
	)

	if snapshot.NetworkType != "cellular" || snapshot.NetworkIface != "wlan0" {
		t.Fatalf("detectNetworkSignalWithCollectors() = type=%q iface=%q, want cellular wlan0", snapshot.NetworkType, snapshot.NetworkIface)
	}
}

func TestDetectNetworkSignalTreatsWlanIfaceWithoutWirelessCapabilitiesAsCellular(t *testing.T) {
	restoreWirelessCapabilityProbe(t, func(string) bool { return false })
	snapshot := detectNetworkSignalWithCollectors(
		"wlan0",
		func(string) (networkSnapshot, bool) {
			return networkSnapshot{}, false
		},
		func(string) (networkSnapshot, bool) {
			return networkSnapshot{}, false
		},
	)

	if snapshot.NetworkType != "cellular" || snapshot.NetworkIface != "wlan0" {
		t.Fatalf("detectNetworkSignalWithCollectors() = type=%q iface=%q, want cellular wlan0", snapshot.NetworkType, snapshot.NetworkIface)
	}
}

func TestDetectNetworkSignalKeepsAssociatedWlanWifi(t *testing.T) {
	restoreWirelessCapabilityProbe(t, func(string) bool { return true })
	snapshot := detectNetworkSignalWithCollectors(
		"wlan0",
		func(string) (networkSnapshot, bool) {
			t.Fatal("cellular collector should not be used for associated wifi")
			return networkSnapshot{}, false
		},
		func(iface string) (networkSnapshot, bool) {
			return networkSnapshot{
				NetworkType:  "wifi",
				NetworkIface: iface,
				WifiSSID:     "site-wifi",
				WifiRSSI:     -61,
			}, true
		},
	)

	if snapshot.NetworkType != "wifi" || snapshot.NetworkIface != "wlan0" || snapshot.WifiSSID != "site-wifi" {
		t.Fatalf("detectNetworkSignalWithCollectors() = %+v, want associated wifi wlan0", snapshot)
	}
}

func TestWifiAssociationEvidenceIgnoresUnassociatedSSID(t *testing.T) {
	snapshot := networkSnapshot{NetworkType: "wifi", NetworkIface: "wlan0", WifiSSID: " off/any "}
	if hasWifiAssociationEvidence(snapshot) {
		t.Fatal("hasWifiAssociationEvidence() = true, want false for off/any")
	}
	if got := snapshot.normalized().WifiSSID; got != "" {
		t.Fatalf("normalized WifiSSID = %q, want empty", got)
	}
}

func TestDetectNetworkSignalKeepsWlanIfaceAsWifiWhenWirelessCapabilitiesExist(t *testing.T) {
	restoreWirelessCapabilityProbe(t, func(string) bool { return true })
	snapshot := detectNetworkSignalWithCollectors(
		"wlan0",
		func(string) (networkSnapshot, bool) {
			return networkSnapshot{}, false
		},
		func(string) (networkSnapshot, bool) {
			return networkSnapshot{}, false
		},
	)

	if snapshot.NetworkType != "wifi" || snapshot.NetworkIface != "wlan0" {
		t.Fatalf("detectNetworkSignalWithCollectors() = type=%q iface=%q, want wifi wlan0", snapshot.NetworkType, snapshot.NetworkIface)
	}
}

func TestDetectNetworkSignalDoesNotPromoteGlobalCellularEvidenceForEthernet(t *testing.T) {
	snapshot := detectNetworkSignalWithCollectors(
		"eth0",
		func(string) (networkSnapshot, bool) {
			return networkSnapshot{
				NetworkType:  "cellular",
				NetworkIface: "eth0",
				CellularRSRP: -88,
			}, true
		},
		func(string) (networkSnapshot, bool) {
			return networkSnapshot{}, false
		},
	)

	if snapshot.NetworkType != "ethernet" || snapshot.NetworkIface != "eth0" {
		t.Fatalf("detectNetworkSignalWithCollectors() = type=%q iface=%q, want ethernet eth0", snapshot.NetworkType, snapshot.NetworkIface)
	}
}

func TestCellularModemEvidenceRecognizesOperatorOutput(t *testing.T) {
	out := `
  -------------------------
  3GPP | operator name: China Mobile
       | access tech: lte
       | registration state: home
`
	if !hasCellularModemEvidence(out) {
		t.Fatal("hasCellularModemEvidence() = false, want true for operator output")
	}
}

func TestCellularModemEvidenceIgnoresDisabledStateOnly(t *testing.T) {
	out := `
  -------------------------
  Status | state: disabled
`
	if hasCellularModemEvidence(out) {
		t.Fatal("hasCellularModemEvidence() = true, want false for disabled state only")
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

func TestNetworkSnapshotDerivesSignalFromWifiRSSI(t *testing.T) {
	snapshot := networkSnapshot{
		NetworkType:  "wifi",
		NetworkIface: "wlan0",
		WifiRSSI:     -70,
	}.normalized()

	if snapshot.SignalDBM != -70 {
		t.Fatalf("signalDbm = %d, want wifi rssi -70", snapshot.SignalDBM)
	}
	if snapshot.SignalPct != 66 {
		t.Fatalf("signalPct = %d, want derived percent 66", snapshot.SignalPct)
	}
}

func TestCollectNetworkSnapshotPrefersOutboundInterface(t *testing.T) {
	restoreOutboundInterfaceDetector(t, func(string) string { return "eth0" })
	restoreNetworkSignalDetector(t, func(iface string) networkSnapshot {
		return networkSnapshot{NetworkType: inferNetworkType(iface), NetworkIface: iface}
	})

	snapshot := collectNetworkSnapshot(context.Background(), clientConfig{}, trafficSnapshot{
		Interface:  "wwan0",
		RXBytes:    1000,
		TXBytes:    2000,
		SampleTime: 1000,
		BootID:     "boot-1",
	})

	if snapshot.NetworkType != "ethernet" || snapshot.NetworkIface != "eth0" {
		t.Fatalf("collectNetworkSnapshot() = type=%q iface=%q, want ethernet eth0", snapshot.NetworkType, snapshot.NetworkIface)
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

func restoreWirelessCapabilityProbe(t *testing.T, probe func(string) bool) {
	t.Helper()
	previous := interfaceHasWirelessCapabilities
	interfaceHasWirelessCapabilities = probe
	t.Cleanup(func() {
		interfaceHasWirelessCapabilities = previous
	})
}

func restoreOutboundInterfaceDetector(t *testing.T, probe func(string) string) {
	t.Helper()
	previous := outboundInterfaceDetector
	outboundInterfaceDetector = probe
	t.Cleanup(func() {
		outboundInterfaceDetector = previous
	})
}

func restoreNetworkSignalDetector(t *testing.T, probe func(string) networkSnapshot) {
	t.Helper()
	previous := networkSignalDetector
	networkSignalDetector = probe
	t.Cleanup(func() {
		networkSignalDetector = previous
	})
}
