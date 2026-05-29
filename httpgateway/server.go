package httpgateway

import (
	"context"
	"errors"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"navmesh-go/domains"
	"navmesh-go/services"
	"navmesh-go/tunnel"

	"github.com/google/uuid"
	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
)

type Server struct {
	addr      string
	server    *http.Server
	manager   *tunnel.Manager
	transport *http.Transport
}

func NewServer(addr string, manager *tunnel.Manager) *Server {
	if strings.TrimSpace(addr) == "" {
		addr = ":8080"
	}
	if manager == nil {
		manager = tunnel.DefaultManager
	}
	s := &Server{addr: addr, manager: manager}
	s.transport = newTunnelHTTPTransport(manager)
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
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
	if s.server != nil {
		_ = s.server.Shutdown(ctx)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	host := normalizeHost(r.Host)
	requestPath := r.URL.RequestURI()
	clientIP := requestSourceIP(r)
	bytesIn := r.ContentLength
	if bytesIn < 0 {
		bytesIn = 0
	}
	mapping, device, err := findMapping(host)
	if err != nil {
		renderGatewayErrorPage(w, gatewayErrorPage{
			StatusCode: http.StatusNotFound,
			Title:      "映射不存在",
			Subtitle:   "没有找到当前访问域名对应的 HTTP 映射。",
			Host:       host,
			Path:       requestPath,
			Hint:       "请确认域名是否填写正确，或在管理后台检查映射配置是否已启用。",
		})
		writeAccessLog(mappingLogInput{Host: host, Method: r.Method, Path: requestPath, SourceIP: clientIP, StatusCode: http.StatusNotFound, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	if !services.ServiceGroupApp.AccessPolicyService.IsAllowed(device.Guid, mapping.Guid, mapping.Protocol) {
		renderGatewayErrorPage(w, gatewayErrorPage{
			StatusCode:  http.StatusForbidden,
			Title:       "访问被拒绝",
			Subtitle:    "当前来源没有访问这个映射的权限。",
			Host:        host,
			Path:        requestPath,
			DeviceName:  displayDeviceName(device),
			MappingName: displayMappingName(mapping),
			Hint:        "如需访问，请在管理后台调整访问策略后再重试。",
		})
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: clientIP, StatusCode: http.StatusForbidden, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: "access policy denied"})
		return
	}
	permit, err := services.DefaultRuntimePolicy.Acquire(device.Guid, clientIP)
	if err != nil {
		renderGatewayErrorPage(w, gatewayErrorPage{
			StatusCode:  http.StatusTooManyRequests,
			Title:       "连接过多",
			Subtitle:    "当前设备的访问会话已达到限制。",
			Host:        host,
			Path:        requestPath,
			DeviceName:  displayDeviceName(device),
			MappingName: displayMappingName(mapping),
			Hint:        "请稍后刷新重试，或关闭不再使用的会话。",
		})
		services.ServiceGroupApp.EventService.Record(services.EventInput{
			DeviceGuid: device.Guid,
			EventType:  "session_rejected",
			Level:      "warn",
			Title:      "http session rejected",
			Message:    err.Error(),
		})
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: clientIP, StatusCode: http.StatusTooManyRequests, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	defer services.DefaultRuntimePolicy.Release(permit)
	proxy := newHTTPReverseProxy(s.transport, mapping, device, host, clientIP)
	proxy.ServeHTTP(w, r)
}

func sanitizeHopByHopRequestHeaders(header http.Header) {
	removeHopByHopHeaders(header)
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

func newHTTPReverseProxy(transport *http.Transport, mapping domains.PortMapping, device domains.Device, host, clientIP string) *httpReverseProxy {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = proxyPoolHost(mapping)
			req.Host = mapping.PublicHost
			sanitizeHopByHopRequestHeaders(req.Header)
			req.Close = true
		},
		Transport: &httpMappingRoundTripper{
			base:     transport,
			mapping:  mapping,
			device:   device,
			host:     host,
			sourceIP: clientIP,
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			title := "目标服务暂时不可用"
			subtitle := "网关暂时无法连接现场设备的目标服务。"
			hint := "请稍后刷新重试；如果持续出现，请检查设备客户端和本地 Web 服务是否正常运行。"
			if strings.Contains(strings.ToLower(err.Error()), "device tunnel offline") {
				title = "设备隧道未连接"
				subtitle = "现场设备当前不在线，HTTP 映射暂时无法访问。"
				hint = "设备客户端重新连接后页面会恢复访问，可以稍后刷新重试。"
			}
			renderGatewayErrorPage(rw, gatewayErrorPage{
				StatusCode:  http.StatusBadGateway,
				Title:       title,
				Subtitle:    subtitle,
				Host:        host,
				Path:        req.URL.RequestURI(),
				DeviceName:  displayDeviceName(device),
				MappingName: displayMappingName(mapping),
				Hint:        hint,
			})
		},
		FlushInterval: 50 * time.Millisecond,
	}
	return &httpReverseProxy{proxy: proxy}
}

func (p *httpReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.proxy.ServeHTTP(w, r)
}

type httpMappingContextKey struct{}

type httpMappingContext struct {
	Mapping  domains.PortMapping
	Device   domains.Device
	Host     string
	SourceIP string
}

type httpMappingRoundTripper struct {
	base     http.RoundTripper
	mapping  domains.PortMapping
	device   domains.Device
	host     string
	sourceIP string
}

func (rt *httpMappingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	requestPath := req.URL.RequestURI()
	requestMethod := req.Method
	source := rt.sourceIP
	var bytesIn int64
	var bytesOut int64
	statusCode := 0
	finishOnce := sync.Once{}
	finish := func(reason string) {
		finishOnce.Do(func() {
			writeAccessLog(mappingLogInput{
				Mapping:      rt.mapping,
				Device:       rt.device,
				Host:         rt.host,
				Method:       requestMethod,
				Path:         requestPath,
				SourceIP:     source,
				StatusCode:   statusCodeOrDefault(statusCode),
				DurationMs:   time.Since(start).Milliseconds(),
				BytesIn:      bytesIn,
				BytesOut:     atomic.LoadInt64(&bytesOut),
				ErrorMessage: strings.TrimSpace(reason),
			})
		})
	}
	if req.Body != nil && req.Body != http.NoBody {
		req.Body = &countingReadCloser{
			ReadCloser: req.Body,
			onRead: func(n int) {
				atomic.AddInt64(&bytesIn, int64(n))
			},
		}
	}
	ctx := context.WithValue(req.Context(), httpMappingContextKey{}, httpMappingContext{
		Mapping:  rt.mapping,
		Device:   rt.device,
		Host:     rt.host,
		SourceIP: rt.sourceIP,
	})
	req = req.WithContext(ctx)
	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		finish("proxy_error: " + err.Error())
		var dialErr *tunnelDialError
		if errors.As(err, &dialErr) {
			recordHTTPGatewayOpenFailed(rt.device.Guid, dialErr.Unwrap())
		}
		return nil, err
	}
	statusCode = resp.StatusCode
	if resp.Body == nil {
		finish("closed")
		return resp, nil
	}
	resp.Body = &countingResponseBody{
		ReadCloser: resp.Body,
		onRead: func(n int) {
			atomic.AddInt64(&bytesOut, int64(n))
		},
		onClose: func() {
			finish("closed")
		},
	}
	return resp, nil
}

func newTunnelHTTPTransport(manager *tunnel.Manager) *http.Transport {
	if manager == nil {
		manager = tunnel.DefaultManager
	}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           tunnelDialContext(manager),
		ForceAttemptHTTP2:     false,
		DisableKeepAlives:     true,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   64,
		MaxConnsPerHost:       128,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    false,
	}
}

