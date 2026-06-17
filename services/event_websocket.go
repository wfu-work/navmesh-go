package services

import (
	"net/http"

	"golang.org/x/net/websocket"
)

type EventWebSocket struct {
	Hub *EventHub
}

func NewEventWebSocket(hub *EventHub) *EventWebSocket {
	return &EventWebSocket{Hub: hub}
}

func (s *EventWebSocket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server := websocket.Server{
		Handshake: func(config *websocket.Config, req *http.Request) error {
			config.Origin, _ = websocket.Origin(config, req)
			return nil
		},
		Handler: s.handle,
	}
	server.ServeHTTP(w, r)
}

func (s *EventWebSocket) handle(ws *websocket.Conn) {
	defer ws.Close()
	if s == nil || s.Hub == nil {
		return
	}
	ch := s.Hub.Subscribe()
	defer s.Hub.Unsubscribe(ch)

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		var message string
		for {
			if err := websocket.Message.Receive(ws, &message); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-clientDone:
			return
		case notification, ok := <-ch:
			if !ok {
				return
			}
			if err := websocket.JSON.Send(ws, notification); err != nil {
				return
			}
		}
	}
}
