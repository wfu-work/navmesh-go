package services

import (
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/wfu-work/nav-common-go-lib/global"
)

type AuditService struct{}

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
	_ = global.NAV_DB.Create(&row).Error
}

func (s AuditService) List(params map[string]string) ([]domains.AuditLog, int64, error) {
	db := global.NAV_DB.Model(&domains.AuditLog{})
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
	var items []domains.AuditLog
	err := db.Order("create_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}
