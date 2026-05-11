package routers

import "github.com/gin-gonic/gin"

type AccessPolicyRouter struct{}

func (a *AccessPolicyRouter) InitAccessPolicyRouter(router *gin.RouterGroup) {
	group := router.Group("access-policies")
	{
		group.GET("list", accessPolicyApi.List)
		group.POST("", accessPolicyApi.Save)
		group.DELETE(":guid", accessPolicyApi.Disable)
	}
}
