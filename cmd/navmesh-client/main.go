package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"navmesh-go/tunnel"

	"github.com/quic-go/quic-go"
)

const clientVersion = "v0.1.0"

type clientConfig struct {
	Server         string
	Port           int
	API            string
	Token          string
	SnCode         string
	DeviceID       string
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
			Guid string `json:"guid"`
		} `json:"device"`
	} `json:"data"`
	Msg string `json:"msg"`
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
	flag.StringVar(&cfg.Server, "server", "127.0.0.1", "NavMesh tunnel server host")
	flag.IntVar(&cfg.Port, "port", 3008, "NavMesh tunnel server UDP port")
	flag.StringVar(&cfg.API, "api", "", "NavMesh management API base URL, default http://<server>:3007")
	flag.StringVar(&cfg.Token, "token", "navfirst@2020", "device register/access token")
	flag.StringVar(&cfg.SnCode, "sncode", "", "device sncode, globally unique")
	flag.StringVar(&cfg.DeviceID, "deviceId", "", "business device id")
	flag.StringVar(&cfg.DeviceType, "type", "ssh", "device type")
	flag.StringVar(&cfg.Alias, "alias", "", "device alias, default sncode")
	flag.StringVar(&cfg.Remark, "remark", "", "device remark")
	flag.IntVar(&cfg.SSHPort, "sshPort", 22, "local SSHD port")
	flag.IntVar(&cfg.WebPort, "webPort", 0, "local web service port")
	flag.StringVar(&cfg.WebDomain, "webDomain", "", "external web mapping domain")
	flag.StringVar(&cfg.Hostname, "hostname", hostname, "reported hostname")
	flag.StringVar(&cfg.HostIP, "hostIp", "", "reported host ip")
	flag.StringVar(&cfg.LocalHost, "localHost", "127.0.0.1", "local host used when gateway requests loopback targets")
	flag.BoolVar(&cfg.SkipRegister, "skipRegister", false, "skip HTTP device registration before opening tunnel")
	flag.BoolVar(&cfg.InsecureQUIC, "insecure", true, "skip QUIC server certificate verification")
	flag.DurationVar(&cfg.ReconnectWait, "reconnectWait", 5*time.Second, "initial reconnect wait duration")
	flag.DurationVar(&cfg.ReconnectMax, "reconnectMax", 60*time.Second, "maximum reconnect wait duration")
	flag.DurationVar(&cfg.Heartbeat, "heartbeat", 30*time.Second, "heartbeat interval")
	flag.IntVar(&cfg.HeartbeatFail, "heartbeatFail", 3, "consecutive heartbeat failures before reconnect")
	flag.DurationVar(&cfg.RequestTimeout, "requestTimeout", 10*time.Second, "HTTP request and local dial timeout")
	flag.Parse()

	cfg.Server = strings.TrimSpace(cfg.Server)
	cfg.API = strings.TrimRight(strings.TrimSpace(cfg.API), "/")
	if cfg.API == "" {
		cfg.API = "http://" + net.JoinHostPort(cfg.Server, "3007")
	}
	if cfg.Alias == "" {
		cfg.Alias = cfg.SnCode
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
	if cfg.SnCode == "" {
		return fmt.Errorf("sncode required")
	}
	if cfg.Token == "" {
		return fmt.Errorf("token required")
	}
	failures := 0
	for {
		if !cfg.SkipRegister {
			if err := registerDevice(ctx, cfg); err != nil {
				log.Printf("register device failed: %v", err)
				failures++
				if err := waitReconnect(ctx, cfg, failures); err != nil {
					return err
				}
				continue
			}
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

func registerDevice(ctx context.Context, cfg clientConfig) error {
	body := map[string]any{
		"token":         cfg.Token,
		"sncode":        cfg.SnCode,
		"deviceId":      cfg.DeviceID,
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
	data, _ := json.Marshal(body)
	reqCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.API+"/api/device/register", bytes.NewReader(data))
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
		return fmt.Errorf("register status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var result registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Code != 200 {
		return fmt.Errorf("%s", result.Msg)
	}
	log.Printf("registered device sncode=%s guid=%s", cfg.SnCode, result.Data.Device.Guid)
	return nil
}

func connectAndServe(ctx context.Context, cfg clientConfig) error {
	addr := net.JoinHostPort(cfg.Server, strconv.Itoa(cfg.Port))
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
	log.Printf("tunnel connected server=%s sncode=%s", addr, cfg.SnCode)

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
		Token:         cfg.Token,
		SnCode:        cfg.SnCode,
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
	return writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeHeartbeat, SnCode: cfg.SnCode})
}

func postHeartbeat(ctx context.Context, cfg clientConfig) error {
	body := map[string]any{
		"token":         cfg.Token,
		"sncode":        cfg.SnCode,
		"deviceId":      cfg.DeviceID,
		"hostIp":        cfg.HostIP,
		"hostname":      cfg.Hostname,
		"clientVersion": clientVersion,
	}
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
	return nil
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
	if targetPort <= 0 {
		_ = writeFrame(stream, tunnel.Frame{Type: tunnel.FrameTypeError, RequestID: frame.RequestID, Message: "invalid target port"})
		return
	}
	dialCtx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
	defer cancel()
	local, err := (&net.Dialer{Timeout: cfg.RequestTimeout}).DialContext(dialCtx, "tcp", net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
	if err != nil {
		log.Printf("dial local target failed target=%s:%d err=%v", targetHost, targetPort, err)
		return
	}
	defer local.Close()
	bridge(local, stream)
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
