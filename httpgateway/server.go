package httpgateway

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
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
	proxy := newHTTPReverseProxy(s.manager, mapping, device, host, r.RemoteAddr, requestPath, r.Method, start, bytesIn)
	proxy.ServeHTTP(w, r)
}

func sanitizeHopByHopRequestHeaders(header http.Header) {
	removeHopByHopHeaders(header)
	header.Set("Connection", "close")
}

func removeHopByHopHeaders(header http.Header) {
	for _, key := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(key)
	}
}

type httpReverseProxy struct {
	proxy *httputil.ReverseProxy
}

func newHTTPReverseProxy(manager *tunnel.Manager, mapping domains.PortMapping, device domains.Device, host, remoteAddr, requestPath, method string, start time.Time, bytesIn int64) *httpReverseProxy {
	tracker := &httpMappingTracker{
		manager:     manager,
		mapping:     mapping,
		device:      device,
		host:        host,
		remoteAddr:  remoteAddr,
		requestPath: requestPath,
		method:      method,
		start:       start,
		bytesIn:     bytesIn,
	}
	targetHostPort := net.JoinHostPort(mapping.TargetHost, intToString(mapping.TargetPort))
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = targetHostPort
			req.Host = mapping.PublicHost
			req.Close = true
			sanitizeHopByHopRequestHeaders(req.Header)
		},
		Transport: tracker,
		ModifyResponse: func(resp *http.Response) error {
			tracker.statusCode = resp.StatusCode
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			tracker.fail("proxy_error: " + err.Error())
			http.Error(rw, "device tunnel offline", http.StatusBadGateway)
		},
		FlushInterval: 50 * time.Millisecond,
	}
	return &httpReverseProxy{proxy: proxy}
}

func (p *httpReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.proxy.ServeHTTP(w, r)
}

type httpMappingTracker struct {
	manager     *tunnel.Manager
	mapping     domains.PortMapping
	device      domains.Device
	host        string
	remoteAddr  string
	requestPath string
	method      string
	start       time.Time
	bytesIn     int64
	bytesOut    int64
	statusCode  int
	upstream    io.Closer
	sessionGuid string
	once        sync.Once
}

func (t *httpMappingTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	streamCtx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
	defer cancel()
	upstream, err := t.manager.OpenTCPStream(streamCtx, t.device.Guid, t.mapping.TargetHost, t.mapping.TargetPort)
	if err != nil {
		t.fail(err.Error())
		services.ServiceGroupApp.EventService.Record(services.EventInput{
			DeviceGuid: t.device.Guid,
			EventType:  "open_tcp_failed",
			Level:      "error",
			Title:      "open http target failed",
			Message:    err.Error(),
		})
		return nil, err
	}
	t.upstream = upstream
	applyIdleDeadline(upstream, services.DefaultRuntimePolicy.IdleTimeout())
	session := createHTTPSession(t.mapping, t.device, t.requestPath, sourceIP(t.remoteAddr))
	t.sessionGuid = session.Guid
	services.DefaultSessionRegistry.RegisterSession(session.Guid, upstream)

	reqBody := req.Body
	if reqBody != nil && reqBody != http.NoBody {
		req.Body = &countingReadCloser{
			ReadCloser: reqBody,
			onRead: func(n int) {
				t.bytesIn += int64(n)
			},
		}
	}
	sanitizeHopByHopRequestHeaders(req.Header)
	req.Close = true
	req.URL.Scheme = "http"
	req.URL.Host = net.JoinHostPort(t.mapping.TargetHost, intToString(t.mapping.TargetPort))
	req.Host = t.mapping.PublicHost

	if err := req.Write(upstream); err != nil {
		t.fail("write_upstream_failed: " + err.Error())
		return nil, err
	}
	resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
	if err != nil {
		t.fail("read_upstream_failed: " + err.Error())
		return nil, err
	}
	resp.Body = &countingResponseBody{
		ReadCloser: resp.Body,
		onRead: func(n int) {
			t.bytesOut += int64(n)
		},
		onClose: func() {
			t.statusCode = resp.StatusCode
			reason := "closed"
			if isForceClosed(t.sessionGuid) {
				reason = "closed_by_admin"
			}
			t.finish(reason)
		},
	}
	if t.statusCode == 0 {
		t.statusCode = resp.StatusCode
	}
	return resp, nil
}

func (t *httpMappingTracker) fail(reason string) {
	t.finish(reason)
}

func (t *httpMappingTracker) finish(reason string) {
	if t == nil {
		return
	}
	t.once.Do(func() {
		if t.upstream != nil {
			_ = t.upstream.Close()
		}
		if t.sessionGuid != "" {
			services.DefaultSessionRegistry.UnregisterSession(t.sessionGuid)
			closeHTTPSession(t.sessionGuid, t.bytesIn, t.bytesOut, reason)
		}
		writeAccessLog(mappingLogInput{
			Mapping:      t.mapping,
			Device:       t.device,
			Host:         t.host,
			Method:       t.method,
			Path:         t.requestPath,
			SourceIP:     sourceIP(t.remoteAddr),
			StatusCode:   statusCodeOrDefault(t.statusCode),
			DurationMs:   time.Since(t.start).Milliseconds(),
			BytesIn:      t.bytesIn,
			BytesOut:     t.bytesOut,
			ErrorMessage: strings.TrimSpace(reason),
		})
	})
}

func statusCodeOrDefault(value int) int {
	if value <= 0 {
		return http.StatusBadGateway
	}
	return value
}

type countingReadCloser struct {
	io.ReadCloser
	onRead func(int)
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if n > 0 && c.onRead != nil {
		c.onRead(n)
	}
	return n, err
}

type countingResponseBody struct {
	io.ReadCloser
	onRead  func(int)
	onClose func()
	once    sync.Once
}

func (b *countingResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.onRead != nil {
		b.onRead(n)
	}
	return n, err
}

func (b *countingResponseBody) Close() error {
	var err error
	b.once.Do(func() {
		err = b.ReadCloser.Close()
		if b.onClose != nil {
			b.onClose()
		}
	})
	return err
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
