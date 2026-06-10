package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"navmesh-go/tunnel"

	"github.com/quic-go/quic-go"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

const (
	clientVersion          = "v0.0.2"
	clientTransportAuto    = "auto"
	defaultTCPDataChannels = 32
)

var serviceLogNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]+\.service$`)

type clientConfig struct {
	Server         string
	Port           int
	API            string
	Token          string
	DeviceToken    string
	StateFile      string
	DeviceGuid     string
	Sncode         string
	DeviceType     string
	Alias          string
	Remark         string
	SSHPort        int
	WebPort        int
	WebDomain      string
	Hostname       string
	HostIP         string
	WanIP          string
	LocalHost      string
	SkipRegister   bool
	InsecureQUIC   bool
	Transport      string
	DataChannels   int
	ReconnectWait  time.Duration
	ReconnectMax   time.Duration
	Heartbeat      time.Duration
	HeartbeatFail  int
	RequestTimeout time.Duration
	ServiceName    string
}

type registerResponse struct {
	Code int `json:"code"`
	Data struct {
		Device struct {
			Guid       string `json:"guid"`
			Sncode     string `json:"sncode"`
			DeviceType string `json:"deviceType"`
			Alias      string `json:"alias"`
			Remark     string `json:"remark"`
		} `json:"device"`
		DeviceToken struct {
			Token string `json:"token"`
			Item  struct {
				Guid string `json:"guid"`
			} `json:"item"`
		} `json:"deviceToken"`
		SSH struct {
			Alias struct {
				Domain       string `json:"domain"`
				EntrypointIP string `json:"entrypointIp"`
			} `json:"alias"`
			EntrypointIP string `json:"entrypointIp"`
			Ready        bool   `json:"ready"`
		} `json:"ssh"`
	} `json:"data"`
	Msg string `json:"msg"`
}

type apiResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Guid       string                   `json:"guid"`
		Sncode     string                   `json:"sncode"`
		DeviceType string                   `json:"deviceType"`
		Alias      string                   `json:"alias"`
		Remark     string                   `json:"remark"`
		Upgrade    *clientUpgradeCommand    `json:"upgrade"`
		VPNRestart *clientVPNRestartCommand `json:"vpnRestart"`
	} `json:"data"`
}

type clientUpgradeCommand struct {
	TaskGuid    string `json:"taskGuid"`
	Version     string `json:"version"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	FileName    string `json:"fileName"`
	DownloadURL string `json:"downloadUrl"`
	Sha256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

type clientVPNRestartCommand struct {
	RequestedAt int64  `json:"requestedAt"`
	Message     string `json:"message"`
}

type clientState struct {
	Sncode      string `json:"sncode"`
	DeviceType  string `json:"deviceType"`
	DeviceGuid  string `json:"deviceGuid"`
	DeviceToken string `json:"deviceToken"`
	TokenGuid   string `json:"tokenGuid"`
	Alias       string `json:"alias"`
	Remark      string `json:"remark"`
	UpdateTime  int64  `json:"updateTime"`
}

type systemSnapshot struct {
	OS          string
	OSVersion   string
	Kernel      string
	Arch        string
	MemoryTotal int64
	MemoryUsed  int64
	MemoryFree  int64
	DiskTotal   int64
	DiskUsed    int64
	DiskFree    int64
	DiskUsedPct float64
}

var upgradeState = struct {
	sync.Mutex
	taskGuid string
}{}

var vpnRestartState = struct {
	sync.Mutex
	lastRequestedAt int64
	ch              chan clientVPNRestartCommand
}{ch: make(chan clientVPNRestartCommand, 1)}

var errVPNRestartRequested = errors.New("vpn restart requested")

type clientUpgradeReporter struct {
	cfg            clientConfig
	upgrade        clientUpgradeCommand
	progress       int
	downloadedSize int64
}

type upgradeProgressReader struct {
	reader io.Reader
	onRead func(int64)
}

func main() {
	cfg := parseFlags()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		log.Fatalf("navmesh-client stopped: %v", err)
	}
}

func parseFlags() clientConfig {
	hostname, _ := os.Hostname()
	cfg := clientConfig{}
	showVersion := false
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		_, _ = fmt.Fprintf(out, "NavMesh 设备侧客户端 %s\n\n", clientVersion)
		_, _ = fmt.Fprintln(out, "用法:")
		_, _ = fmt.Fprintln(out, "  navmesh-client [参数]")
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "参数:")
		flag.PrintDefaults()
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "示例:")
		_, _ = fmt.Fprintln(out, "  navmesh-client -server tunnel.navfirst.com -port 3008 -token navfirst@2020")
	}
	flag.StringVar(&cfg.Server, "server", "navmesh.navfirst.com", "NavMesh 隧道服务器地址")
	flag.IntVar(&cfg.Port, "port", 3008, "NavMesh 隧道服务器 UDP 端口")
	flag.StringVar(&cfg.API, "api", "https://navmesh.navfirst.com", "NavMesh 管理 API 基础地址，默认 http://<server>:3007")
	flag.StringVar(&cfg.Token, "token", "navfirst@2020", "首次注册令牌；注册成功后客户端会保存并改用设备独立 Token")
	flag.StringVar(&cfg.DeviceToken, "deviceToken", "", "设备独立 Token；状态文件丢失时可由管理端重新生成后填入")
	flag.StringVar(&cfg.StateFile, "stateFile", "", "客户端状态文件路径，默认 navmesh-client 同级目录 navmesh-client.json")
	flag.StringVar(&cfg.Sncode, "sncode", "", "设备 SN 编码，全局唯一")
	flag.StringVar(&cfg.DeviceType, "type", "ssh", "设备类型，默认 ssh；注册后可在管理端修改")
	flag.StringVar(&cfg.Alias, "alias", "", "设备别名，默认由服务端使用 sncode")
	flag.StringVar(&cfg.Remark, "remark", "", "设备备注")
	flag.IntVar(&cfg.SSHPort, "sshPort", 22, "本机 SSH 服务端口")
	flag.IntVar(&cfg.WebPort, "webPort", 0, "本机 Web 服务端口，0 表示不启用")
	flag.StringVar(&cfg.WebDomain, "webDomain", "", "外部 Web 映射域名")
	flag.StringVar(&cfg.Hostname, "hostname", hostname, "上报的主机名")
	flag.StringVar(&cfg.HostIP, "hostIp", "", "上报的主机IP，默认自动探测")
	flag.StringVar(&cfg.WanIP, "wanIp", "", "上报的外网IP，默认自动探测")
	flag.StringVar(&cfg.LocalHost, "localHost", "127.0.0.1", "服务端请求回环目标时使用的本机地址")
	flag.BoolVar(&cfg.SkipRegister, "skipRegister", false, "跳过HTTP设备注册，直接建立隧道")
	flag.BoolVar(&cfg.InsecureQUIC, "insecure", true, "跳过QUIC服务器证书校验")
	flag.StringVar(&cfg.Transport, "transport", clientTransportAuto, "隧道传输协议：auto、quic 或 tcp")
	flag.IntVar(&cfg.DataChannels, "dataChannels", defaultTCPDataChannels, "TCP 隧道数据连接池大小")
	flag.BoolVar(&showVersion, "v", false, "查看当前客户端版本")
	flag.DurationVar(&cfg.ReconnectWait, "reconnectWait", 5*time.Second, "首次重连等待时间")
	flag.DurationVar(&cfg.ReconnectMax, "reconnectMax", 60*time.Second, "指数退避最大重连等待时间")
	flag.DurationVar(&cfg.Heartbeat, "heartbeat", 30*time.Second, "QUIC 和 HTTP 心跳间隔")
	flag.IntVar(&cfg.HeartbeatFail, "heartbeatFail", 3, "连续心跳失败多少次后主动断开并重连")
	flag.DurationVar(&cfg.RequestTimeout, "requestTimeout", 10*time.Second, "HTTP 请求和本地端口连接超时时间")
	flag.StringVar(&cfg.ServiceName, "serviceName", "navmesh-client", "systemd 服务名称，用于客户端在线升级后重启")
	flag.Parse()
	if showVersion {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), clientVersion)
		os.Exit(0)
	}

	cfg.Server = strings.TrimSpace(cfg.Server)
	cfg.API = strings.TrimRight(strings.TrimSpace(cfg.API), "/")
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.DeviceToken = strings.TrimSpace(cfg.DeviceToken)
	if cfg.API == "" {
		cfg.API = "http://" + net.JoinHostPort(cfg.Server, "3007")
	}
	if cfg.StateFile == "" {
		cfg.StateFile = defaultStateFile()
	}
	if cfg.HostIP == "" {
		cfg.HostIP = detectOutboundIP()
	}
	cfg.WanIP = normalizeIP(cfg.WanIP)
	if cfg.WanIP == "" {
		cfg.WanIP = detectPublicIP(3 * time.Second)
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = 5 * time.Second
	}
	if cfg.ReconnectMax < cfg.ReconnectWait {
		cfg.ReconnectMax = cfg.ReconnectWait
	}
	if cfg.Heartbeat <= 0 {
		cfg.Heartbeat = 30 * time.Second
	}
	if cfg.HeartbeatFail <= 0 {
		cfg.HeartbeatFail = 3
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 10 * time.Second
	}
	cfg.Transport = normalizeTransport(cfg.Transport)
	if cfg.DataChannels <= 0 {
		cfg.DataChannels = defaultTCPDataChannels
	}
	return cfg
}

