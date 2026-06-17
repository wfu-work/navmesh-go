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

func TestRecordPublishesEventNotification(t *testing.T) {
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

	input := EventInput{
		DeviceGuid: "device-1",
		EventType:  "device_offline",
		Level:      "warning",
		Title:      "设备离线提醒",
		Message:    "heartbeat timeout",
	}
	ServiceGroupApp.EventService.WithDB(db).RecordAt(input, 1000)

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
		t.Fatal("expected event notification")
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
