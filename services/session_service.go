package services

import (
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/wfu-work/nav-common-go-lib/global"
)

type SessionService struct{}

func (s SessionService) List(params map[string]string) ([]domains.TunnelSession, int64, error) {
	db := global.NAV_DB.Model(&domains.TunnelSession{})
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	if sessionType := strings.TrimSpace(params["sessionType"]); sessionType != "" {
		db = db.Where("session_type = ?", sessionType)
	}
	if publicHost := strings.TrimSpace(params["publicHost"]); publicHost != "" {
		db = db.Where("public_host = ?", publicHost)
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
	var items []domains.TunnelSession
	err := db.Order("start_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}