func normalizeTransport(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", clientTransportAuto:
		return clientTransportAuto
	case tunnel.TransportQUIC:
		return tunnel.TransportQUIC
	case tunnel.TransportTCP:
		return tunnel.TransportTCP
	default:
		return clientTransportAuto
	}
}

func run(ctx context.Context, cfg clientConfig) error {
	if cfg.Token == "" {
		return fmt.Errorf("token required")
	}
	state, err := loadClientState(cfg.StateFile)
	if err != nil {
		log.Printf("load client state failed path=%s err=%v", cfg.StateFile, err)
	}
	if cfg.Sncode == "" {
		cfg.Sncode = strings.TrimSpace(state.Sncode)
	}
	if cfg.Sncode == "" {
		cfg.Sncode = generateSncode(cfg.Hostname)
	}
	if cfg.DeviceType == "" {
		cfg.DeviceType = strings.TrimSpace(state.DeviceType)
	}
	if cfg.DeviceType == "" {
		cfg.DeviceType = "ssh"
	}
	if cfg.DeviceGuid == "" {
		cfg.DeviceGuid = strings.TrimSpace(state.DeviceGuid)
	}
	if cfg.DeviceToken == "" && state.DeviceToken != "" {
		cfg.DeviceToken = state.DeviceToken
		log.Printf("loaded device token state path=%s sncode=%s", cfg.StateFile, cfg.Sncode)
	} else if cfg.DeviceToken != "" {
		log.Printf("using device token from command line sncode=%s stateFile=%s", cfg.Sncode, cfg.StateFile)
	}
	if cfg.Alias == "" && strings.TrimSpace(state.Alias) != "" {
		cfg.Alias = strings.TrimSpace(state.Alias)
	}
	if cfg.Remark == "" && strings.TrimSpace(state.Remark) != "" {
		cfg.Remark = strings.TrimSpace(state.Remark)
	}
	if strings.TrimSpace(state.Sncode) == "" {
		_ = saveClientState(cfg.StateFile, clientState{
			Sncode:      cfg.Sncode,
			DeviceType:  cfg.DeviceType,
			DeviceGuid:  cfg.DeviceGuid,
			DeviceToken: cfg.DeviceToken,
			Alias:       cfg.Alias,
			Remark:      cfg.Remark,
			UpdateTime:  time.Now().UnixMilli(),
		})
	}
	snapshot := collectSystemSnapshot()
	log.Printf(
		"navmesh-client starting version=%s sncode=%s type=%s guid=%s server=%s port=%d api=%s hostname=%s hostIp=%s wanIp=%s os=%s osVersion=%s kernel=%s arch=%s memoryTotal=%d diskTotal=%d sshPort=%d webPort=%d heartbeat=%s transport=%s dataChannels=%d stateFile=%s skipRegister=%t",
		clientVersion,
		cfg.Sncode,
		cfg.DeviceType,
		cfg.DeviceGuid,
		cfg.Server,
		cfg.Port,
		cfg.API,
		cfg.Hostname,
		cfg.HostIP,
		cfg.WanIP,
		snapshot.OS,
		snapshot.OSVersion,
		snapshot.Kernel,
		snapshot.Arch,
		snapshot.MemoryTotal,
		snapshot.DiskTotal,
		cfg.SSHPort,
		cfg.WebPort,
		cfg.Heartbeat,
		cfg.Transport,
		cfg.DataChannels,
		cfg.StateFile,
		cfg.SkipRegister,
	)
	failures := 0
	for {
		if !cfg.SkipRegister {
			nextCfg, err := registerDevice(ctx, cfg)
			if err != nil {
				if shouldRetryWithRegisterToken(err, cfg) {
					log.Printf("device token rejected, retrying register with default token: %v", err)
					cfg.DeviceToken = ""
					_ = saveClientState(cfg.StateFile, clientState{
						Sncode:     cfg.Sncode,
						DeviceType: cfg.DeviceType,
						DeviceGuid: cfg.DeviceGuid,
						Alias:      cfg.Alias,
						Remark:     cfg.Remark,
						UpdateTime: time.Now().UnixMilli(),
					})
					continue
				}
				log.Printf("register device failed: %v", err)
				failures++
				if err := waitReconnect(ctx, cfg, failures); err != nil {
					return err
				}
				continue
			}
			cfg = nextCfg
		}
		if strings.TrimSpace(cfg.DeviceToken) == "" {
			log.Printf("device registered and waiting for activation sncode=%s", cfg.Sncode)
			if err := waitActivationPoll(ctx, cfg); err != nil {
				return err
			}
			continue
		}
		failures = 0
		if err := connectAndServe(ctx, cfg); err != nil {
			log.Printf("tunnel disconnected: %v", err)
			if errors.Is(err, errVPNRestartRequested) {
				failures = 0
				continue
			}
		}
		failures++
		if err := waitReconnect(ctx, cfg, failures); err != nil {
			return err
		}
	}
}

