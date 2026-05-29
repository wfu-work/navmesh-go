package apis

import "navmesh-go/services"

var ApiGroupApp = new(ApiGroup)

type ApiGroup struct {
	AccessPolicyApi
	AuditApi
	ClientReleaseApi
	DeviceApi
	EventApi
	GroupApi
	MappingApi
	MaintenanceApi
	SessionApi
	SettingApi
	SSHApi
	TunnelApi
}

var (
	accessPolicyService  = services.ServiceGroupApp.AccessPolicyService
	auditService         = services.ServiceGroupApp.AuditService
	clientReleaseService = services.ServiceGroupApp.ClientReleaseService
	customDomainService  = services.ServiceGroupApp.CustomDomainService
	deviceService        = services.ServiceGroupApp.DeviceService
	deviceTokenService   = services.ServiceGroupApp.DeviceTokenService
	deviceUpgradeService = services.ServiceGroupApp.DeviceUpgradeService
	eventService         = services.ServiceGroupApp.EventService
	groupService         = services.ServiceGroupApp.GroupService
	mappingService       = services.ServiceGroupApp.MappingService
	maintenanceService   = services.ServiceGroupApp.MaintenanceService
	sessionService       = services.ServiceGroupApp.SessionService
	settingService       = services.ServiceGroupApp.SettingService
	sshService           = services.ServiceGroupApp.SSHService
)
