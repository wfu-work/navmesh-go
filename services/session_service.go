package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/gorm"
)

type SessionService struct{}

type SessionStats struct {
	Total            int64 `json:"total"`
	Active           int64 `json:"active"`
	Closed           int64 `json:"closed"`
	SSH              int64 `json:"ssh"`
	HTTP             int64 `json:"http"`
	BytesIn          int64 `json:"bytesIn"`
	BytesOut         int64 `json:"bytesOut"`
	ForceClosed      int64 `json:"forceClosed"`
	RuntimeActive    int   `json:"runtimeActive"`
	MaxConcurrent    int   `json:"maxConcurrent"`
	MaxDevice        int   `json:"maxDevice"`
	RateLimitPerMin  int   `json:"rateLimitPerMin"`
	IdleTimeoutMilli int64 `json:"idleTimeoutMilli"`
}

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

func (s SessionService) Close(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	closedRuntime := DefaultSessionRegistry.CloseSession(guid)
	now := domains.NowMilli()
	reason := "closed_by_admin"
	if !closedRuntime {
		reason = "closed_by_admin_offline"
	}
	return global.NAV_DB.Model(&domains.TunnelSession{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":            int(domains.StatusDisabled),
		"force_closed":      true,
		"disconnect_reason": reason,
		"end_time":          now,
		"update_time":       now,
	}).Error
}

func (s SessionService) Stats(params map[string]string) (*SessionStats, error) {
	db := global.NAV_DB.Model(&domains.TunnelSession{})
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	stats := &SessionStats{}
	if err := db.Count(&stats.Total).Error; err != nil {
		return nil, err
	}
	_ = db.Session(&gorm.Session{}).Where("status = ?", int(domains.StatusEnabled)).Count(&stats.Active).Error
	_ = db.Session(&gorm.Session{}).Where("status = ?", int(domains.StatusDisabled)).Count(&stats.Closed).Error
	_ = db.Session(&gorm.Session{}).Where("session_type = ?", "ssh").Count(&stats.SSH).Error
	_ = db.Session(&gorm.Session{}).Where("session_type = ?", "http").Count(&stats.HTTP).Error
	_ = db.Session(&gorm.Session{}).Where("force_closed = ?", true).Count(&stats.ForceClosed).Error
	var sums struct {
		BytesIn  int64
		BytesOut int64
	}
	_ = db.Session(&gorm.Session{}).Select("COALESCE(SUM(bytes_in), 0) AS bytes_in, COALESCE(SUM(bytes_out), 0) AS bytes_out").Scan(&sums).Error
	stats.BytesIn = sums.BytesIn
	stats.BytesOut = sums.BytesOut
	runtimeStats := DefaultSessionRegistry.Stats()
	settings := DefaultRuntimePolicy.Snapshot()
	stats.RuntimeActive = runtimeStats.Active
	stats.MaxConcurrent = settings.MaxConcurrentSessions
	stats.MaxDevice = settings.MaxDeviceSessions
	stats.RateLimitPerMin = settings.RateLimitPerMinute
	stats.IdleTimeoutMilli = settings.SessionIdleTimeout.Milliseconds()
	return stats, nil
}
