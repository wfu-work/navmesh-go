package routers

import "github.com/gin-gonic/gin"

type DeviceRouter struct{}

func (d *DeviceRouter) InitDeviceRouter(publicGroup *gin.RouterGroup, privateGroup *gin.RouterGroup) {
	devicePublic := publicGroup.Group("device")
	{
		devicePublic.POST("register", deviceApi.Register)
		devicePublic.POST("heartbeat", deviceApi.Heartbeat)
	}

	devices := privateGroup.Group("devices")
	{
		devices.GET("list", deviceApi.List)
		devices.GET("types/defaults", deviceApi.TypeDefaults)
		devices.GET(":guid", deviceApi.Get)
		devices.DELETE(":guid", deviceApi.Delete)
		devices.POST(":guid/tokens", deviceApi.CreateToken)
		devices.POST(":guid/tokens/:tokenGuid/rotate", deviceApi.RotateToken)
		devices.POST(":guid/tokens/:tokenGuid/enable", deviceApi.EnableToken)
		devices.DELETE(":guid/tokens/:tokenGuid", deviceApi.DisableToken)
	}
}