func tunnelDialContext(manager *tunnel.Manager) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		info, ok := ctx.Value(httpMappingContextKey{}).(httpMappingContext)
		if !ok {
			return nil, net.InvalidAddrError("missing http mapping context")
		}
		streamCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		upstream, err := manager.OpenTCPStream(streamCtx, info.Device.Guid, info.Mapping.TargetHost, info.Mapping.TargetPort)
		if err != nil {
			return nil, &tunnelDialError{err: err}
		}
		session := createHTTPSession(info.Mapping, info.Device, info.Host, info.SourceIP)
		conn := &tunnelHTTPConn{
			ReadWriteCloser: upstream,
			sessionGuid:     session.Guid,
			sourceIP:        info.SourceIP,
		}
		applyIdleDeadline(conn, services.DefaultRuntimePolicy.IdleTimeout())
		services.DefaultSessionRegistry.RegisterSession(session.Guid, conn)
		return conn, nil
	}
}

func proxyPoolHost(mapping domains.PortMapping) string {
	key := strings.TrimSpace(mapping.Guid)
	if key == "" {
		key = strings.TrimSpace(mapping.PublicHost)
	}
	if key == "" {
		key = strings.TrimSpace(mapping.DeviceGuid)
	}
	if key == "" {
		key = "mapping"
	}
	return net.JoinHostPort(key+".navmesh-http.local", intToString(mapping.TargetPort))
}

type tunnelDialError struct {
	err error
}

func (e *tunnelDialError) Error() string {
	if e == nil || e.err == nil {
		return "open tunnel tcp stream failed"
	}
	return e.err.Error()
}

