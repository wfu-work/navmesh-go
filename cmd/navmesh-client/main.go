package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

const clientVersion = "v0.0.1"

type clientConfig struct {
	Server         string
	Port           int
	API            string
	Token          string
	DeviceToken    string
	StateFile      string
	Sncode         string
	DeviceType     string
	Alias          string
	Remark         string
	SSHPort        int
	WebPort        int
	WebDomain      string
	Hostname       string
	HostIP         string
	LocalHost      string
	SkipRegister   bool
	InsecureQUIC   bool
	ReconnectWait  time.Duration
	ReconnectMax   time.Duration
	Heartbeat      time.Duration
	HeartbeatFail  int
	RequestTimeout time.Duration
}

type registerResponse struct {
	Code int `json:"code"`
	Data struct {
		Device struct {
			Guid   string `json:"guid"`
			Alias  string `json:"alias"`
			Remark string `json:"remark"`
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
}

type clientState struct {
	Sncode      string `json:"sncode"`
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
		_, _ = fmt.Fprintln(out, "  navmesh-client -server tunnel.navfirst.com -port 3008 -token navfirst@2020 -sncode test01 -type rain -sshPort 22 -webPort 7090 -remark 深圳工厂1号测试网关")
	}
	flag.StringVar(&cfg.Server, "server", "127.0.0.1", "NavMesh 隧道服务器地址")
	flag.IntVar(&cfg.Port, "port", 3008, "NavMesh 隧道服务器 UDP 端口")
	flag.StringVar(&cfg.API, "api", "", "NavMesh 管理 API 基础地址，默认 http://<server>:3007")
	flag.StringVar(&cfg.Token, "token", "navfirst@2020", "首次注册令牌；注册成功后客户端会保存并改用设备独立 Token")
	flag.StringVar(&cfg.DeviceToken, "deviceToken", "", "设备独立 Token；状态文件丢失时可由管理端重新生成后填入")
	flag.StringVar(&cfg.StateFile, "stateFile", "", "客户端状态文件路径，默认 /var/lib/navmesh-client/<sncode>.json，无法写入时回退当前目录")
	flag.StringVar(&cfg.Sncode, "sncode", "MAC001", "设备 SN 编码，全局唯一")
	flag.StringVar(&cfg.DeviceType, "type", "rain", "设备类型")
	flag.StringVar(&cfg.Alias, "alias", "MacAir笔记本电脑", "设备别名，默认使用 sncode")
	flag.StringVar(&cfg.Remark, "remark", "Mac Wfu测试设备", "设备备注")
	flag.IntVar(&cfg.SSHPort, "sshPort", 22, "本机 SSH 服务端口")
	flag.IntVar(&cfg.WebPort, "webPort", 0, "本机 Web 服务端口，0 表示不启用")
	flag.StringVar(&cfg.WebDomain, "webDomain", "", "外部 Web 映射域名")
	flag.StringVar(&cfg.Hostname, "hostname", hostname, "上报的主机名")
	flag.StringVar(&cfg.HostIP, "hostIp", "", "上报的主机IP，默认自动探测")
	flag.StringVar(&cfg.LocalHost, "localHost", "127.0.0.1", "服务端请求回环目标时使用的本机地址")
	flag.BoolVar(&cfg.SkipRegister, "skipRegister", false, "跳过HTTP设备注册，直接建立隧道")
	flag.BoolVar(&cfg.InsecureQUIC, "insecure", true, "跳过QUIC服务器证书校验")
	flag.BoolVar(&showVersion, "v", false, "查看当前客户端版本")
	flag.DurationVar(&cfg.ReconnectWait, "reconnectWait", 5*time.Second, "首次重连等待时间")
	flag.DurationVar(&cfg.ReconnectMax, "reconnectMax", 60*time.Second, "指数退避最大重连等待时间")
	flag.DurationVar(&cfg.Heartbeat, "heartbeat", 30*time.Second, "QUIC 和 HTTP 心跳间隔")
	flag.IntVar(&cfg.HeartbeatFail, "heartbeatFail", 3, "连续心跳失败多少次后主动断开并重连")
	flag.DurationVar(&cfg.RequestTimeout, "requestTimeout", 10*time.Second, "HTTP 请求和本地端口连接超时时间")
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
	if cfg.Alias == "" {
		cfg.Alias = cfg.Sncode
	}
	if cfg.StateFile == "" {
		cfg.StateFile = defaultStateFile(cfg.Sncode)
	}
	if cfg.HostIP == "" {
		cfg.HostIP = detectOutboundIP()
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
	return cfg
}

func run(ctx context.Context, cfg clientConfig) error {
	if cfg.Sncode == "" {
		return fmt.Errorf("sncode required")
	}
	if cfg.Token == "" {
		return fmt.Errorf("token required")
	}
	snapshot := collectSystemSnapshot()
	log.Printf(
		"navmesh-client starting version=%s sncode=%s type=%s server=%s port=%d api=%s hostname=%s hostIp=%s os=%s osVersion=%s kernel=%s arch=%s memoryTotal=%d diskTotal=%d sshPort=%d webPort=%d heartbeat=%s stateFile=%s skipRegister=%t",
		clientVersion,
		cfg.Sncode,
		cfg.DeviceType,
		cfg.Server,
		cfg.Port,
		cfg.API,
		cfg.Hostname,
		cfg.HostIP,
		snapshot.OS,
		snapshot.OSVersion,
		snapshot.Kernel,
		snapshot.Arch,
		snapshot.MemoryTotal,
		snapshot.DiskTotal,
		cfg.SSHPort,
		cfg.WebPort,
		cfg.Heartbeat,
		cfg.StateFile,
		cfg.SkipRegister,
	)
	state, err := loadClientState(cfg.StateFile)
	if err != nil {
		log.Printf("load client state failed path=%s err=%v", cfg.StateFile, err)
	}
	if cfg.DeviceToken == "" && state.DeviceToken != "" && (state.Sncode == "" || state.Sncode == cfg.Sncode) {
		cfg.DeviceToken = state.DeviceToken
		log.Printf("loaded device token state path=%s sncode=%s", cfg.StateFile, cfg.Sncode)
	} else if cfg.DeviceToken != "" {
		log.Printf("using device token from command line sncode=%s stateFile=%s", cfg.Sncode, cfg.StateFile)
	}
	if state.Sncode == "" || state.Sncode == cfg.Sncode {
		if strings.TrimSpace(state.Alias) != "" {
			cfg.Alias = strings.TrimSpace(state.Alias)
		}
		if strings.TrimSpace(state.Remark) != "" {
			cfg.Remark = strings.TrimSpace(state.Remark)
		}
	}
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
		"registering device api=%s sncode=%s type=%s alias=%s hostname=%s hostIp=%s os=%s kernel=%s memoryUsed=%d diskUsed=%d sshPort=%d webPort=%d auth=%s",
		cfg.API,
		cfg.Sncode,
		cfg.DeviceType,
		cfg.Alias,
		cfg.Hostname,
		cfg.HostIP,
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
		"sncode":        cfg.Sncode,
		"type":          cfg.DeviceType,
		"alias":         cfg.Alias,
		"remark":        cfg.Remark,
		"hostname":      cfg.Hostname,
		"hostIp":        cfg.HostIP,
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
	if result.Data.Device.Alias != "" {
		cfg.Alias = result.Data.Device.Alias
	}
	cfg.Remark = result.Data.Device.Remark
	state := clientState{
		Sncode:      cfg.Sncode,
		DeviceGuid:  result.Data.Device.Guid,
		DeviceToken: cfg.DeviceToken,
		TokenGuid:   result.Data.DeviceToken.Item.Guid,
		Alias:       cfg.Alias,
		Remark:      cfg.Remark,
		UpdateTime:  time.Now().UnixMilli(),
	}
	if state.DeviceToken != "" || state.Alias != "" || state.Remark != "" {
		if err := saveClientState(cfg.StateFile, state); err != nil {
			log.Printf("save client state failed path=%s err=%v", cfg.StateFile, err)
		} else {
			log.Printf("saved device state path=%s sncode=%s alias=%s", cfg.StateFile, cfg.Sncode, cfg.Alias)
		}
	}
	if result.Data.SSH.Alias.Domain != "" {
		log.Printf("registered device sncode=%s guid=%s ssh=%s ready=%t entrypoint=%s", cfg.Sncode, result.Data.Device.Guid, result.Data.SSH.Alias.Domain, result.Data.SSH.Ready, result.Data.SSH.EntrypointIP)
	} else {
		log.Printf("registered device sncode=%s guid=%s", cfg.Sncode, result.Data.Device.Guid)
	}
	return cfg, nil
}

func connectAndServe(ctx context.Context, cfg clientConfig) error {
	addr := net.JoinHostPort(cfg.Server, strconv.Itoa(cfg.Port))
	log.Printf("connecting tunnel server=%s sncode=%s insecure=%t heartbeat=%s", addr, cfg.Sncode, cfg.InsecureQUIC, cfg.Heartbeat)
	conn, err := quic.DialAddr(ctx, addr, &tls.Config{
		InsecureSkipVerify: cfg.InsecureQUIC,
		NextProtos:         []string{"navmesh-quic"},
		MinVersion:         tls.VersionTLS13,
	}, &quic.Config{
		KeepAlivePeriod: cfg.Heartbeat,
		MaxIdleTimeout:  cfg.Heartbeat * 4,
	})
	if err != nil {
		return err
	}
	defer conn.CloseWithError(0, "client reconnecting")

	if err := sendHello(ctx, conn, cfg); err != nil {
		return err
	}
	snapshot := collectSystemSnapshot()
	log.Printf("tunnel connected server=%s sncode=%s hostname=%s hostIp=%s %s", addr, cfg.Sncode, cfg.Hostname, cfg.HostIP, snapshot.logFields())

	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	heartbeatErr := make(chan error, 1)
	go heartbeatLoop(heartbeatCtx, conn, cfg, heartbeatErr)

	var wg sync.WaitGroup
	defer wg.Wait()
	acceptErr := make(chan error, 1)
	go func() {
		for {
			stream, err := conn.AcceptStream(ctx)
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
			_ = conn.CloseWithError(2, "heartbeat failed")
			return err
		case <-ctx.Done():
			_ = conn.CloseWithError(0, "client stopping")
			return ctx.Err()
		}
	}
}

func sendHello(ctx context.Context, conn *quic.Conn, cfg clientConfig) error {
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	frame := tunnel.Frame{
		Type:          tunnel.FrameTypeHello,
		Token:         cfg.authToken(),
		SnCode:        cfg.Sncode,
		HostIP:        cfg.HostIP,
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

func heartbeatLoop(ctx context.Context, conn *quic.Conn, cfg clientConfig, errCh chan<- error) {
	ticker := time.NewTicker(cfg.Heartbeat)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := sendHeartbeat(ctx, conn, cfg); err != nil {
				failures++
				log.Printf("quic heartbeat failed failures=%d err=%v", failures, err)
			} else if err := postHeartbeat(ctx, cfg); err != nil {
				failures++
				log.Printf("http heartbeat failed failures=%d err=%v", failures, err)
			} else {
				failures = 0
				snapshot := collectSystemSnapshot()
				log.Printf("heartbeat ok sncode=%s hostname=%s hostIp=%s interval=%s %s", cfg.Sncode, cfg.Hostname, cfg.HostIP, cfg.Heartbeat, snapshot.logFields())
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

func sendHeartbeat(ctx context.Context, conn *quic.Conn, cfg clientConfig) error {
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	return writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeHeartbeat, SnCode: cfg.Sncode})
}

func postHeartbeat(ctx context.Context, cfg clientConfig) error {
	snapshot := collectSystemSnapshot()
	body := map[string]any{
		"token":         cfg.authToken(),
		"sncode":        cfg.Sncode,
		"hostIp":        cfg.HostIP,
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
	return nil
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
	if frame.Type != tunnel.FrameTypeOpenTCP {
		_ = writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, Message: "unsupported frame type"})
		return
	}
	targetHost := normalizeTargetHost(cfg, frame.TargetHost)
	targetPort := frame.TargetPort
	log.Printf("open tcp request requestId=%s target=%s:%d sncode=%s", frame.RequestID, targetHost, targetPort, cfg.Sncode)
	if targetPort <= 0 {
		_ = writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, Message: "invalid target port"})
		return
	}
	dialCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	local, err := (&net.Dialer{Timeout: cfg.RequestTimeout}).DialContext(dialCtx, "tcp", net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
	if err != nil {
		log.Printf("dial local target failed target=%s:%d err=%v", targetHost, targetPort, err)
		_ = writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, OK: false, Message: err.Error()})
		return
	}
	defer local.Close()
	if err := writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeOpenTCPAck, RequestID: frame.RequestID, OK: true}); err != nil {
		log.Printf("write open_tcp ack failed: %v", err)
		return
	}
	log.Printf("open tcp connected requestId=%s target=%s:%d", frame.RequestID, targetHost, targetPort)
	bridge(local, stream)
	log.Printf("open tcp closed requestId=%s target=%s:%d", frame.RequestID, targetHost, targetPort)
}

