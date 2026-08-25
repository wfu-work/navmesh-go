package tcpgateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"navmesh-go/domains"
	"navmesh-go/internal/streambridge"
	"navmesh-go/services"
	"navmesh-go/tunnel"

	"github.com/google/uuid"
	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
)

type Server struct {
	manager   *tunnel.Manager
	mu        sync.Mutex
	listeners map[int]*portListener
	stopped   bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

type portListener struct {
	port     int
	listener net.Listener
	mu       sync.RWMutex
	mapping  domains.TCPMapping
}

const tcpOpenFailedEventSuppressionWindow = 2 * time.Hour

func NewServer(manager *tunnel.Manager) *Server {
	return &Server{
		manager:   manager,
		listeners: make(map[int]*portListener),
	}
}

func (s *Server) Start() error {
	s.mu.Lock()
	s.stopped = false
	s.ctx, s.cancel = context.WithCancel(context.Background())
	if s.listeners == nil {
		s.listeners = make(map[int]*portListener)
	}
	s.mu.Unlock()
	err := s.Reload()
	global.NAV_LOG.Info("navmesh tcp mapping gateway started", zap.Int("listeners", s.ListenerCount()))
	return err
}

func (s *Server) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	cancel := s.cancel
	listeners := s.listeners
	s.listeners = make(map[int]*portListener)
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	for _, item := range listeners {
		_ = item.listener.Close()
	}
	s.wg.Wait()
	global.NAV_LOG.Info("navmesh tcp mapping gateway stopped")
}

func (s *Server) Reload() error {
	mappings, err := services.ServiceGroupApp.TCPMappingService.Enabled()
	if err != nil {
		return err
	}
	desired := make(map[int]domains.TCPMapping, len(mappings))
	for _, mapping := range mappings {
		if mapping.PublicPort <= 0 {
			continue
		}
		if _, exists := desired[mapping.PublicPort]; exists {
			global.NAV_LOG.Warn("duplicate enabled tcp mapping public port", zap.Int("publicPort", mapping.PublicPort), zap.String("mappingGuid", mapping.Guid))
			continue
		}
		desired[mapping.PublicPort] = mapping
	}

	var errs []error
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return nil
	}
	for port, mapping := range desired {
		if item := s.listeners[port]; item != nil {
			item.setMapping(mapping)
			continue
		}
		listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err != nil {
			errs = append(errs, fmt.Errorf("listen tcp public port %d: %w", port, err))
			continue
		}
		item := &portListener{port: port, listener: listener}
		item.setMapping(mapping)
		s.listeners[port] = item
		s.wg.Add(1)
		go s.acceptLoop(item)
		global.NAV_LOG.Info("tcp mapping listener started", zap.Int("publicPort", port), zap.String("mappingGuid", mapping.Guid))
	}
	for port, item := range s.listeners {
		if _, ok := desired[port]; ok {
			continue
		}
		delete(s.listeners, port)
		_ = item.listener.Close()
		global.NAV_LOG.Info("tcp mapping listener stopped", zap.Int("publicPort", port))
	}
	return errors.Join(errs...)
}

func (s *Server) ListenerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.listeners)
}

func (s *Server) acceptLoop(item *portListener) {
	defer s.wg.Done()
	for {
		conn, err := item.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || s.isStopped() {
				return
			}
			global.NAV_LOG.Warn("accept tcp mapping connection failed", zap.Int("publicPort", item.port), zap.Error(err))
			time.Sleep(200 * time.Millisecond)
			continue
		}
		ctx := s.serverContext()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(ctx, item, conn)
		}()
	}
}

