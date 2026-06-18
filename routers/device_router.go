package routers

import "github.com/gin-gonic/gin"

type DeviceRouter struct{}

func (d *DeviceRouter) InitDeviceRouter(publicGroup *gin.RouterGroup, privateGroup *gin.RouterGroup) {
	devicePublic := publicGroup.Group("device")
	{
		devicePublic.POST("register", deviceApi.Register)
		devicePublic.POST("heartbeat", deviceApi.Heartbeat)
		devicePublic.POST("upgrade/report", deviceApi.ReportUpgrade)
	}

	devices := privateGroup.Group("devices")
	{
		devices.GET("list", deviceApi.List)
		devices.GET("stats", deviceApi.Stats)
		devices.GET("traffic/daily", deviceApi.TrafficDaily)
		devices.GET("types/defaults", deviceApi.TypeDefaults)
		devices.GET(":guid/service-logs/stream", deviceApi.StreamServiceLogs)
		devices.GET(":guid/traffic/daily", deviceApi.TrafficDaily)
		devices.GET(":guid", deviceApi.Get)
		devices.PUT(":guid", deviceApi.Update)
		devices.POST(":guid/enable", deviceApi.Enable)
		devices.POST(":guid/disable", deviceApi.Disable)
		devices.POST(":guid/vpn/restart", deviceApi.RestartVPN)
		devices.DELETE(":guid", deviceApi.Delete)
		devices.POST(":guid/tokens", deviceApi.CreateToken)
		devices.POST(":guid/tokens/:tokenGuid/rotate", deviceApi.RotateToken)
		devices.POST(":guid/tokens/:tokenGuid/enable", deviceApi.EnableToken)
		devices.DELETE(":guid/tokens/:tokenGuid", deviceApi.DisableToken)
		devices.GET(":guid/upgrades", deviceApi.ListUpgradeTasks)
		devices.POST(":guid/upgrades", deviceApi.CreateUpgradeTask)
	}
}
