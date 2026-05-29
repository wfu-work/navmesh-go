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
		writeAccessLog(mappingLogInput{Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusNotFound, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
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
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusForbidden, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: "access policy denied"})
		return
	}
	permit, err := services.DefaultRuntimePolicy.Acquire(device.Guid, sourceIP(r.RemoteAddr))
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
		writeAccessLog(mappingLogInput{Mapping: mapping, Device: device, Host: host, Method: r.Method, Path: requestPath, SourceIP: sourceIP(r.RemoteAddr), StatusCode: http.StatusTooManyRequests, DurationMs: time.Since(start).Milliseconds(), BytesIn: bytesIn, ErrorMessage: err.Error()})
		return
	}
	defer services.DefaultRuntimePolicy.Release(permit)
	proxy := newHTTPReverseProxy(s.transport, mapping, device, host, r.RemoteAddr)
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

func newHTTPReverseProxy(transport *http.Transport, mapping domains.PortMapping, device domains.Device, host, remoteAddr string) *httpReverseProxy {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = proxyPoolHost(mapping)
			req.Host = mapping.PublicHost
			req.Close = false
			sanitizeHopByHopRequestHeaders(req.Header)
		},
		Transport: &httpMappingRoundTripper{
			base:       transport,
			mapping:    mapping,
			device:     device,
			host:       host,
			remoteAddr: remoteAddr,
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
	Mapping    domains.PortMapping
	Device     domains.Device
	Host       string
	RemoteAddr string
}

type httpMappingRoundTripper struct {
	base       http.RoundTripper
	mapping    domains.PortMapping
	device     domains.Device
	host       string
	remoteAddr string
}

func (rt *httpMappingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	requestPath := req.URL.RequestURI()
	requestMethod := req.Method
	source := sourceIP(rt.remoteAddr)
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
		Mapping:    rt.mapping,
		Device:     rt.device,
		Host:       rt.host,
		RemoteAddr: rt.remoteAddr,
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
		session := createHTTPSession(info.Mapping, info.Device, info.Host, sourceIP(info.RemoteAddr))
		conn := &tunnelHTTPConn{
			ReadWriteCloser: upstream,
			sessionGuid:     session.Guid,
			sourceIP:        sourceIP(info.RemoteAddr),
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
      --bg: #f6f8fb;
      --panel: #ffffff;
      --text: #1f2937;
      --muted: #667085;
      --line: #d9e1ec;
      --brand: #2563eb;
      --brand-text: #ffffff;
      --soft: #eef4ff;
      --soft-text: #1d4ed8;
      --shadow: 0 20px 60px rgba(15, 23, 42, .12);
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #0f141b;
        --panel: #171d26;
        --text: #edf2f7;
        --muted: #a2adbd;
        --line: #2a3442;
        --brand: #60a5fa;
        --brand-text: #06111f;
        --soft: #13243a;
        --soft-text: #9cc8ff;
        --shadow: 0 22px 70px rgba(0, 0, 0, .34);
      }
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 32px 18px;
      background:
        radial-gradient(circle at 20% 8%, rgba(37, 99, 235, .12), transparent 28%),
        linear-gradient(180deg, var(--bg), color-mix(in srgb, var(--bg) 92%, #2563eb));
      color: var(--text);
      font: 14px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
    }
    main {
      width: min(680px, 100%);
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 14px;
      box-shadow: var(--shadow);
      padding: 34px;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      height: 28px;
      padding: 0 10px;
      border-radius: 999px;
      background: var(--soft);
      color: var(--soft-text);
      font-weight: 700;
      letter-spacing: .02em;
    }
    h1 {
      margin: 18px 0 8px;
      font-size: clamp(26px, 5vw, 38px);
      line-height: 1.15;
      letter-spacing: 0;
    }
    p { margin: 0; }
    .subtitle {
      color: var(--muted);
      font-size: 16px;
    }
    dl {
      display: grid;
      grid-template-columns: 92px minmax(0, 1fr);
      gap: 10px 14px;
      margin: 26px 0;
      padding: 18px;
      border: 1px solid var(--line);
      border-radius: 10px;
      background: color-mix(in srgb, var(--panel) 88%, var(--bg));
    }
    dt {
      color: var(--muted);
      white-space: nowrap;
    }
    dd {
      margin: 0;
      min-width: 0;
      word-break: break-word;
      color: var(--text);
    }
    .hint {
      margin-top: 4px;
      color: var(--muted);
    }
    .actions {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      margin-top: 28px;
    }
    a, button {
      height: 40px;
      border-radius: 20px;
      padding: 0 18px;
      border: 1px solid var(--line);
      font: inherit;
      font-weight: 700;
      cursor: pointer;
      text-decoration: none;
    }
    .primary {
      display: inline-flex;
      align-items: center;
      background: var(--brand);
      border-color: var(--brand);
      color: var(--brand-text);
    }
    button {
      background: transparent;
      color: var(--text);
    }
    @media (max-width: 520px) {
      main { padding: 26px 20px; }
      dl { grid-template-columns: 1fr; gap: 4px; }
      dt { font-size: 12px; }
      .actions { flex-direction: column; }
      a, button { width: 100%; text-align: center; justify-content: center; }
    }
  </style>
</head>
<body>
  <main>
    <span class="badge">HTTP {{.StatusCode}}</span>
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
      <button type="button" onclick="history.length > 1 ? history.back() : location.href='/'">返回上一页</button>
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
