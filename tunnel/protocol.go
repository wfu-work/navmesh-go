package tunnel

import "encoding/json"

const (
	FrameTypeHello      = "hello"
	FrameTypeHelloAck   = "hello_ack"
	FrameTypeHeartbeat  = "heartbeat"
	FrameTypePing       = "ping"
	FrameTypePong       = "pong"
	FrameTypeOpenTCP    = "open_tcp"
	FrameTypeOpenTCPAck = "open_tcp_ack"
	FrameTypeError      = "error"
)

const (
	TransportQUIC = "quic"
	TransportTCP  = "tcp"

	RoleLegacy  = ""
	RoleControl = "control"
	RoleData    = "data"
)

type Frame struct {
	Type          string `json:"type"`
	Role          string `json:"role,omitempty"`
	Transport     string `json:"transport,omitempty"`
	RequestID     string `json:"requestId,omitempty"`
	Token         string `json:"token,omitempty"`
	DeviceGuid    string `json:"deviceGuid,omitempty"`
	SnCode        string `json:"sncode,omitempty"`
	HostIP        string `json:"hostIp,omitempty"`
	WanIP         string `json:"wanIp,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	ClientVersion string `json:"clientVersion,omitempty"`
	TargetHost    string `json:"targetHost,omitempty"`
	TargetPort    int    `json:"targetPort,omitempty"`
	OK            bool   `json:"ok,omitempty"`
	Message       string `json:"message,omitempty"`
}

func EncodeFrame(frame Frame) ([]byte, error) {
	data, err := json.Marshal(frame)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