func (s *Server) handleConn(ctx context.Context, item *portListener, client net.Conn) {
	defer client.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
		case <-done:
		}
	}()
	mapping := item.snapshot()
	sourceIP := remoteAddrIP(client.RemoteAddr())
	if mapping.Guid == "" || mapping.Status == int(domains.StatusDisabled) {
		return
	}
	var device domains.Device
	if err := global.NAV_DB.Where("guid = ? AND status != ?", mapping.DeviceGuid, domains.DeviceStatusDisabled).First(&device).Error; err != nil {
		global.NAV_LOG.Warn("tcp mapping device not found", zap.String("mappingGuid", mapping.Guid), zap.String("deviceGuid", mapping.DeviceGuid), zap.Error(err))
		return
	}
	if !services.ServiceGroupApp.AccessPolicyService.IsAllowed(device.Guid, mapping.Guid, "tcp") {
		global.NAV_LOG.Warn("tcp mapping access denied by policy", zap.String("mappingGuid", mapping.Guid), zap.String("deviceGuid", device.Guid), zap.String("sourceIp", sourceIP))
		return
	}
	permit, err := services.DefaultRuntimePolicy.Acquire(device.Guid, sourceIP)
	if err != nil {
		global.NAV_LOG.Warn("tcp mapping session rejected by runtime policy", zap.String("mappingGuid", mapping.Guid), zap.String("deviceGuid", device.Guid), zap.String("sourceIp", sourceIP), zap.Error(err))
		services.ServiceGroupApp.EventService.Record(services.EventInput{
			DeviceGuid: device.Guid,
			EventType:  "session_rejected",
			Level:      "warn",
			Title:      "tcp session rejected",
			Message:    err.Error(),
		})
		return
	}
	defer services.DefaultRuntimePolicy.Release(permit)

	session := createTCPSession(mapping, device, sourceIP)
	streamCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	upstream, err := s.manager.OpenTCPStream(streamCtx, device.Guid, mapping.TargetHost, mapping.TargetPort)
	cancel()
	if err != nil {
		global.NAV_LOG.Warn("open tcp mapping tunnel stream failed", zap.String("mappingGuid", mapping.Guid), zap.String("deviceGuid", device.Guid), zap.Int("publicPort", mapping.PublicPort), zap.Error(err))
		recordTCPOpenFailedEvent(device.Guid, err.Error())
		closeTCPSession(session.Guid, 0, 0, "open_tunnel_failed: "+err.Error())
		return
	}
	defer upstream.Close()
	services.DefaultSessionRegistry.RegisterSession(session.Guid, client, upstream)
	defer services.DefaultSessionRegistry.UnregisterSession(session.Guid)

	global.NAV_LOG.Info(
		"tcp mapping connection established",
		zap.String("mappingGuid", mapping.Guid),
		zap.String("deviceGuid", device.Guid),
		zap.Int("publicPort", mapping.PublicPort),
		zap.String("targetHost", mapping.TargetHost),
		zap.Int("targetPort", mapping.TargetPort),
		zap.String("sourceIp", sourceIP),
	)
	bytesIn, bytesOut := streambridge.Bridge(client, upstream, services.DefaultRuntimePolicy.IdleTimeout())
	reason := "closed"
	if services.DefaultSessionRegistry.ConsumeForceClosed(session.Guid) {
		reason = "closed_by_admin"
	}
	closeTCPSession(session.Guid, bytesIn, bytesOut, reason)
}

func recordTCPOpenFailedEvent(deviceGuid string, message string) bool {
	return services.ServiceGroupApp.EventService.RecordSuppressed(services.EventInput{
		DeviceGuid: deviceGuid,
		EventType:  "open_tcp_failed",
		Level:      "error",
		Title:      "open tcp mapping target failed",
		Message:    message,
	}, tcpOpenFailedEventSuppressionWindow)
}

func (s *Server) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func (s *Server) serverContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (l *portListener) setMapping(mapping domains.TCPMapping) {
	l.mu.Lock()
	l.mapping = mapping
	l.mu.Unlock()
}

func (l *portListener) snapshot() domains.TCPMapping {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.mapping
}

func createTCPSession(mapping domains.TCPMapping, device domains.Device, sourceIP string) domains.TunnelSession {
	now := domains.NowMilli()
	session := domains.TunnelSession{
		Guid:        uuid.NewString(),
		DeviceGuid:  device.Guid,
		SessionType: "tcp",
		SourceIP:    sourceIP,
		Username:    mapping.Name,
		TargetHost:  mapping.TargetHost,
		TargetPort:  mapping.TargetPort,
		PublicHost:  publicEndpoint(mapping),
		Status:      int(domains.StatusEnabled),
		StartTime:   now,
		CreateTime:  now,
		UpdateTime:  now,
	}
	_ = global.NAV_DB.Create(&session).Error
	return session
}

func closeTCPSession(guid string, bytesIn, bytesOut int64, reason string) {
	now := domains.NowMilli()
	_ = global.NAV_DB.Model(&domains.TunnelSession{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":            int(domains.StatusDisabled),
		"bytes_in":          bytesIn,
		"bytes_out":         bytesOut,
		"end_time":          now,
		"update_time":       now,
		"disconnect_reason": strings.TrimSpace(reason),
	}).Error
}

func publicEndpoint(mapping domains.TCPMapping) string {
	host := strings.TrimSpace(mapping.PublicHost)
	if host == "" {
		return ":" + strconv.Itoa(mapping.PublicPort)
	}
	return net.JoinHostPort(host, strconv.Itoa(mapping.PublicPort))
}

func remoteAddrIP(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return streambridge.NormalizeIP(host)
}
