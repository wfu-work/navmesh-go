package httpgateway

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"navmesh-go/services"
	"navmesh-go/tunnel"

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
	streamCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	upstream, err := s.manager.OpenTCPStream(streamCtx, device.Guid, mapping.TargetHost, mapping.TargetPort)
	if err != nil {
		http.Error(w, "device tunnel offline", http.StatusBadGateway)
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusBadGateway, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	defer upstream.Close()

	r.RequestURI = ""
	r.URL.Scheme = "http"
	r.URL.Host = net.JoinHostPort(mapping.TargetHost, intToString(mapping.TargetPort))
	r.Host = mapping.PublicHost
	if err := r.Write(upstream); err != nil {
		http.Error(w, "write upstream failed", http.StatusBadGateway)
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusBadGateway, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(upstream), r)
	if err != nil {
		http.Error(w, "read upstream failed", http.StatusBadGateway)
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusBadGateway, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	bytesOut, _ := io.Copy(w, resp.Body)
	writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: resp.StatusCode, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, BytesOut: bytesOut})
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
