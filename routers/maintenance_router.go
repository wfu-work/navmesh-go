package routers

import "github.com/gin-gonic/gin"

type MaintenanceRouter struct{}

func (m *MaintenanceRouter) InitMaintenanceRouter(router *gin.RouterGroup) {
	group := router.Group("maintenance")
	{
		group.POST("retention-cleanup", maintenanceApi.CleanupRetention)
	}
}
