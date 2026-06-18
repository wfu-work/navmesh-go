package services

import (
	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/utils"
)

var ServiceGroupApp = newServiceGroup()

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
	GroupService
	MappingService
	MaintenanceService
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
	page := utils.Str2Int(params["page"])
	size := utils.Str2Int(params["size"])
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	return commonDomains.PageResult{
		Data:  items,
		Total: total,
		Page:  page,
		Size:  size,
	}
}
