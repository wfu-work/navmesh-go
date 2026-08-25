package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/gorm"
)

type SessionService struct {
	db *gorm.DB
}

func (s SessionService) WithDB(db *gorm.DB) SessionService {
	s.db = db
	return s
}

func (s SessionService) DB() *gorm.DB {
	if s.db != nil {
		return s.db
	}
	return global.NAV_DB
}

type SessionStats struct {
	Total            int64 `json:"total"`
	Active           int64 `json:"active"`
	Closed           int64 `json:"closed"`
	SSH              int64 `json:"ssh"`
	HTTP             int64 `json:"http"`
	TCP              int64 `json:"tcp"`
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
	db := s.DB()
	if db == nil {
		return nil, 0, errors.New("database is not initialized")
	}
	db = db.Model(&domains.TunnelSession{})
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
	return queryPageCursor[domains.TunnelSession](db, params, DefaultMaxPageSize, "start_time", "start_time DESC, id DESC")
}

func (s SessionService) Close(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	db := s.DB()
	if db == nil {
		return errors.New("database is not initialized")
	}
	closedRuntime := DefaultSessionRegistry.CloseSession(guid)
	now := domains.NowMilli()
	reason := "closed_by_admin"
	if !closedRuntime {
		reason = "closed_by_admin_offline"
	}
	return db.Model(&domains.TunnelSession{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":            int(domains.StatusDisabled),
		"force_closed":      true,
		"disconnect_reason": reason,
		"end_time":          now,
		"update_time":       now,
	}).Error
}

func (s SessionService) Stats(params map[string]string) (*SessionStats, error) {
	db := s.DB()
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	db = db.Model(&domains.TunnelSession{})
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	stats := &SessionStats{}
	if err := db.Select(`
		COUNT(*) AS total,
		COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS active,
		COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS closed,
		COALESCE(SUM(CASE WHEN session_type = 'ssh' THEN 1 ELSE 0 END), 0) AS ssh,
		COALESCE(SUM(CASE WHEN session_type = 'http' THEN 1 ELSE 0 END), 0) AS http,
		COALESCE(SUM(CASE WHEN session_type = 'tcp' THEN 1 ELSE 0 END), 0) AS tcp,
		COALESCE(SUM(bytes_in), 0) AS bytes_in,
		COALESCE(SUM(bytes_out), 0) AS bytes_out,
		COALESCE(SUM(CASE WHEN force_closed = ? THEN 1 ELSE 0 END), 0) AS force_closed
	`, int(domains.StatusEnabled), int(domains.StatusDisabled), true).Scan(stats).Error; err != nil {
		return nil, err
	}
	runtimeStats := DefaultSessionRegistry.Stats()
	settings := DefaultRuntimePolicy.Snapshot()
	stats.RuntimeActive = runtimeStats.Active
	stats.MaxConcurrent = settings.MaxConcurrentSessions
	stats.MaxDevice = settings.MaxDeviceSessions
	stats.RateLimitPerMin = settings.RateLimitPerMinute
	stats.IdleTimeoutMilli = settings.SessionIdleTimeout.Milliseconds()
	return stats, nil
}
