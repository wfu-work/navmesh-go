package services

import (
	"errors"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

type EventService struct {
	commonServices.CrudService[domains.Event]
}

const (
	ignoredServiceLogEventType           = "service_log"
	httpGatewayOpenTCPFailedEventType    = "open_tcp_failed"
	httpGatewayOpenTCPFailedEventTitle   = "open http target failed"
	httpGatewaySessionRejectedEventType  = "session_rejected"
	httpGatewaySessionRejectedEventTitle = "http connection rejected"
	httpEventTitleMarker                 = "http"
)

func (s EventService) WithDB(db *gorm.DB) EventService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type EventInput struct {
	DeviceGuid string
	EventType  string
	Level      string
	Title      string
	Message    string
}

func (s EventService) List(params map[string]string) ([]domains.Event, int64, error) {
	db := s.DB().Model(&domains.Event{}).Where("event_type <> ?", ignoredServiceLogEventType)
	db = withoutEventCenterNoise(db)
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	if eventType := strings.TrimSpace(params["eventType"]); eventType != "" {
		db = db.Where("event_type = ?", eventType)
	}
	if level := strings.TrimSpace(params["level"]); level != "" {
		db = db.Where("level = ?", level)
	}
	if statusParam, ok := params["status"]; ok {
		statusParam = strings.TrimSpace(statusParam)
		if statusParam != "" {
			db = db.Where("status = ?", utils.Str2Int(statusParam))
		}
	}
	return queryPageCursor[domains.Event](db, params, DefaultMaxPageSize, "create_time", "create_time DESC, id DESC")
}

func (s EventService) Ack(guid string) error {
	return s.setStatus(guid, int(domains.StatusDisabled))
}

func (s EventService) AckAll() (int64, error) {
	db := s.DB().Model(&domains.Event{}).
		Where("status = ?", int(domains.StatusEnabled)).
		Where("event_type <> ?", ignoredServiceLogEventType)
	db = withoutEventCenterNoise(db)
	result := db.Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	})
	return result.RowsAffected, result.Error
}

func (s EventService) Close(guid string) error {
	return s.setStatus(guid, int(domains.StatusDisabled))
}

func (s EventService) Record(input EventInput) {
	s.RecordAt(input, domains.NowMilli())
}

func (s EventService) RecordSuppressed(input EventInput, window time.Duration) bool {
	return s.RecordSuppressedAt(input, window, domains.NowMilli())
}

func (s EventService) RecordSuppressedAt(input EventInput, window time.Duration, now int64) bool {
	input.EventType = strings.TrimSpace(input.EventType)
	input.Title = strings.TrimSpace(input.Title)
	if shouldIgnoreEventInput(input) {
		return false
	}
	if window <= 0 {
		s.RecordAt(input, now)
		return true
	}
	input.DeviceGuid = strings.TrimSpace(input.DeviceGuid)
	if input.DeviceGuid == "" || input.EventType == "" || input.Title == "" {
		s.RecordAt(input, now)
		return true
	}
	if now <= 0 {
		now = domains.NowMilli()
	}
	var existing domains.Event
	result := s.DB().Model(&domains.Event{}).
		Select("id").
		Where("device_guid = ? AND event_type = ? AND title = ? AND create_time >= ?", input.DeviceGuid, input.EventType, input.Title, now-window.Milliseconds()).
		Limit(1).
		Find(&existing)
	if result.Error != nil || result.RowsAffected > 0 {
		return false
	}
	s.RecordAt(input, now)
	return true
}

func (s EventService) RecordAt(input EventInput, now int64) {
	input.EventType = strings.TrimSpace(input.EventType)
	input.Title = strings.TrimSpace(input.Title)
	if shouldIgnoreEventInput(input) {
		return
	}
	level := strings.TrimSpace(input.Level)
	if level == "" {
		level = "info"
	}
	if now <= 0 {
		now = domains.NowMilli()
	}
	event := domains.Event{
		BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now, UpdateTime: now},
		DeviceGuid:     strings.TrimSpace(input.DeviceGuid),
		EventType:      input.EventType,
		Level:          level,
		Title:          input.Title,
		Message:        strings.TrimSpace(input.Message),
		Status:         int(domains.StatusEnabled),
	}
	db := s.DB()
	if db == nil {
		return
	}
	if err := db.Create(&event).Error; err == nil && ServiceGroupApp.EventHub != nil {
		ServiceGroupApp.EventHub.Publish(event)
	}
}

func (s EventService) setStatus(guid string, status int) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return s.DB().Model(&domains.Event{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      status,
		"update_time": domains.NowMilli(),
	}).Error
}

func withoutEventCenterNoise(db *gorm.DB) *gorm.DB {
	return db.Where(
		"NOT ((event_type = ? AND LOWER(title) LIKE ?) OR (event_type = ? AND LOWER(title) LIKE ?))",
		httpGatewayOpenTCPFailedEventType,
		"%"+httpEventTitleMarker+"%",
		httpGatewaySessionRejectedEventType,
		"%"+httpEventTitleMarker+"%",
	)
}

func shouldIgnoreEventInput(input EventInput) bool {
	return input.EventType == "" ||
		input.EventType == ignoredServiceLogEventType ||
		input.Title == "" ||
		isEventCenterNoise(input)
}

func isEventCenterNoise(input EventInput) bool {
	return (input.EventType == httpGatewayOpenTCPFailedEventType && isHTTPEventTitle(input.Title)) ||
		(input.EventType == httpGatewaySessionRejectedEventType && isHTTPEventTitle(input.Title))
}

func isHTTPEventTitle(title string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(title)), httpEventTitleMarker)
}
