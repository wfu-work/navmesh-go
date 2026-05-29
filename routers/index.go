package routers

import (
	"navmesh-go/apis"

	"github.com/gin-gonic/gin"
)

var RouterGroupApp = new(RouterGroup)

type RouterGroup struct {
	AccessPolicyRouter
	AuditRouter
	ClientReleaseRouter
	DeviceRouter
	EventRouter
	GroupRouter
	MappingRouter
	MaintenanceRouter
	SessionRouter
	SettingRouter
	SSHRouter
	TunnelRouter
}

var (
	accessPolicyApi  = apis.ApiGroupApp.AccessPolicyApi
	auditApi         = apis.ApiGroupApp.AuditApi
	clientReleaseApi = apis.ApiGroupApp.ClientReleaseApi
	deviceApi        = apis.ApiGroupApp.DeviceApi
	eventApi         = apis.ApiGroupApp.EventApi
	groupApi         = apis.ApiGroupApp.GroupApi
	mappingApi       = apis.ApiGroupApp.MappingApi
	maintenanceApi   = apis.ApiGroupApp.MaintenanceApi
	sessionApi       = apis.ApiGroupApp.SessionApi
	settingApi       = apis.ApiGroupApp.SettingApi
	sshApi           = apis.ApiGroupApp.SSHApi
	tunnelApi        = apis.ApiGroupApp.TunnelApi
)

func (r *RouterGroup) InitRouters(publicGroup *gin.RouterGroup, privateGroup *gin.RouterGroup) {
	r.InitAccessPolicyRouter(privateGroup)
	r.InitAuditRouter(privateGroup)
	r.InitClientReleaseRouter(publicGroup, privateGroup)
	r.InitDeviceRouter(publicGroup, privateGroup)
	r.InitEventRouter(privateGroup)
	r.InitGroupRouter(privateGroup)
	r.InitMappingRouter(privateGroup)
	r.InitMaintenanceRouter(privateGroup)
	r.InitSessionRouter(privateGroup)
	r.InitSettingRouter(privateGroup)
	r.InitSSHRouter(privateGroup)
	r.InitTunnelRouter(privateGroup)
}
