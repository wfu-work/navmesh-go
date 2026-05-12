package routers

import "navmesh-go/apis"

var RouterGroupApp = new(RouterGroup)

type RouterGroup struct {
	AccessPolicyRouter
	AuditRouter
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
	deviceApi       = apis.ApiGroupApp.DeviceApi
	mappingApi      = apis.ApiGroupApp.MappingApi
	maintenanceApi  = apis.ApiGroupApp.MaintenanceApi
	sessionApi      = apis.ApiGroupApp.SessionApi
	settingApi      = apis.ApiGroupApp.SettingApi
	sshApi          = apis.ApiGroupApp.SSHApi
	tunnelApi       = apis.ApiGroupApp.TunnelApi
)