func normalizeTargetHost(cfg clientConfig, targetHost string) string {
	targetHost = strings.TrimSpace(targetHost)
	if targetHost == "" || targetHost == "localhost" || targetHost == "127.0.0.1" || targetHost == "::1" {
		return cfg.LocalHost
	}
	return targetHost
}

func readFrame(r io.Reader) (tunnel.Frame, error) {
	line, err := bufio.NewReader(r).ReadBytes('\n')
	if err != nil {
		return tunnel.Frame{}, err
	}
	var frame tunnel.Frame
	if err := json.Unmarshal(line, &frame); err != nil {
		return tunnel.Frame{}, err
	}
	return frame, nil
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
		_ = a.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(b, a)
		_ = b.Close()
		done <- struct{}{}
	}()
	<-done
	<-done
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

func defaultStateFile(sncode string) string {
	name := strings.TrimSpace(sncode)
	if name == "" {
		name = "device"
	}
	name = strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(name)
	if canUseStateDir("/var/lib/navmesh-client") {
		return filepath.Join("/var/lib/navmesh-client", name+".json")
	}
	return filepath.Join(".", ".navmesh-client-"+name+".json")
}

func canUseStateDir(dir string) bool {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	test, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	path := test.Name()
	_ = test.Close()
	_ = os.Remove(path)
	return true
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
