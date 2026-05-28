package httpgateway

import (
	"bufio"
	"context"
	"html/template"
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
