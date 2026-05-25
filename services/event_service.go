package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/google/uuid"
	"github.com/wfu-work/nav-common-go-lib/global"
)

type EventService struct{}

type EventInput struct {
	DeviceGuid string
	EventType  string
	Level      string
	Title      string
	Message    string
}

func (s EventService) List(params map[string]string) ([]domains.Event, int64, error) {
	db := global.NAV_DB.Model(&domains.Event{})
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	if eventType := strings.TrimSpace(params["eventType"]); eventType != "" {
		db = db.Where("event_type = ?", eventType)
	}
	if level := strings.TrimSpace(params["level"]); level != "" {
		db = db.Where("level = ?", level)
	}
	if status := utils.Str2Int(params["status"]); status > 0 {
		db = db.Where("status = ?", status)
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
	input.EventType = strings.TrimSpace(input.EventType)
	input.Title = strings.TrimSpace(input.Title)
	if input.EventType == "" || input.Title == "" {
		return
	}
	level := strings.TrimSpace(input.Level)
	if level == "" {
		level = "info"
	}
	now := domains.NowMilli()
	_ = global.NAV_DB.Create(&domains.Event{
		Guid:       uuid.NewString(),
		DeviceGuid: strings.TrimSpace(input.DeviceGuid),
		EventType:  input.EventType,
		Level:      level,
		Title:      input.Title,
		Message:    strings.TrimSpace(input.Message),
		Status:     int(domains.StatusEnabled),
		CreateTime: now,
		UpdateTime: now,
	}).Error
}

func (s EventService) setStatus(guid string, status int) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return global.NAV_DB.Model(&domains.Event{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      status,
		"update_time": domains.NowMilli(),
	}).Error
}
