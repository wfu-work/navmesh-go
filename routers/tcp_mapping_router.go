package routers

import "github.com/gin-gonic/gin"

type TCPMappingRouter struct{}

func (m *TCPMappingRouter) InitTCPMappingRouter(router *gin.RouterGroup) {
	group := router.Group("tcp-mappings")
	{
		group.GET("list", tcpMappingApi.List)
		group.GET("port-range", tcpMappingApi.PortRange)
		group.POST("", tcpMappingApi.Save)
		group.DELETE(":guid", tcpMappingApi.Disable)
	}
}