func shouldRetryWithRegisterToken(err error, cfg clientConfig) bool {
	if err == nil || strings.TrimSpace(cfg.DeviceToken) == "" {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid register token") ||
		strings.Contains(msg, "invalid device token") ||
		strings.Contains(msg, "device not found")
}

func waitActivationPoll(ctx context.Context, cfg clientConfig) error {
	delay := cfg.ReconnectWait
	if delay <= 0 || delay > 5*time.Second {
		delay = 5 * time.Second
	}
	log.Printf("activation poll scheduled in %s sncode=%s", delay, cfg.Sncode)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func waitReconnect(ctx context.Context, cfg clientConfig, failures int) error {
	delay := backoffDelay(cfg, failures)
	log.Printf("reconnect scheduled in %s failures=%d", delay, failures)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func backoffDelay(cfg clientConfig, failures int) time.Duration {
	if failures <= 1 {
		return cfg.ReconnectWait
	}
	delay := cfg.ReconnectWait
	for i := 1; i < failures; i++ {
		delay *= 2
		if delay >= cfg.ReconnectMax {
			return cfg.ReconnectMax
		}
	}
	return delay
}

func registerDevice(ctx context.Context, cfg clientConfig) (clientConfig, error) {
	snapshot := collectSystemSnapshot()
	log.Printf(
		"registering device api=%s sncode=%s type=%s alias=%s hostname=%s hostIp=%s wanIp=%s os=%s kernel=%s memoryUsed=%d diskUsed=%d sshPort=%d webPort=%d auth=%s",
		cfg.API,
		cfg.Sncode,
		cfg.DeviceType,
		cfg.Alias,
		cfg.Hostname,
		cfg.HostIP,
		cfg.WanIP,
		snapshot.OS,
		snapshot.Kernel,
		snapshot.MemoryUsed,
		snapshot.DiskUsed,
		cfg.SSHPort,
		cfg.WebPort,
		cfg.authMode(),
	)
	body := map[string]any{
		"token":         cfg.authToken(),
		"guid":          cfg.DeviceGuid,
		"sncode":        cfg.Sncode,
		"type":          cfg.DeviceType,
		"alias":         cfg.Alias,
		"remark":        cfg.Remark,
		"hostname":      cfg.Hostname,
		"hostIp":        cfg.HostIP,
		"wanIp":         cfg.WanIP,
		"clientVersion": clientVersion,
		"sshPort":       cfg.SSHPort,
		"webPort":       cfg.WebPort,
		"webDomain":     cfg.WebDomain,
	}
	snapshot.addTo(body)
	data, _ := json.Marshal(body)
	reqCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.API+"/api/device/register", bytes.NewReader(data))
	if err != nil {
		return cfg, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return cfg, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return cfg, fmt.Errorf("register status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var result registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return cfg, err
	}
	if result.Code != 200 {
		return cfg, fmt.Errorf("%s", result.Msg)
	}
	if result.Data.DeviceToken.Token != "" {
		cfg.DeviceToken = result.Data.DeviceToken.Token
	}
	if result.Data.Device.Guid != "" {
		cfg.DeviceGuid = result.Data.Device.Guid
	}
	if result.Data.Device.Sncode != "" {
		cfg.Sncode = result.Data.Device.Sncode
	}
	if result.Data.Device.DeviceType != "" {
		cfg.DeviceType = result.Data.Device.DeviceType
	}
	if result.Data.Device.Alias != "" {
		cfg.Alias = result.Data.Device.Alias
	}
	cfg.Remark = result.Data.Device.Remark
	state := clientState{
		Sncode:      cfg.Sncode,
		DeviceType:  cfg.DeviceType,
		DeviceGuid:  cfg.DeviceGuid,
		DeviceToken: cfg.DeviceToken,
		TokenGuid:   result.Data.DeviceToken.Item.Guid,
		Alias:       cfg.Alias,
		Remark:      cfg.Remark,
		UpdateTime:  time.Now().UnixMilli(),
	}
	if err := saveClientState(cfg.StateFile, state); err != nil {
		log.Printf("save client state failed path=%s err=%v", cfg.StateFile, err)
	} else {
		log.Printf("saved device state path=%s sncode=%s alias=%s", cfg.StateFile, cfg.Sncode, cfg.Alias)
	}
	if result.Data.SSH.Alias.Domain != "" {
		log.Printf("registered device sncode=%s guid=%s ssh=%s ready=%t entrypoint=%s", cfg.Sncode, result.Data.Device.Guid, result.Data.SSH.Alias.Domain, result.Data.SSH.Ready, result.Data.SSH.EntrypointIP)
	} else {
		log.Printf("registered device sncode=%s guid=%s", cfg.Sncode, result.Data.Device.Guid)
	}
	return cfg, nil
}

func connectAndServe(ctx context.Context, cfg clientConfig) error {
	switch cfg.Transport {
	case tunnel.TransportTCP:
		return connectAndServeTCP(ctx, cfg)
	case tunnel.TransportQUIC:
		return connectAndServeQUIC(ctx, cfg)
	default:
		return connectAndServeAuto(ctx, cfg)
	}
}

func connectAndServeAuto(ctx context.Context, cfg clientConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	quicErr := connectAndServeQUIC(ctx, cfg)
	if err := ctx.Err(); err != nil {
		return err
	}
	if quicErr == nil {
		return nil
	}
	log.Printf("quic tunnel unavailable, falling back to tcp: %v", quicErr)
	tcpErr := connectAndServeTCP(ctx, cfg)
	if err := ctx.Err(); err != nil {
		return err
	}
	if tcpErr == nil {
		return nil
	}
	return fmt.Errorf("auto tunnel failed: quic=%v tcp=%w", quicErr, tcpErr)
}

func connectAndServeQUIC(ctx context.Context, cfg clientConfig) error {
	addr := net.JoinHostPort(cfg.Server, strconv.Itoa(cfg.Port))
	log.Printf("connecting tunnel server=%s sncode=%s insecure=%t heartbeat=%s", addr, cfg.Sncode, cfg.InsecureQUIC, cfg.Heartbeat)
	controlConn, err := quic.DialAddr(ctx, addr, &tls.Config{
		InsecureSkipVerify: cfg.InsecureQUIC,
		NextProtos:         []string{"navmesh-quic"},
		MinVersion:         tls.VersionTLS13,
	}, tunnel.NewQUICConfig(cfg.Heartbeat))
	if err != nil {
		return err
	}
	defer controlConn.CloseWithError(0, "client reconnecting")

	if err := sendHello(ctx, controlConn, cfg, tunnel.RoleControl); err != nil {
		return err
	}
	dataConn, err := quic.DialAddr(ctx, addr, &tls.Config{
		InsecureSkipVerify: cfg.InsecureQUIC,
		NextProtos:         []string{"navmesh-quic"},
		MinVersion:         tls.VersionTLS13,
	}, tunnel.NewQUICConfig(cfg.Heartbeat))
	if err != nil {
		return err
	}
	defer dataConn.CloseWithError(0, "client reconnecting")
	if err := sendHello(ctx, dataConn, cfg, tunnel.RoleData); err != nil {
		return err
	}
	snapshot := collectSystemSnapshot()
	log.Printf("tunnel connected server=%s sncode=%s hostname=%s hostIp=%s wanIp=%s %s", addr, cfg.Sncode, cfg.Hostname, cfg.HostIP, cfg.WanIP, snapshot.logFields())

	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	heartbeatErr := make(chan error, 2)
	go heartbeatLoop(heartbeatCtx, controlConn, cfg, heartbeatErr)
	go quicTunnelHeartbeatLoop(heartbeatCtx, dataConn, cfg, tunnel.RoleData, heartbeatErr)

	var wg sync.WaitGroup
	defer wg.Wait()
	acceptErr := make(chan error, 1)
	go func() {
		for {
			stream, err := dataConn.AcceptStream(ctx)
			if err != nil {
				acceptErr <- err
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				handleStream(ctx, cfg, stream)
			}()
		}
	}()

	for {
		select {
		case err := <-acceptErr:
			return err
		case err := <-heartbeatErr:
			_ = controlConn.CloseWithError(2, "heartbeat failed")
			_ = dataConn.CloseWithError(2, "heartbeat failed")
			return err
		case <-ctx.Done():
			_ = controlConn.CloseWithError(0, "client stopping")
			_ = dataConn.CloseWithError(0, "client stopping")
			return ctx.Err()
		}
	}
}

func sendHello(ctx context.Context, conn *quic.Conn, cfg clientConfig, role string) error {
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	frame := tunnel.Frame{
		Type:          tunnel.FrameTypeHello,
		Role:          role,
		Transport:     tunnel.TransportQUIC,
		Token:         cfg.authToken(),
		DeviceGuid:    cfg.DeviceGuid,
		SnCode:        cfg.Sncode,
		HostIP:        cfg.HostIP,
		WanIP:         cfg.WanIP,
		Hostname:      cfg.Hostname,
		ClientVersion: clientVersion,
	}
	if err := writeFrame(stream, frame); err != nil {
		return err
	}
	ack, err := readFrame(stream)
	if err != nil {
		return err
	}
	if ack.Type != tunnel.FrameTypeHelloAck || !ack.OK {
		return fmt.Errorf("hello rejected: %s", ack.Message)
	}
	log.Printf("hello accepted sncode=%s message=%s", cfg.Sncode, ack.Message)
	return nil
}

func connectAndServeTCP(ctx context.Context, cfg clientConfig) error {
	addr := net.JoinHostPort(cfg.Server, strconv.Itoa(cfg.Port))
	log.Printf("connecting tcp tunnel server=%s sncode=%s heartbeat=%s dataChannels=%d", addr, cfg.Sncode, cfg.Heartbeat, cfg.DataChannels)
	control, err := (&net.Dialer{Timeout: cfg.RequestTimeout, KeepAlive: cfg.Heartbeat}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer control.Close()
	if err := sendTCPHello(control, cfg, tunnel.RoleControl); err != nil {
		return err
	}
	dataCtx, cancelData := context.WithCancel(ctx)
	defer cancelData()
	dataErr := make(chan error, cfg.DataChannels)
	for i := 0; i < cfg.DataChannels; i++ {
		go tcpDataLoop(dataCtx, cfg, i, dataErr)
	}
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	heartbeatErr := make(chan error, 1)
	go tcpHeartbeatLoop(heartbeatCtx, control, cfg, heartbeatErr)
	snapshot := collectSystemSnapshot()
	log.Printf("tcp tunnel connected server=%s sncode=%s hostname=%s hostIp=%s wanIp=%s %s", addr, cfg.Sncode, cfg.Hostname, cfg.HostIP, cfg.WanIP, snapshot.logFields())
	for {
		select {
		case err := <-heartbeatErr:
			return err
		case err := <-dataErr:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("tcp data channel exited: %v", err)
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func sendTCPHello(conn net.Conn, cfg clientConfig, role string) error {
	_ = conn.SetDeadline(time.Now().Add(cfg.RequestTimeout))
	frame := tunnel.Frame{
		Type:          tunnel.FrameTypeHello,
		Role:          role,
		Transport:     tunnel.TransportTCP,
		Token:         cfg.authToken(),
		DeviceGuid:    cfg.DeviceGuid,
		SnCode:        cfg.Sncode,
		HostIP:        cfg.HostIP,
		WanIP:         cfg.WanIP,
		Hostname:      cfg.Hostname,
		ClientVersion: clientVersion,
	}
	if err := writeFrame(conn, frame); err != nil {
		return err
	}
	ack, err := readFrame(conn)
	if err != nil {
		return err
	}
	if ack.Type != tunnel.FrameTypeHelloAck || !ack.OK {
		return fmt.Errorf("hello rejected: %s", ack.Message)
	}
	_ = conn.SetDeadline(time.Time{})
	return nil
}

func tcpDataLoop(ctx context.Context, cfg clientConfig, index int, errCh chan<- error) {
	addr := net.JoinHostPort(cfg.Server, strconv.Itoa(cfg.Port))
	for {
		if ctx.Err() != nil {
			return
		}
		conn, err := (&net.Dialer{Timeout: cfg.RequestTimeout, KeepAlive: cfg.Heartbeat}).DialContext(ctx, "tcp", addr)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}
		if err := sendTCPHello(conn, cfg, tunnel.RoleData); err != nil {
			_ = conn.Close()
			select {
			case errCh <- err:
			default:
			}
			return
		}
		log.Printf("tcp data channel ready index=%d sncode=%s", index, cfg.Sncode)
		if err := handleTCPDataConnection(ctx, cfg, conn); err != nil && ctx.Err() == nil {
			log.Printf("tcp data channel recycled index=%d err=%v", index, err)
		}
	}
}

func handleTCPDataConnection(ctx context.Context, cfg clientConfig, conn net.Conn) error {
	defer conn.Close()
	frame, err := readFrame(conn)
	if err != nil {
		return err
	}
	switch frame.Type {
	case tunnel.FrameTypeOpenTCP:
		return handleOpenTCPFrame(ctx, cfg, conn, frame)
	case tunnel.FrameTypeOpenLog:
		return handleOpenServiceLogFrame(ctx, conn, frame)
	default:
		_ = writeFrame(conn, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, Message: "unsupported frame type"})
		return errors.New("unsupported frame type")
	}
}

func handleOpenTCPFrame(ctx context.Context, cfg clientConfig, conn io.ReadWriteCloser, frame tunnel.Frame) error {
	targetHost := normalizeTargetHost(cfg, frame.TargetHost)
	targetPort := frame.TargetPort
	log.Printf("open tcp request requestId=%s target=%s:%d sncode=%s", frame.RequestID, targetHost, targetPort, cfg.Sncode)
	if targetPort <= 0 {
		_ = writeFrame(conn, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, Message: "invalid target port"})
		return errors.New("invalid target port")
	}
	dialCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	local, err := (&net.Dialer{Timeout: cfg.RequestTimeout}).DialContext(dialCtx, "tcp", net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
	if err != nil {
		_ = writeFrame(conn, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, OK: false, Message: err.Error()})
		return err
	}
	defer local.Close()
	if err := writeFrame(conn, tunnel.Frame{Type: tunnel.FrameTypeOpenTCPAck, RequestID: frame.RequestID, OK: true}); err != nil {
		return err
	}
	bridge(local, conn)
	log.Printf("open tcp closed requestId=%s target=%s:%d", frame.RequestID, targetHost, targetPort)
	return nil
}

func handleOpenServiceLogFrame(ctx context.Context, conn io.ReadWriteCloser, frame tunnel.Frame) error {
	serviceName, tail, err := normalizeServiceLogRequest(frame.ServiceName, frame.Tail)
	if err != nil {
		_ = writeFrame(conn, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, OK: false, Message: err.Error()})
		return err
	}
	if _, err := exec.LookPath("journalctl"); err != nil {
		_ = writeFrame(conn, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, OK: false, Message: "journalctl not found"})
		return err
	}
	if err := writeFrame(conn, tunnel.Frame{Type: tunnel.FrameTypeOpenLogAck, RequestID: frame.RequestID, OK: true}); err != nil {
		return err
	}
	log.Printf("service log opened requestId=%s service=%s tail=%d", frame.RequestID, serviceName, tail)
	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		cancel()
		close(done)
	}()
	cmd := exec.CommandContext(cmdCtx, "journalctl", "-u", serviceName, "-n", strconv.Itoa(tail), "-f", "-o", "short-iso", "--no-pager")
	cmd.Stdout = conn
	cmd.Stderr = conn
	err = cmd.Run()
	cancel()
	_ = conn.Close()
	waitForServiceLogDrain(done)
	if err != nil && ctx.Err() == nil && cmdCtx.Err() == nil {
		log.Printf("service log closed with error service=%s err=%v", serviceName, err)
		return err
	}
	log.Printf("service log closed service=%s", serviceName)
	return nil
}

func waitForServiceLogDrain(done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func tcpHeartbeatLoop(ctx context.Context, conn net.Conn, cfg clientConfig, errCh chan<- error) {
	ticker := time.NewTicker(cfg.Heartbeat)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case cmd := <-vpnRestartState.ch:
			select {
			case errCh <- fmt.Errorf("%w requestedAt=%d message=%s", errVPNRestartRequested, cmd.RequestedAt, cmd.Message):
			default:
			}
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			tcpErr := sendTCPHeartbeat(conn, cfg)
			httpErr := postHeartbeat(ctx, cfg)
			if heartbeatFailed(tcpErr, httpErr) {
				failures++
				log.Printf("tcp heartbeat failed failures=%d tcpErr=%v httpErr=%v", failures, tcpErr, httpErr)
			} else {
				if httpErr != nil {
					log.Printf("http heartbeat failed but tcp tunnel heartbeat ok err=%v", httpErr)
				}
				failures = 0
				snapshot := collectSystemSnapshot()
				log.Printf("heartbeat ok sncode=%s hostname=%s hostIp=%s wanIp=%s interval=%s %s", cfg.Sncode, cfg.Hostname, cfg.HostIP, cfg.WanIP, cfg.Heartbeat, snapshot.logFields())
			}
			if failures >= cfg.HeartbeatFail {
				select {
				case errCh <- fmt.Errorf("heartbeat failed %d times", failures):
				default:
				}
				return
			}
		}
	}
}

