package routers

import "github.com/gin-gonic/gin"

type TunnelRouter struct{}

func (t *TunnelRouter) InitTunnelRouter(router *gin.RouterGroup) {
	group := router.Group("tunnel")
	{
		group.GET("connections", tunnelApi.Connections)
	}
}
