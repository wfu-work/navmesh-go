package services

import (
	"sync"

	"navmesh-go/domains"
)

type EventNotification struct {
	Type string        `json:"type"`
	Data domains.Event `json:"data"`
}

type EventHub struct {
	mu      sync.RWMutex
	clients map[chan EventNotification]struct{}
}

func NewEventHub() *EventHub {
	return &EventHub{clients: make(map[chan EventNotification]struct{})}
}

func (h *EventHub) Subscribe() chan EventNotification {
	ch := make(chan EventNotification, 8)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *EventHub) Unsubscribe(ch chan EventNotification) {
	if ch == nil {
		return
	}
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *EventHub) Publish(event domains.Event) {
	notification := EventNotification{Type: "event.created", Data: event}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- notification:
		default:
		}
	}
}
