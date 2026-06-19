package services

import (
	"testing"
	"time"

	"navmesh-go/domains"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecordSuppressedLimitsEventsWithinWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Event{}); err != nil {
		t.Fatalf("migrate events: %v", err)
	}

	service := ServiceGroupApp.EventService.WithDB(db)
	input := EventInput{
		DeviceGuid: "device-1",
		EventType:  "open_tcp_failed",
		Level:      "error",
		Title:      "open ssh target failed",
		Message:    "connection refused",
	}
	start := time.Date(2026, 6, 17, 8, 0, 0, 0, time.Local).UnixMilli()

	if !service.RecordSuppressedAt(input, 2*time.Hour, start) {
		t.Fatal("first event should be recorded")
	}
	if service.RecordSuppressedAt(input, 2*time.Hour, start+90*time.Minute.Milliseconds()) {
		t.Fatal("second event inside suppression window should be skipped")
	}
	assertSuppressedEventCount(t, db, input, 1)

	if !service.RecordSuppressedAt(input, 2*time.Hour, start+121*time.Minute.Milliseconds()) {
		t.Fatal("event after suppression window should be recorded")
	}
	assertSuppressedEventCount(t, db, input, 2)
}

func TestRecordPublishesWebSocketNotificationsForMessageEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Event{}); err != nil {
		t.Fatalf("migrate events: %v", err)
	}

	oldHub := ServiceGroupApp.EventHub
	hub := NewEventHub()
	ServiceGroupApp.EventHub = hub
	t.Cleanup(func() {
		ServiceGroupApp.EventHub = oldHub
	})
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	cases := []EventInput{
		{
			DeviceGuid: "device-1",
			EventType:  deviceOfflineEventType,
			Level:      "warning",
			Title:      "设备离线提醒",
			Message:    "heartbeat timeout",
		},
		{
			DeviceGuid: "device-1",
			EventType:  diskUsageHighEventType,
			Level:      "warning",
			Title:      "磁盘空间不足",
			Message:    "disk usage high",
		},
		{
			DeviceGuid: "device-1",
			EventType:  "client_upgrade",
			Level:      "info",
			Title:      "客户端升级成功",
			Message:    "v0.0.4",
		},
		{
			DeviceGuid: "device-1",
			EventType:  "vpn_restart",
			Level:      "info",
			Title:      "VPN 重启指令已创建",
			Message:    "等待客户端心跳执行",
		},
	}

	for index, input := range cases {
		ServiceGroupApp.EventService.WithDB(db).RecordAt(input, int64(1000+index))

		select {
		case notification := <-ch:
			if notification.Type != "event.created" {
				t.Fatalf("notification type = %q, want event.created", notification.Type)
			}
			if notification.Data.Guid == "" {
				t.Fatal("notification event guid should be set")
			}
			if notification.Data.EventType != input.EventType {
				t.Fatalf("notification event type = %q, want %q", notification.Data.EventType, input.EventType)
			}
		case <-time.After(time.Second):
			t.Fatalf("expected event notification for %s", input.EventType)
		}
	}
}

func TestHTTPGatewayFailuresDoNotEnterEventCenter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.Event{}); err != nil {
		t.Fatalf("migrate events: %v", err)
	}

	service := ServiceGroupApp.EventService.WithDB(db)
	events := []EventInput{
		{
			DeviceGuid: "device-1",
			EventType:  httpGatewayOpenTCPFailedEventType,
			Level:      "error",
			Title:      httpGatewayOpenTCPFailedEventTitle,
			Message:    "device tunnel offline",
		},
		{
			DeviceGuid: "device-1",
			EventType:  httpGatewaySessionRejectedEventType,
			Level:      "warn",
			Title:      httpGatewaySessionRejectedEventTitle,
			Message:    "max device sessions exceeded",
		},
		{
			DeviceGuid: "device-1",
			EventType:  "open_tcp_failed",
			Level:      "error",
			Title:      "open ssh target failed",
			Message:    "connection refused",
		},
		{
			DeviceGuid: "device-1",
			EventType:  deviceOfflineEventType,
			Level:      "warning",
			Title:      "设备离线提醒",
			Message:    "heartbeat timeout",
		},
	}
	for index, input := range events {
		service.RecordAt(input, int64(1000+index))
	}

	var stored int64
	if err := db.Model(&domains.Event{}).Count(&stored).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if stored != 2 {
		t.Fatalf("stored events = %d, want 2", stored)
	}

	items, total, err := service.List(map[string]string{"page": "1", "size": "20"})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if total != 2 {
		t.Fatalf("listed total = %d, want 2", total)
	}
	if len(items) != 2 {
		t.Fatalf("listed events = %d, want 2", len(items))
	}
	for _, item := range items {
		if isEventCenterNoise(EventInput{EventType: item.EventType, Title: item.Title}) {
			t.Fatalf("noise event should not be listed: %#v", item)
		}
	}
}

func assertSuppressedEventCount(t *testing.T, db *gorm.DB, input EventInput, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&domains.Event{}).
		Where("device_guid = ? AND event_type = ? AND title = ?", input.DeviceGuid, input.EventType, input.Title).
		Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != want {
		t.Fatalf("event count = %d, want %d", count, want)
	}
}