func sendTCPHeartbeat(conn net.Conn, cfg clientConfig) error {
	timeout := heartbeatRoundTripTimeout(cfg)
	_ = conn.SetDeadline(time.Now().Add(timeout))
	defer conn.SetDeadline(time.Time{})
	requestID := strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := writeFrame(conn, tunnel.Frame{Type: tunnel.FrameTypeHeartbeat, SnCode: cfg.Sncode, RequestID: requestID}); err != nil {
		return err
	}
	ack, err := readFrame(conn)
	if err != nil {
		return err
	}
	if ack.Type != tunnel.FrameTypePong || !ack.OK {
		return fmt.Errorf("heartbeat rejected: type=%s ok=%t message=%s", ack.Type, ack.OK, ack.Message)
	}
	return nil
}

func heartbeatLoop(ctx context.Context, conn *quic.Conn, cfg clientConfig, errCh chan<- error) {
	ticker := time.NewTicker(cfg.Heartbeat)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case cmd := <-vpnRestartState.ch:
			select {
			case errCh <- fmt.Errorf("%w requestedAt=%d message=%s", errVPNRestartRequested, cmd.RequestedAt, cmd.Message):
			default:
			}
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			quicErr := sendHeartbeat(ctx, conn, cfg)
			httpErr := postHeartbeat(ctx, cfg)
			if heartbeatFailed(quicErr, httpErr) {
				failures++
				log.Printf("heartbeat failed failures=%d quicErr=%v httpErr=%v", failures, quicErr, httpErr)
			} else {
				if httpErr != nil {
					log.Printf("http heartbeat failed but tunnel heartbeat ok err=%v", httpErr)
				}
				failures = 0
				snapshot := collectSystemSnapshot()
				log.Printf("heartbeat ok sncode=%s hostname=%s hostIp=%s wanIp=%s interval=%s %s", cfg.Sncode, cfg.Hostname, cfg.HostIP, cfg.WanIP, cfg.Heartbeat, snapshot.logFields())
			}
			if failures >= cfg.HeartbeatFail {
				select {
				case errCh <- fmt.Errorf("heartbeat failed %d times", failures):
				default:
				}
				return
			}
		}
	}
}

