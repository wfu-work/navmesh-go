package apis

import "navmesh-go/services"

var ApiGroupApp = new(ApiGroup)

type ApiGroup struct {
	AccessPolicyApi
	AuditApi
	AuthApi
	DeviceApi
	MappingApi
	MaintenanceApi
	SessionApi
	SettingApi
	SSHApi
	TunnelApi
}

var (
	accessPolicyService = services.ServiceGroupApp.AccessPolicyService
	auditService        = services.ServiceGroupApp.AuditService
	authService         = services.ServiceGroupApp.AuthService
	deviceService       = services.ServiceGroupApp.DeviceService
	mappingService      = services.ServiceGroupApp.MappingService
	maintenanceService  = services.ServiceGroupApp.MaintenanceService
	sessionService      = services.ServiceGroupApp.SessionService
	settingService      = services.ServiceGroupApp.SettingService
	sshService          = services.ServiceGroupApp.SSHService
)
