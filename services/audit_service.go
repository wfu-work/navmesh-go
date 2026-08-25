package services

import (
	"strings"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/gorm"
)

type AuditService struct {
	db *gorm.DB
}

func (s AuditService) WithDB(db *gorm.DB) AuditService {
	s.db = db
	return s
}

func (s AuditService) DB() *gorm.DB {
	if s.db != nil {
		return s.db
	}
	return global.NAV_DB
}

type AuditInput struct {
	Actor      string
	Action     string
	Resource   string
	ResourceID string
	Message    string
	SourceIP   string
}

func (s AuditService) Record(input AuditInput) {
	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		actor = "system"
	}
	row := domains.AuditLog{
		Actor:      actor,
		Action:     strings.TrimSpace(input.Action),
		Resource:   strings.TrimSpace(input.Resource),
		ResourceID: strings.TrimSpace(input.ResourceID),
		Message:    strings.TrimSpace(input.Message),
		SourceIP:   strings.TrimSpace(input.SourceIP),
		CreateTime: domains.NowMilli(),
	}
	if db := s.DB(); db != nil {
		_ = db.Create(&row).Error
	}
}

func (s AuditService) List(params map[string]string) ([]domains.AuditLog, int64, error) {
	db := s.DB()
	if db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	db = db.Model(&domains.AuditLog{})
	if actor := strings.TrimSpace(params["actor"]); actor != "" {
		db = db.Where("actor = ?", actor)
	}
	if action := strings.TrimSpace(params["action"]); action != "" {
		db = db.Where("action = ?", action)
	}
	if resource := strings.TrimSpace(params["resource"]); resource != "" {
		db = db.Where("resource = ?", resource)
	}
	if resourceID := strings.TrimSpace(params["resourceId"]); resourceID != "" {
		db = db.Where("resource_id = ?", resourceID)
	}
	return queryPageCursor[domains.AuditLog](db, params, DefaultMaxPageSize, "create_time", "create_time DESC, id DESC")
}
