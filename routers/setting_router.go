package routers

import "github.com/gin-gonic/gin"

type SettingRouter struct{}

func (s *SettingRouter) InitSettingRouter(router *gin.RouterGroup) {
	group := router.Group("settings")
	{
		group.GET("list", settingApi.List)
		group.PUT(":key", settingApi.Save)
	}
}
