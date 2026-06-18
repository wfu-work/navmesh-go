package apis

import "navmesh-go/services"

var ApiGroupApp = new(ApiGroup)

type ApiGroup struct {
	AccessPolicyApi
	AuditApi
	ReleaseApi
	DeviceApi
	EventApi
	GroupApi
	MappingApi
	MaintenanceApi
	SessionApi
	SettingApi
	SSHApi
	TCPMappingApi
	TunnelApi
}

var (
	accessPolicyService  = services.ServiceGroupApp.AccessPolicyService
	auditService         = services.ServiceGroupApp.AuditService
	releaseService       = services.ServiceGroupApp.ReleaseService
	customDomainService  = services.ServiceGroupApp.CustomDomainService
	deviceService        = services.ServiceGroupApp.DeviceService
	deviceTokenService   = services.ServiceGroupApp.DeviceTokenService
	deviceTrafficService = services.ServiceGroupApp.DeviceTrafficService
	deviceUpgradeService = services.ServiceGroupApp.DeviceUpgradeService
	eventService         = services.ServiceGroupApp.EventService
	groupService         = services.ServiceGroupApp.GroupService
	mappingService       = services.ServiceGroupApp.MappingService
	maintenanceService   = services.ServiceGroupApp.MaintenanceService
	sessionService       = services.ServiceGroupApp.SessionService
	settingService       = services.ServiceGroupApp.SettingService
	sshService           = services.ServiceGroupApp.SSHService
	tcpMappingService    = services.ServiceGroupApp.TCPMappingService
)