func quicTunnelHeartbeatLoop(ctx context.Context, conn *quic.Conn, cfg clientConfig, name string, errCh chan<- error) {
	ticker := time.NewTicker(cfg.Heartbeat)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := sendHeartbeat(ctx, conn, cfg)
			if err != nil {
				failures++
				log.Printf("%s tunnel heartbeat failed failures=%d err=%v", name, failures, err)
			} else {
				failures = 0
			}
			if failures >= cfg.HeartbeatFail {
				select {
				case errCh <- fmt.Errorf("%s tunnel heartbeat failed %d times: %w", name, failures, err):
				default:
				}
				return
			}
		}
	}
}

func heartbeatFailed(quicErr error, httpErr error) bool {
	return quicErr != nil
}

func sendHeartbeat(ctx context.Context, conn *quic.Conn, cfg clientConfig) error {
	timeout := heartbeatRoundTripTimeout(cfg)
	hbCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stream, err := conn.OpenStreamSync(hbCtx)
	if err != nil {
		return err
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(timeout))

	requestID := strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeHeartbeat, SnCode: cfg.Sncode, RequestID: requestID}); err != nil {
		return err
	}
	ack, err := readFrame(stream)
	if err != nil {
		return err
	}
	if ack.Type != tunnel.FrameTypePong || !ack.OK {
		return fmt.Errorf("heartbeat rejected: type=%s ok=%t message=%s", ack.Type, ack.OK, ack.Message)
	}
	if ack.RequestID != "" && ack.RequestID != requestID {
		return fmt.Errorf("heartbeat response request mismatch: want=%s got=%s", requestID, ack.RequestID)
	}
	return nil
}

