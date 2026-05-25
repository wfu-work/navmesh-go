package httpgateway

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/services"
	"navmesh-go/tunnel"

	"github.com/google/uuid"
	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
)

type Server struct {
	addr    string
	server  *http.Server
	manager *tunnel.Manager
}

func NewServer(addr string, manager *tunnel.Manager) *Server {
	if strings.TrimSpace(addr) == "" {
		addr = ":8080"
	}
	if manager == nil {
		manager = tunnel.DefaultManager
	}
	s := &Server{addr: addr, manager: manager}
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s,
		ReadTimeout:  10 * time.Minute,
		WriteTimeout: 10 * time.Minute,
	}
	return s
}

func (s *Server) Start() error {
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			global.NAV_LOG.Error("navmesh http mapping gateway failed", zap.String("addr", s.addr), zap.Error(err))
		}
	}()
	global.NAV_LOG.Info("navmesh http mapping gateway started", zap.String("addr", s.addr))
	return nil
}

func (s *Server) Stop(ctx context.Context) {
	if s.server != nil {
		_ = s.server.Shutdown(ctx)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	host := normalizeHost(r.Host)
	requestPath := r.URL.RequestURI()
	bytesIn := r.ContentLength
	if bytesIn < 0 {
		bytesIn = 0
	}
	mapping, device, err := findMapping(host)
	if err != nil {
		http.Error(w, "mapping not found", http.StatusNotFound)
		writeAccessLog(mappingLogInput{Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusNotFound, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	if !services.ServiceGroupApp.AccessPolicyService.IsAllowed(device.Guid, mapping.Guid, mapping.Protocol) {
		http.Error(w, "mapping forbidden", http.StatusForbidden)
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusForbidden, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: "access policy denied"})
		return
	}
	permit, err := services.DefaultRuntimePolicy.Acquire(device.Guid, sourceIP(r.RemoteAddr))
	if err != nil {
		http.Error(w, "session rejected", http.StatusTooManyRequests)
		services.ServiceGroupApp.EventService.Record(services.EventInput{
			DeviceGuid: device.Guid,
			EventType:  "session_rejected",
			Level:      "warn",
			Title:      "http session rejected",
			Message:    err.Error(),
		})
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusTooManyRequests, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	defer services.DefaultRuntimePolicy.Release(permit)
	streamCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	upstream, err := s.manager.OpenTCPStream(streamCtx, device.Guid, mapping.TargetHost, mapping.TargetPort)
	if err != nil {
		services.ServiceGroupApp.EventService.Record(services.EventInput{
			DeviceGuid: device.Guid,
			EventType:  "open_tcp_failed",
			Level:      "error",
			Title:      "open http target failed",
			Message:    err.Error(),
		})
		http.Error(w, "device tunnel offline", http.StatusBadGateway)
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusBadGateway, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	defer upstream.Close()
	applyIdleDeadline(upstream, services.DefaultRuntimePolicy.IdleTimeout())
	session := createHTTPSession(mapping, device, requestPath, sourceIP(r.RemoteAddr))
	services.DefaultSessionRegistry.RegisterSession(session.Guid, upstream)
	defer services.DefaultSessionRegistry.UnregisterSession(session.Guid)

	r.RequestURI = ""
	r.URL.Scheme = "http"
	r.URL.Host = net.JoinHostPort(mapping.TargetHost, intToString(mapping.TargetPort))
	r.Host = mapping.PublicHost
	if err := r.Write(upstream); err != nil {
		http.Error(w, "write upstream failed", http.StatusBadGateway)
		closeHTTPSession(session.Guid, 0, 0, "write_upstream_failed: "+err.Error())
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusBadGateway, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(upstream), r)
	if err != nil {
		http.Error(w, "read upstream failed", http.StatusBadGateway)
		closeHTTPSession(session.Guid, 0, 0, "read_upstream_failed: "+err.Error())
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusBadGateway, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	bytesOut, _ := io.Copy(w, resp.Body)
	reason := "closed"
	if isForceClosed(session.Guid) {
		reason = "closed_by_admin"
	}
	closeHTTPSession(session.Guid, bytesIn, bytesOut, reason)
	writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: resp.StatusCode, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, BytesOut: bytesOut})
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func createHTTPSession(mapping domains.PortMapping, device domains.Device, requestPath, sourceIP string) domains.TunnelSession {
	now := domains.NowMilli()
	session := domains.TunnelSession{
		Guid:        uuid.NewString(),
		DeviceGuid:  device.Guid,
		SessionType: "http",
		SourceIP:    sourceIP,
		TargetHost:  mapping.TargetHost,
		TargetPort:  mapping.TargetPort,
		PublicHost:  mapping.PublicHost,
		Status:      int(domains.StatusEnabled),
		StartTime:   now,
		CreateTime:  now,
		UpdateTime:  now,
	}
	if requestPath != "" {
		session.Username = requestPath
	}
	_ = global.NAV_DB.Create(&session).Error
	return session
}

func closeHTTPSession(guid string, bytesIn, bytesOut int64, reason string) {
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

func isForceClosed(guid string) bool {
	var session domains.TunnelSession
	if err := global.NAV_DB.Select("force_closed").Where("guid = ?", guid).First(&session).Error; err != nil {
		return false
	}
	return session.ForceClosed
}

func applyIdleDeadline(conn io.ReadWriteCloser, timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	if deadliner, ok := conn.(interface{ SetDeadline(time.Time) error }); ok {
		_ = deadliner.SetDeadline(time.Now().Add(timeout))
	}
}
