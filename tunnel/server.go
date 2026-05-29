package tunnel

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"

	"navmesh-go/services"

	"github.com/quic-go/quic-go"
	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
)

type Server struct {
	addr     string
	manager  *Manager
	listener *quic.Listener
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewServer(addr string, manager *Manager) *Server {
	if strings.TrimSpace(addr) == "" {
		addr = ":3008"
	}
	if manager == nil {
		manager = DefaultManager
	}
	return &Server{addr: addr, manager: manager}
}

func (s *Server) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	listener, err := quic.ListenAddr(s.addr, generateTLSConfig(), &quic.Config{
		KeepAlivePeriod: 30 * time.Second,
		MaxIdleTimeout:  90 * time.Second,
	})
	if err != nil {
		return err
	}
	s.listener = listener
	s.wg.Add(1)
	go s.acceptLoop(ctx)
	global.NAV_LOG.Info("navmesh quic tunnel server started", zap.String("addr", s.addr))
	return nil
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.manager.CloseAll()
	s.wg.Wait()
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			global.NAV_LOG.Warn("accept quic tunnel failed", zap.Error(err))
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(ctx, conn)
		}()
	}
}

func (s *Server) handleConnection(ctx context.Context, conn *quic.Conn) {
	authCtx, cancel := contextWithTimeout(ctx, 10*time.Second)
	defer cancel()
	stream, err := conn.AcceptStream(authCtx)
	if err != nil {
		_ = conn.CloseWithError(1, "hello timeout")
		return
	}
	frame, err := readFrame(stream)
	if err != nil || frame.Type != FrameTypeHello {
		_ = writeFrame(stream, Frame{Type: FrameTypeError, OK: false, Message: "hello required"})
		_ = conn.CloseWithError(1, "invalid hello")
		return
	}
	device, err := services.ServiceGroupApp.DeviceService.Authenticate(
		frame.Token,
		frame.DeviceGuid,
		frame.SnCode,
		conn.RemoteAddr().String(),
		frame.HostIP,
		frame.WanIP,
		frame.Hostname,
		frame.ClientVersion,
	)
	if err != nil {
		_ = writeFrame(stream, Frame{Type: FrameTypeError, OK: false, Message: err.Error()})
		_ = conn.CloseWithError(1, "auth failed")
		return
	}
	s.manager.Register(*device, conn)
	_ = writeFrame(stream, Frame{Type: FrameTypeHelloAck, OK: true, DeviceGuid: device.Guid, SnCode: device.Sncode})
	_ = stream.Close()
	defer func() {
		s.manager.Unregister(device.Guid)
		s.manager.SetOffline(device.Guid)
		services.ServiceGroupApp.EventService.Record(services.EventInput{
			DeviceGuid: device.Guid,
			EventType:  "device_offline",
			Level:      "warn",
			Title:      "device tunnel offline",
			Message:    conn.RemoteAddr().String(),
		})
	}()

	for {
		next, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleStream(ctx, device.Guid, next)
		}()
	}
}

func (s *Server) handleStream(ctx context.Context, deviceGuid string, stream *quic.Stream) {
	defer stream.Close()
	frame, err := readFrame(stream)
	if err != nil {
		global.NAV_LOG.Warn("read tunnel stream frame failed", zap.String("deviceGuid", deviceGuid), zap.Error(err))
		return
	}
	s.manager.Touch(deviceGuid)
	switch frame.Type {
	case FrameTypeHeartbeat, FrameTypePing:
		_ = writeFrame(stream, Frame{Type: FrameTypePong, OK: true, RequestID: frame.RequestID})
	case FrameTypeOpenTCPAck:
		_ = writeFrame(stream, Frame{Type: FrameTypeOpenTCPAck, OK: true, RequestID: frame.RequestID})
	default:
		_ = writeFrame(stream, Frame{Type: FrameTypeError, OK: false, RequestID: frame.RequestID, Message: "unsupported frame type"})
	}
}

func readFrame(stream *quic.Stream) (Frame, error) {
	line, err := readFrameLine(stream)
	if err != nil {
		return Frame{}, err
	}
	var frame Frame
	if err := json.Unmarshal(line, &frame); err != nil {
		return Frame{}, err
	}
	return frame, nil
}

func readFrameLine(r io.Reader) ([]byte, error) {
	line := make([]byte, 0, 256)
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line = append(line, buf[0])
			if buf[0] == '\n' {
				return line, nil
			}
			if len(line) > 64*1024 {
				return nil, errors.New("frame too large")
			}
		}
		if err != nil {
			return nil, err
		}
	}
}

func writeFrame(stream *quic.Stream, frame Frame) error {
	data, err := EncodeFrame(frame)
	if err != nil {
		return err
	}
	_, err = stream.Write(data)
	return err
}

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"navmesh-tunnel"},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"navmesh-quic"},
		MinVersion:   tls.VersionTLS13,
	}
}