func (e *tunnelDialError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func recordHTTPGatewayOpenFailed(deviceGuid string, err error) {
	if err == nil {
		return
	}
	services.ServiceGroupApp.EventService.Record(services.EventInput{
		DeviceGuid: deviceGuid,
		EventType:  "open_tcp_failed",
		Level:      "error",
		Title:      "open http target failed",
		Message:    err.Error(),
	})
}

type tunnelHTTPConn struct {
	io.ReadWriteCloser
	sessionGuid string
	sourceIP    string
	bytesIn     atomic.Int64
	bytesOut    atomic.Int64
	closed      atomic.Bool
}

func (c *tunnelHTTPConn) Read(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Read(p)
	if n > 0 {
		c.bytesOut.Add(int64(n))
	}
	return n, err
}

func (c *tunnelHTTPConn) Write(p []byte) (int, error) {
	n, err := c.ReadWriteCloser.Write(p)
	if n > 0 {
		c.bytesIn.Add(int64(n))
	}
	return n, err
}

func (c *tunnelHTTPConn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	reason := "closed"
	if isForceClosed(c.sessionGuid) {
		reason = "closed_by_admin"
	}
	if c.sessionGuid != "" {
		services.DefaultSessionRegistry.UnregisterSession(c.sessionGuid)
		closeHTTPSession(c.sessionGuid, c.bytesIn.Load(), c.bytesOut.Load(), reason)
	}
	return c.ReadWriteCloser.Close()
}

func (c *tunnelHTTPConn) LocalAddr() net.Addr {
	return tunnelHTTPAddr("navmesh-http-gateway")
}

func (c *tunnelHTTPConn) RemoteAddr() net.Addr {
	if strings.TrimSpace(c.sourceIP) != "" {
		return tunnelHTTPAddr(c.sourceIP)
	}
	return tunnelHTTPAddr("navmesh-device")
}

func (c *tunnelHTTPConn) SetDeadline(t time.Time) error {
	if deadliner, ok := c.ReadWriteCloser.(interface{ SetDeadline(time.Time) error }); ok {
		return deadliner.SetDeadline(t)
	}
	return nil
}

func (c *tunnelHTTPConn) SetReadDeadline(t time.Time) error {
	if deadliner, ok := c.ReadWriteCloser.(interface{ SetReadDeadline(time.Time) error }); ok {
		return deadliner.SetReadDeadline(t)
	}
	return c.SetDeadline(t)
}

func (c *tunnelHTTPConn) SetWriteDeadline(t time.Time) error {
	if deadliner, ok := c.ReadWriteCloser.(interface{ SetWriteDeadline(time.Time) error }); ok {
		return deadliner.SetWriteDeadline(t)
	}
	return c.SetDeadline(t)
}

type tunnelHTTPAddr string

func (a tunnelHTTPAddr) Network() string { return "navmesh-tunnel" }

func (a tunnelHTTPAddr) String() string { return string(a) }

type gatewayErrorPage struct {
	StatusCode  int
	Title       string
	Subtitle    string
	Host        string
	Path        string
	DeviceName  string
	MappingName string
	Hint        string
}

var gatewayErrorTemplate = template.Must(template.New("gateway-error").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="robots" content="noindex,nofollow">
  <title>{{.Title}} - NavMesh</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #eef3fb;
      --panel: #fbfcff;
      --text: #1d2736;
      --muted: #627086;
      --line: #d6deeb;
      --line-strong: #b9c6d8;
      --brand: #255fe8;
      --brand-strong: #1747bf;
      --brand-text: #ffffff;
      --soft: #e8f0ff;
      --soft-text: #1747bf;
      --warn: #f59e0b;
      --shadow: 0 30px 90px rgba(31, 45, 68, .16);
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #111821;
        --panel: #18212d;
        --text: #edf3fb;
        --muted: #aab6c6;
        --line: #2d3a4a;
        --line-strong: #415165;
        --brand: #78a6ff;
        --brand-strong: #a5c2ff;
        --brand-text: #0d1420;
        --soft: #20314c;
        --soft-text: #b9d1ff;
        --warn: #fbbf24;
        --shadow: 0 30px 90px rgba(0, 0, 0, .38);
      }
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: clamp(24px, 6vw, 72px) 18px;
      background: linear-gradient(145deg, var(--bg), color-mix(in srgb, var(--bg) 82%, #d7e2f5));
      color: var(--text);
      font: 14px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
    }
    main {
      position: relative;
      width: min(760px, 100%);
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
      overflow: hidden;
    }
    main::before {
      content: "";
      display: block;
      height: 6px;
      background: linear-gradient(90deg, var(--brand), var(--warn));
    }
    .content {
      padding: clamp(28px, 5vw, 48px);
    }
    .topline {
      display: flex;
      align-items: center;
      gap: 10px;
      margin-bottom: 22px;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      height: 30px;
      padding: 0 12px;
      border-radius: 999px;
      background: var(--soft);
      color: var(--soft-text);
      font-weight: 800;
      letter-spacing: .03em;
    }
    .brand {
      color: var(--muted);
      font-weight: 700;
    }
    h1 {
      margin: 0;
      max-width: 11em;
      font-size: clamp(34px, 6vw, 56px);
      line-height: 1.05;
      letter-spacing: 0;
    }
    p { margin: 0; }
    .subtitle {
      margin-top: 14px;
      max-width: 34em;
      color: var(--muted);
      font-size: clamp(16px, 2vw, 18px);
      font-weight: 650;
    }
    dl {
      display: grid;
      grid-template-columns: minmax(92px, max-content) minmax(0, 1fr);
      gap: 12px 26px;
      margin: 32px 0 0;
      padding: 22px 24px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: color-mix(in srgb, var(--panel) 76%, var(--bg));
    }
    dt {
      color: var(--muted);
      white-space: nowrap;
      font-weight: 750;
    }
    dd {
      margin: 0;
      min-width: 0;
      word-break: break-word;
      color: var(--text);
      font-weight: 700;
    }
    .hint {
      margin-top: 28px;
      color: var(--muted);
      font-size: 15px;
    }
    .actions {
      display: flex;
      margin-top: 30px;
    }
    a {
      min-height: 44px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      border-radius: 999px;
      padding: 0 22px;
      font: inherit;
      font-weight: 800;
      cursor: pointer;
      text-decoration: none;
    }
    .primary {
      background: var(--brand);
      border-color: var(--brand);
      color: var(--brand-text);
      box-shadow: 0 12px 26px color-mix(in srgb, var(--brand) 24%, transparent);
      transition: transform .16s ease, background-color .16s ease, box-shadow .16s ease;
    }
    .primary:hover {
      background: var(--brand-strong);
      transform: translateY(-1px);
      box-shadow: 0 16px 32px color-mix(in srgb, var(--brand) 30%, transparent);
    }
    .primary:focus-visible {
      outline: 3px solid color-mix(in srgb, var(--brand) 28%, transparent);
      outline-offset: 3px;
    }
    @media (prefers-reduced-motion: reduce) {
      .primary { transition: none; }
      .primary:hover { transform: none; }
    }
    @media (max-width: 520px) {
      body { align-items: stretch; }
      .content { padding: 26px 20px 30px; }
      dl { grid-template-columns: 1fr; gap: 4px; }
      dt { font-size: 12px; }
      .actions { display: block; }
      a { width: 100%; }
    }
  </style>
</head>
<body>
  <main>
    <div class="content">
      <div class="topline">
        <span class="badge">HTTP {{.StatusCode}}</span>
        <span class="brand">NavMesh Gateway</span>
      </div>
      <h1>{{.Title}}</h1>
      <p class="subtitle">{{.Subtitle}}</p>
      <dl>
        <dt>访问域名</dt><dd>{{.Host}}</dd>
        <dt>请求路径</dt><dd>{{.Path}}</dd>
        {{if .DeviceName}}<dt>现场设备</dt><dd>{{.DeviceName}}</dd>{{end}}
        {{if .MappingName}}<dt>映射名称</dt><dd>{{.MappingName}}</dd>{{end}}
      </dl>
      {{if .Hint}}<p class="hint">{{.Hint}}</p>{{end}}
      <div class="actions">
        <a class="primary" href="javascript:location.reload()">刷新重试</a>
      </div>
    </div>
  </main>
</body>
</html>`))

func renderGatewayErrorPage(w http.ResponseWriter, page gatewayErrorPage) {
	if page.StatusCode <= 0 {
		page.StatusCode = http.StatusBadGateway
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(page.StatusCode)
	if err := gatewayErrorTemplate.Execute(w, page); err != nil {
		global.NAV_LOG.Warn("render http gateway error page failed", zap.Error(err))
	}
}

func displayDeviceName(device domains.Device) string {
	if value := strings.TrimSpace(device.Alias); value != "" {
		return value
	}
	if value := strings.TrimSpace(device.Sncode); value != "" {
		return value
	}
	return strings.TrimSpace(device.Guid)
}

func displayMappingName(mapping domains.PortMapping) string {
	if value := strings.TrimSpace(mapping.Name); value != "" {
		return value
	}
	return strings.TrimSpace(mapping.PublicHost)
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
