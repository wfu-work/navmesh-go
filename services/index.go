package services

import (
	"strconv"
	"strings"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/utils"
	"gorm.io/gorm"
)

var ServiceGroupApp = newServiceGroup()

const DefaultMaxPageSize = 100

type PageOptions struct {
	Page int
	Size int
}

func ApplyBeforeCursor(db *gorm.DB, params map[string]string, timeColumn string) (*gorm.DB, bool) {
	if db == nil {
		return db, false
	}
	beforeTime, err := strconv.ParseInt(strings.TrimSpace(params["beforeTime"]), 10, 64)
	if err != nil || beforeTime <= 0 {
		return db, false
	}
	beforeID, _ := strconv.ParseUint(strings.TrimSpace(params["beforeId"]), 10, 64)
	if beforeID > 0 {
		return db.Where("("+timeColumn+" < ?) OR ("+timeColumn+" = ? AND id < ?)", beforeTime, beforeTime, beforeID), true
	}
	return db.Where(timeColumn+" < ?", beforeTime), true
}

func ParsePage(params map[string]string, maxSize int) PageOptions {
	page := utils.Str2Int(params["page"])
	size := utils.Str2Int(params["size"])
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if maxSize <= 0 {
		maxSize = DefaultMaxPageSize
	}
	if size > maxSize {
		size = maxSize
	}
	return PageOptions{Page: page, Size: size}
}

type ServiceGroup struct {
	AccessPolicyService
	AuditService
	ReleaseService
	CustomDomainService
	DeviceService
	DeviceTokenService
	DeviceTrafficService
	DeviceUpgradeService
	EventService
	EmailService
	GroupService
	MappingService
	MaintenanceService
	MessageService
	SessionService
	SettingService
	SSHService
	TCPMappingService
	EventHub       *EventHub
	EventWebSocket *EventWebSocket
}

func newServiceGroup() *ServiceGroup {
	eventHub := NewEventHub()
	return &ServiceGroup{
		EventHub:       eventHub,
		EventWebSocket: NewEventWebSocket(eventHub),
	}
}

func PageResult(items any, total int64, params map[string]string) commonDomains.PageResult {
	page := ParsePage(params, DefaultMaxPageSize)
	return commonDomains.PageResult{
		Data:  items,
		Total: total,
		Page:  page.Page,
		Size:  page.Size,
	}
}