func heartbeatRoundTripTimeout(cfg clientConfig) time.Duration {
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if cfg.Heartbeat > 0 {
		limit := cfg.Heartbeat / 2
		if limit <= 0 {
			limit = cfg.Heartbeat
		}
		if timeout > limit {
			timeout = limit
		}
	}
	if timeout < time.Second {
		timeout = time.Second
	}
	return timeout
}

func postHeartbeat(ctx context.Context, cfg clientConfig) error {
	snapshot := collectSystemSnapshot()
	body := map[string]any{
		"token":         cfg.authToken(),
		"guid":          cfg.DeviceGuid,
		"sncode":        cfg.Sncode,
		"hostIp":        cfg.HostIP,
		"wanIp":         cfg.WanIP,
		"hostname":      cfg.Hostname,
		"clientVersion": clientVersion,
	}
	snapshot.addTo(body)
	data, _ := json.Marshal(body)
	reqCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.API+"/api/device/heartbeat", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("heartbeat status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if result.Code != 0 && result.Code != 200 {
		return fmt.Errorf("heartbeat code %d: %s", result.Code, result.Msg)
	}
	if result.Data.Guid != "" || result.Data.Sncode != "" || result.Data.DeviceType != "" || result.Data.Alias != "" || result.Data.Remark != "" {
		state := clientState{
			Sncode:      firstNonEmpty(result.Data.Sncode, cfg.Sncode),
			DeviceType:  firstNonEmpty(result.Data.DeviceType, cfg.DeviceType),
			DeviceGuid:  firstNonEmpty(result.Data.Guid, cfg.DeviceGuid),
			DeviceToken: cfg.DeviceToken,
			Alias:       firstNonEmpty(result.Data.Alias, cfg.Alias),
			Remark:      firstNonEmpty(result.Data.Remark, cfg.Remark),
			UpdateTime:  time.Now().UnixMilli(),
		}
		if err := saveClientState(cfg.StateFile, state); err != nil {
			log.Printf("save heartbeat state failed path=%s err=%v", cfg.StateFile, err)
		}
	}
	if result.Data.Upgrade != nil {
		startClientUpgrade(cfg, *result.Data.Upgrade)
	}
	if result.Data.VPNRestart != nil {
		signalVPNRestart(*result.Data.VPNRestart)
	}
	return nil
}

func signalVPNRestart(command clientVPNRestartCommand) {
	if command.RequestedAt <= 0 {
		command.RequestedAt = time.Now().UnixMilli()
	}
	vpnRestartState.Lock()
	if command.RequestedAt <= vpnRestartState.lastRequestedAt {
		vpnRestartState.Unlock()
		return
	}
	vpnRestartState.lastRequestedAt = command.RequestedAt
	vpnRestartState.Unlock()
	if command.Message == "" {
		command.Message = "restart vpn tunnel"
	}
	select {
	case vpnRestartState.ch <- command:
		log.Printf("vpn restart requested requestedAt=%d message=%s", command.RequestedAt, command.Message)
	default:
		select {
		case <-vpnRestartState.ch:
		default:
		}
		select {
		case vpnRestartState.ch <- command:
			log.Printf("vpn restart requested requestedAt=%d message=%s", command.RequestedAt, command.Message)
		default:
		}
	}
}

func startClientUpgrade(cfg clientConfig, upgrade clientUpgradeCommand) {
	upgrade.TaskGuid = strings.TrimSpace(upgrade.TaskGuid)
	upgrade.DownloadURL = strings.TrimSpace(upgrade.DownloadURL)
	if upgrade.TaskGuid == "" || upgrade.DownloadURL == "" {
		return
	}
	upgradeState.Lock()
	if upgradeState.taskGuid != "" {
		upgradeState.Unlock()
		return
	}
	upgradeState.taskGuid = upgrade.TaskGuid
	upgradeState.Unlock()
	go func() {
		defer func() {
			upgradeState.Lock()
			upgradeState.taskGuid = ""
			upgradeState.Unlock()
		}()
		reporter := newClientUpgradeReporter(cfg, upgrade)
		if err := performClientUpgrade(cfg, upgrade, reporter); err != nil {
			log.Printf("client upgrade failed task=%s version=%s err=%v", upgrade.TaskGuid, upgrade.Version, err)
			reporter.Failed(err.Error())
		}
	}()
}

func newClientUpgradeReporter(cfg clientConfig, upgrade clientUpgradeCommand) *clientUpgradeReporter {
	return &clientUpgradeReporter{cfg: cfg, upgrade: upgrade}
}

func (r *clientUpgradeReporter) Running(message string, progress int, downloadedSize int64) {
	r.report("running", message, "", progress, downloadedSize)
}

func (r *clientUpgradeReporter) Success(message string) {
	r.report("success", message, "", 100, r.downloadedSize)
}

func (r *clientUpgradeReporter) Failed(detail string) {
	r.report("failed", "客户端升级失败", detail, r.progress, r.downloadedSize)
}

func (r *clientUpgradeReporter) report(status string, message string, detail string, progress int, downloadedSize int64) {
	if r == nil {
		return
	}
	if downloadedSize >= 0 {
		r.downloadedSize = maxInt64(r.downloadedSize, downloadedSize)
	}
	r.progress = clampClientProgress(progress, 0, 100)
	if err := reportClientUpgrade(context.Background(), r.cfg, r.upgrade, status, message, detail, r.progress, r.downloadedSize); err != nil {
		log.Printf("client upgrade report failed task=%s status=%s progress=%d err=%v", r.upgrade.TaskGuid, status, r.progress, err)
	}
}

func (r *upgradeProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && r.onRead != nil {
		r.onRead(int64(n))
	}
	return n, err
}

