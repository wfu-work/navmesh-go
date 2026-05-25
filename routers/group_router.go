package routers

import "github.com/gin-gonic/gin"

type GroupRouter struct{}

func (g *GroupRouter) InitGroupRouter(router *gin.RouterGroup) {
	groups := router.Group("device-groups")
	{
		groups.GET("list", groupApi.List)
		groups.POST("", groupApi.Save)
		groups.DELETE(":guid", groupApi.Disable)
	}

	devices := router.Group("devices")
	{
		devices.PUT(":guid/group", groupApi.AssignDevice)
	}
}
