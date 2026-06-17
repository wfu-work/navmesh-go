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

const ignoredServiceLogEventType = "service_log"

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
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := utils.Str2Int(params["page"])
	size := utils.Str2Int(params["size"])
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	var items []domains.Event
	err := db.Order("create_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (s EventService) Ack(guid string) error {
	return s.setStatus(guid, int(domains.StatusDisabled))
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
	if window <= 0 {
		s.RecordAt(input, now)
		return true
	}
	input.DeviceGuid = strings.TrimSpace(input.DeviceGuid)
	input.EventType = strings.TrimSpace(input.EventType)
	input.Title = strings.TrimSpace(input.Title)
	if input.DeviceGuid == "" || input.EventType == "" || input.Title == "" {
		s.RecordAt(input, now)
		return true
	}
	if now <= 0 {
		now = domains.NowMilli()
	}
	var count int64
	if err := s.DB().Model(&domains.Event{}).
		Where("device_guid = ? AND event_type = ? AND title = ? AND create_time >= ?", input.DeviceGuid, input.EventType, input.Title, now-window.Milliseconds()).
		Count(&count).Error; err != nil || count > 0 {
		return false
	}
	s.RecordAt(input, now)
	return true
}

func (s EventService) RecordAt(input EventInput, now int64) {
	input.EventType = strings.TrimSpace(input.EventType)
	input.Title = strings.TrimSpace(input.Title)
	if input.EventType == "" || input.EventType == ignoredServiceLogEventType || input.Title == "" {
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