func performClientUpgrade(cfg clientConfig, upgrade clientUpgradeCommand, reporter *clientUpgradeReporter) error {
	log.Printf("client upgrade started task=%s version=%s url=%s", upgrade.TaskGuid, upgrade.Version, upgrade.DownloadURL)
	reporter.Running("准备升级客户端", 3, 0)
	if err := validateUpgradePlatform(upgrade); err != nil {
		return err
	}
	currentPath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	tmpPath := currentPath + ".new"
	backupPath := currentPath + ".bak"
	reporter.Running("正在下载客户端二进制", 8, 0)
	if err := downloadUpgradeBinary(cfg, upgrade, tmpPath, reporter); err != nil {
		return err
	}
	reporter.Running("正在设置客户端文件权限", 90, reporter.downloadedSize)
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}
	reporter.Running("正在备份当前客户端", 92, reporter.downloadedSize)
	_ = os.Remove(backupPath)
	if err := os.Rename(currentPath, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	reporter.Running("正在替换客户端二进制", 96, reporter.downloadedSize)
	if err := os.Rename(tmpPath, currentPath); err != nil {
		_ = os.Rename(backupPath, currentPath)
		return err
	}
	reporter.Success("客户端二进制已替换，准备重启服务")
	log.Printf("client upgrade installed task=%s current=%s backup=%s", upgrade.TaskGuid, currentPath, backupPath)
	restartClientService(cfg)
	return nil
}

func downloadUpgradeBinary(cfg clientConfig, upgrade clientUpgradeCommand, targetPath string, reporter *clientUpgradeReporter) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	url := upgrade.DownloadURL
	if strings.HasPrefix(url, "/") {
		url = strings.TrimRight(cfg.API, "/") + url
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	totalSize := upgrade.Size
	if totalSize <= 0 && resp.ContentLength > 0 {
		totalSize = resp.ContentLength
	}
	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	hash := sha256.New()
	downloadedSize := int64(0)
	lastProgress := 8
	lastReportAt := time.Now()
	reader := io.Reader(resp.Body)
	if reporter != nil {
		reader = &upgradeProgressReader{
			reader: resp.Body,
			onRead: func(n int64) {
				downloadedSize += n
				progress := downloadProgress(downloadedSize, totalSize)
				now := time.Now()
				if progress >= lastProgress+5 || now.Sub(lastReportAt) >= 2*time.Second {
					reporter.Running("正在下载客户端二进制", progress, downloadedSize)
					lastProgress = progress
					lastReportAt = now
				}
			},
		}
	}
	size, copyErr := io.Copy(io.MultiWriter(out, hash), reader)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(targetPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(targetPath)
		return closeErr
	}
	if reporter != nil {
		reporter.Running("正在校验客户端二进制", 84, size)
	}
	if upgrade.Size > 0 && size != upgrade.Size {
		_ = os.Remove(targetPath)
		return fmt.Errorf("download size mismatch want=%d got=%d", upgrade.Size, size)
	}
	if upgrade.Sha256 != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if !strings.EqualFold(got, upgrade.Sha256) {
			_ = os.Remove(targetPath)
			return fmt.Errorf("sha256 mismatch want=%s got=%s", upgrade.Sha256, got)
		}
	}
	if reporter != nil {
		reporter.Running("客户端二进制校验完成", 88, size)
	}
	return nil
}

func downloadProgress(downloadedSize int64, totalSize int64) int {
	if totalSize <= 0 {
		return 10
	}
	progress := 10 + int(downloadedSize*70/totalSize)
	return clampClientProgress(progress, 10, 80)
}

func clampClientProgress(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func reportClientUpgrade(ctx context.Context, cfg clientConfig, upgrade clientUpgradeCommand, status, message, detail string, progress int, downloadedSize int64) error {
	body := map[string]any{
		"token":          cfg.authToken(),
		"taskGuid":       upgrade.TaskGuid,
		"deviceGuid":     cfg.DeviceGuid,
		"sncode":         cfg.Sncode,
		"status":         status,
		"progress":       progress,
		"downloadedSize": downloadedSize,
		"message":        message,
		"errorMessage":   detail,
		"clientVersion":  clientVersion,
	}
	if status == "success" {
		body["clientVersion"] = firstNonEmpty(upgrade.Version, clientVersion)
	}
	data, _ := json.Marshal(body)
	reqCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.API+"/api/device/upgrade/report", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upgrade report status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if result.Code != 0 && result.Code != 200 {
		return fmt.Errorf("upgrade report code %d: %s", result.Code, result.Msg)
	}
	return nil
}

func validateUpgradePlatform(upgrade clientUpgradeCommand) error {
	if upgrade.OS = strings.ToLower(strings.TrimSpace(upgrade.OS)); upgrade.OS != "" && upgrade.OS != runtime.GOOS {
		return fmt.Errorf("upgrade os mismatch want=%s current=%s", upgrade.OS, runtime.GOOS)
	}
	if upgrade.Arch = strings.ToLower(strings.TrimSpace(upgrade.Arch)); upgrade.Arch != "" && upgrade.Arch != runtime.GOARCH {
		return fmt.Errorf("upgrade arch mismatch want=%s current=%s", upgrade.Arch, runtime.GOARCH)
	}
	return nil
}

func restartClientService(cfg clientConfig) {
	serviceName := strings.TrimSpace(cfg.ServiceName)
	if serviceName != "" && runtime.GOOS == "linux" {
		if _, err := exec.LookPath("systemctl"); err == nil {
			cmd := exec.Command("systemctl", "restart", serviceName)
			if err := cmd.Start(); err == nil {
				return
			}
		}
	}
	log.Printf("client upgrade completed, exiting for supervisor restart")
	os.Exit(0)
}

func currentExecutablePath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolvedPath, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolvedPath
	}
	return execPath, nil
}

func collectSystemSnapshot() systemSnapshot {
	snapshot := systemSnapshot{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
	if info, err := host.Info(); err == nil && info != nil {
		snapshot.OS = firstNonEmpty(info.Platform, info.OS, snapshot.OS)
		snapshot.OSVersion = firstNonEmpty(info.PlatformVersion, info.PlatformFamily)
		snapshot.Kernel = strings.TrimSpace(info.KernelVersion)
	}
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		snapshot.MemoryTotal = int64(vm.Total)
		snapshot.MemoryUsed = int64(vm.Used)
		snapshot.MemoryFree = int64(vm.Available)
	}
	if usage, err := disk.Usage(rootDiskPath()); err == nil && usage != nil {
		snapshot.DiskTotal = int64(usage.Total)
		snapshot.DiskUsed = int64(usage.Used)
		snapshot.DiskFree = int64(usage.Free)
		snapshot.DiskUsedPct = usage.UsedPercent
	}
	return snapshot
}

func (snapshot systemSnapshot) addTo(body map[string]any) {
	body["os"] = snapshot.OS
	body["osVersion"] = snapshot.OSVersion
	body["kernel"] = snapshot.Kernel
	body["arch"] = snapshot.Arch
	body["memoryTotal"] = snapshot.MemoryTotal
	body["memoryUsed"] = snapshot.MemoryUsed
	body["memoryFree"] = snapshot.MemoryFree
	body["diskTotal"] = snapshot.DiskTotal
	body["diskUsed"] = snapshot.DiskUsed
	body["diskFree"] = snapshot.DiskFree
	body["diskUsedPct"] = snapshot.DiskUsedPct
}

