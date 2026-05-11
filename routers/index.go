package routers

import "navmesh-go/apis"

var RouterGroupApp = new(RouterGroup)

type RouterGroup struct {
	AccessPolicyRouter
	AuditRouter
	AuthRouter
	DeviceRouter
	MappingRouter
	MaintenanceRouter
	SessionRouter
	SettingRouter
	SSHRouter
	TunnelRouter
}

var (
	accessPolicyApi = apis.ApiGroupApp.AccessPolicyApi
	auditApi        = apis.ApiGroupApp.AuditApi
	authApi         = apis.ApiGroupApp.AuthApi
	deviceApi       = apis.ApiGroupApp.DeviceApi
	mappingApi      = apis.ApiGroupApp.MappingApi
	maintenanceApi  = apis.ApiGroupApp.MaintenanceApi
	sessionApi      = apis.ApiGroupApp.SessionApi
	settingApi      = apis.ApiGroupApp.SettingApi
	sshApi          = apis.ApiGroupApp.SSHApi
	tunnelApi       = apis.ApiGroupApp.TunnelApi
)
