package routers

import "github.com/gin-gonic/gin"

type AuditRouter struct{}

func (a *AuditRouter) InitAuditRouter(router *gin.RouterGroup) {
	group := router.Group("audit-logs")
	{
		group.GET("list", auditApi.List)
	}
}