func (snapshot systemSnapshot) logFields() string {
	return fmt.Sprintf(
		"os=%s osVersion=%s kernel=%s arch=%s memory=%s/%s freeMemory=%s disk=%s/%s freeDisk=%s diskUsedPct=%.1f%%",
		valueOrDash(snapshot.OS),
		valueOrDash(snapshot.OSVersion),
		valueOrDash(snapshot.Kernel),
		valueOrDash(snapshot.Arch),
		formatBytes(snapshot.MemoryUsed),
		formatBytes(snapshot.MemoryTotal),
		formatBytes(snapshot.MemoryFree),
		formatBytes(snapshot.DiskUsed),
		formatBytes(snapshot.DiskTotal),
		formatBytes(snapshot.DiskFree),
		snapshot.DiskUsedPct,
	)
}

func formatBytes(value int64) string {
	if value <= 0 {
		return "0B"
	}
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%dB", value)
	}
	if value < unit*unit {
		return fmt.Sprintf("%.1fKB", float64(value)/unit)
	}
	if value < unit*unit*unit {
		return fmt.Sprintf("%.1fMB", float64(value)/(unit*unit))
	}
	return fmt.Sprintf("%.1fGB", float64(value)/(unit*unit*unit))
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return strings.TrimSpace(value)
}

func rootDiskPath() string {
	if runtime.GOOS == "windows" {
		if volume := filepath.VolumeName(os.TempDir()); volume != "" {
			return volume + `\`
		}
		if drive := strings.TrimSpace(os.Getenv("SystemDrive")); drive != "" {
			return drive + `\`
		}
		return `C:\`
	}
	return "/"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (cfg clientConfig) authToken() string {
	if strings.TrimSpace(cfg.DeviceToken) != "" {
		return strings.TrimSpace(cfg.DeviceToken)
	}
	return strings.TrimSpace(cfg.Token)
}

func (cfg clientConfig) authMode() string {
	if strings.TrimSpace(cfg.DeviceToken) != "" {
		return "device-token"
	}
	return "register-token"
}

func handleStream(ctx context.Context, cfg clientConfig, stream *quic.Stream) {
	defer stream.Close()
	frame, err := readFrame(stream)
	if err != nil {
		log.Printf("read stream frame failed: %v", err)
		return
	}
	switch frame.Type {
	case tunnel.FrameTypeOpenTCP:
		if err := handleOpenTCPFrame(ctx, cfg, stream, frame); err != nil {
			log.Printf("handle open tcp failed requestId=%s err=%v", frame.RequestID, err)
		}
	case tunnel.FrameTypeOpenLog:
		if err := handleOpenServiceLogFrame(ctx, stream, frame); err != nil {
			log.Printf("handle service log failed requestId=%s err=%v", frame.RequestID, err)
		}
	default:
		_ = writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, Message: "unsupported frame type"})
		return
	}
}

func normalizeServiceLogRequest(serviceName string, tail int) (string, int, error) {
	serviceName = strings.TrimSpace(serviceName)
	if !serviceLogNamePattern.MatchString(serviceName) {
		return "", 0, errors.New("invalid service name")
	}
	if tail <= 0 {
		tail = 200
	}
	if tail > 2000 {
		tail = 2000
	}
	return serviceName, tail, nil
}

func normalizeTargetHost(cfg clientConfig, targetHost string) string {
	targetHost = strings.TrimSpace(targetHost)
	if targetHost == "" || targetHost == "localhost" || targetHost == "127.0.0.1" || targetHost == "::1" {
		return cfg.LocalHost
	}
	return targetHost
}

func readFrame(r io.Reader) (tunnel.Frame, error) {
	line, err := readFrameLine(r)
	if err != nil {
		return tunnel.Frame{}, err
	}
	var frame tunnel.Frame
	if err := json.Unmarshal(line, &frame); err != nil {
		return tunnel.Frame{}, err
	}
	return frame, nil
}

func readFrameLine(r io.Reader) ([]byte, error) {
	line := make([]byte, 0, 256)
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line = append(line, buf[0])
			if buf[0] == '\n' {
				return line, nil
			}
			if len(line) > 64*1024 {
				return nil, errors.New("frame too large")
			}
		}
		if err != nil {
			return nil, err
		}
	}
}

func writeFrame(w io.Writer, frame tunnel.Frame) error {
	data, err := tunnel.EncodeFrame(frame)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func bridge(a io.ReadWriteCloser, b io.ReadWriteCloser) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(a, b)
		closeWrite(a)
		closeRead(b)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(b, a)
		closeWrite(b)
		closeRead(a)
		done <- struct{}{}
	}()
	<-done
	<-done
	_ = a.Close()
	_ = b.Close()
}

func closeWrite(conn io.Closer) {
	if conn == nil {
		return
	}
	if closer, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite()
		return
	}
	_ = conn.Close()
}

func closeRead(conn io.Closer) {
	if conn == nil {
		return
	}
	if closer, ok := conn.(interface{ CloseRead() error }); ok {
		_ = closer.CloseRead()
		return
	}
	if closer, ok := conn.(interface{ CancelRead(quic.StreamErrorCode) }); ok {
		closer.CancelRead(0)
	}
}

func detectOutboundIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

func detectPublicIP(timeout time.Duration) string {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := http.Client{Timeout: timeout}
	for _, endpoint := range []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode >= 300 {
			continue
		}
		if ip := normalizeIP(string(data)); isPublicIP(ip) {
			return ip
		}
	}
	return ""
}

func normalizeIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func isPublicIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	return ip.IsGlobalUnicast() &&
		!ip.IsPrivate() &&
		!ip.IsLoopback() &&
		!ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast()
}

func defaultStateFile() string {
	dir, err := executableDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "navmesh-client.json")
}

func executableDir() (string, error) {
	execPath, err := os.Executable()
	if err == nil {
		if resolvedPath, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
			execPath = resolvedPath
		}
		return filepath.Dir(execPath), nil
	}
	return os.Getwd()
}

func generateSncode(hostname string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "nmc-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	prefix := strings.ToLower(strings.TrimSpace(hostname))
	prefix = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, prefix)
	prefix = strings.Trim(prefix, "-")
	if prefix == "" {
		prefix = "device"
	}
	if len(prefix) > 24 {
		prefix = prefix[:24]
	}
	return "nmc-" + prefix + "-" + hex.EncodeToString(buf)
}

func loadClientState(path string) (clientState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return clientState{}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return clientState{}, nil
	}
	if err != nil {
		return clientState{}, err
	}
	var state clientState
	if err := json.Unmarshal(data, &state); err != nil {
		return clientState{}, err
	}
	return state, nil
}

func saveClientState(path string, state clientState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
