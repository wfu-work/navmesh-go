package routers

import "github.com/gin-gonic/gin"

type MappingRouter struct{}

func (m *MappingRouter) InitMappingRouter(router *gin.RouterGroup) {
	group := router.Group("port-mappings")
	{
		group.GET("list", mappingApi.List)
		group.POST("", mappingApi.Save)
		group.DELETE(":guid", mappingApi.Disable)
	}

	logs := router.Group("http-access-logs")
	{
		logs.GET("list", mappingApi.AccessLogs)
	}
}
