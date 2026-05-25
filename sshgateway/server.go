package sshgateway

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"navmesh-go/domains"
	"navmesh-go/services"
	"navmesh-go/tunnel"

	"github.com/google/uuid"
	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Server struct {
	addr     string
	listener net.Listener
	manager  *tunnel.Manager
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewServer(addr string, manager *tunnel.Manager) *Server {
	if strings.TrimSpace(addr) == "" {
		addr = ":22"
	}
	if manager == nil {
		manager = tunnel.DefaultManager
	}
	return &Server{addr: addr, manager: manager}
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.listener = listener
	s.wg.Add(1)
	go s.acceptLoop(ctx)
	global.NAV_LOG.Info("navmesh ssh gateway started", zap.String("addr", s.addr))
	return nil
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			global.NAV_LOG.Warn("accept ssh gateway connection failed", zap.Error(err))
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
}

func (s *Server) handleConn(ctx context.Context, client net.Conn) {
	defer client.Close()
	localIP := localAddrIP(client.LocalAddr())
	sourceIP := remoteAddrIP(client.RemoteAddr())
	route, err := findRouteByEntrypoint(localIP)
	if err != nil {
		global.NAV_LOG.Warn("ssh route not found", zap.String("entrypointIp", localIP), zap.String("sourceIp", sourceIP), zap.Error(err))
		return
	}
	if !services.ServiceGroupApp.AccessPolicyService.IsAllowed(route.Device.Guid, "", "ssh") {
		global.NAV_LOG.Warn("ssh access denied by policy", zap.String("deviceGuid", route.Device.Guid), zap.String("sourceIp", sourceIP))
		return
	}
	permit, err := services.DefaultRuntimePolicy.Acquire(route.Device.Guid, sourceIP)
	if err != nil {
		global.NAV_LOG.Warn("ssh session rejected by runtime policy", zap.String("deviceGuid", route.Device.Guid), zap.String("sourceIp", sourceIP), zap.Error(err))
		services.ServiceGroupApp.EventService.Record(services.EventInput{
			DeviceGuid: route.Device.Guid,
			EventType:  "session_rejected",
			Level:      "warn",
			Title:      "ssh session rejected",
			Message:    err.Error(),
		})
		return
	}
	defer services.DefaultRuntimePolicy.Release(permit)
	targetPort := route.Device.SSHPort
	if targetPort <= 0 {
		targetPort = 22
	}
	start := domains.NowMilli()
	session := domains.TunnelSession{
		Guid:        uuid.NewString(),
		DeviceGuid:  route.Device.Guid,
		SessionType: "ssh",
		SourceIP:    sourceIP,
		TargetHost:  "127.0.0.1",
		TargetPort:  targetPort,
		PublicHost:  route.Alias.Domain,
		Status:      int(domains.StatusEnabled),
		StartTime:   start,
		CreateTime:  start,
		UpdateTime:  start,
	}
	_ = global.NAV_DB.Create(&session).Error

	streamCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	upstream, err := s.manager.OpenTCPStream(streamCtx, route.Device.Guid, "127.0.0.1", targetPort)
	if err != nil {
		global.NAV_LOG.Warn("open ssh tunnel stream failed", zap.String("deviceGuid", route.Device.Guid), zap.Error(err))
		services.ServiceGroupApp.EventService.Record(services.EventInput{
			DeviceGuid: route.Device.Guid,
			EventType:  "open_tcp_failed",
			Level:      "error",
			Title:      "open ssh target failed",
			Message:    err.Error(),
		})
		markSessionClosed(session.Guid, 0, 0, "open_tunnel_failed: "+err.Error())
		return
	}
	defer upstream.Close()
	services.DefaultSessionRegistry.RegisterSession(session.Guid, client, upstream)
	defer services.DefaultSessionRegistry.UnregisterSession(session.Guid)

	bytesIn, bytesOut := bridge(client, upstream, services.DefaultRuntimePolicy.IdleTimeout())
	reason := "closed"
	if isForceClosed(session.Guid) {
		reason = "closed_by_admin"
	}
	markSessionClosed(session.Guid, bytesIn, bytesOut, reason)
}

type sshRoute struct {
	Alias  domains.SSHAlias
	Device domains.Device
}

func findRouteByEntrypoint(entrypointIP string) (*sshRoute, error) {
	if entrypointIP == "" {
		return nil, errors.New("empty entrypoint ip")
	}
	var alias domains.SSHAlias
	err := global.NAV_DB.Where("entrypoint_ip = ? AND status = ?", entrypointIP, int(domains.StatusEnabled)).First(&alias).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var entry domains.SSHEntrypoint
		if eerr := global.NAV_DB.Where("ip = ? AND status = ?", entrypointIP, int(domains.StatusEnabled)).First(&entry).Error; eerr != nil {
			return nil, err
		}
		err = global.NAV_DB.Where("device_guid = ? AND status = ?", entry.DeviceGuid, int(domains.StatusEnabled)).First(&alias).Error
	}
	if err != nil {
		return nil, err
	}
	var device domains.Device
	if err := global.NAV_DB.Where("guid = ? AND status != ?", alias.DeviceGuid, domains.DeviceStatusDisabled).First(&device).Error; err != nil {
		return nil, err
	}
	return &sshRoute{Alias: alias, Device: device}, nil
}

func localAddrIP(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return normalizeIP(host)
}

func remoteAddrIP(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return normalizeIP(host)
}

func normalizeIP(host string) string {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return strings.Trim(host, "[]")
	}
	return ip.String()
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func bridge(a io.ReadWriteCloser, b io.ReadWriteCloser, idleTimeout time.Duration) (int64, int64) {
	aCounter := &countingWriter{w: a}
	bCounter := &countingWriter{w: b}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = copyWithIdleDeadline(bCounter, a, idleTimeout)
		_ = b.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = copyWithIdleDeadline(aCounter, b, idleTimeout)
		_ = a.Close()
		done <- struct{}{}
	}()
	<-done
	<-done
	return bCounter.n, aCounter.n
}

func copyWithIdleDeadline(dst io.Writer, src io.Reader, idleTimeout time.Duration) (int64, error) {
	if idleTimeout <= 0 {
		return io.Copy(dst, src)
	}
	buf := make([]byte, 32*1024)
	var written int64
	for {
		setReadDeadline(src, time.Now().Add(idleTimeout))
		nr, er := src.Read(buf)
		if nr > 0 {
			setWriteDeadline(dst, time.Now().Add(idleTimeout))
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			return written, er
		}
	}
}

func setReadDeadline(value any, deadline time.Time) {
	if conn, ok := value.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = conn.SetReadDeadline(deadline)
	}
}

func setWriteDeadline(value any, deadline time.Time) {
	if conn, ok := value.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = conn.SetWriteDeadline(deadline)
	}
}

func markSessionClosed(guid string, bytesIn, bytesOut int64, reason string) {
	now := domains.NowMilli()
	updates := map[string]any{
		"status":            int(domains.StatusDisabled),
		"bytes_in":          bytesIn,
		"bytes_out":         bytesOut,
		"end_time":          now,
		"update_time":       now,
		"disconnect_reason": strings.TrimSpace(reason),
	}
	_ = global.NAV_DB.Model(&domains.TunnelSession{}).Where("guid = ?", guid).Updates(updates).Error
}

func isForceClosed(guid string) bool {
	var session domains.TunnelSession
	if err := global.NAV_DB.Select("force_closed").Where("guid = ?", guid).First(&session).Error; err != nil {
		return false
	}
	return session.ForceClosed
}

func ListenHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return host
}

func ListenPort(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(port)
	return n
}
