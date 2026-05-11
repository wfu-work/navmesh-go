package routers

import "github.com/gin-gonic/gin"

type SessionRouter struct{}

func (s *SessionRouter) InitSessionRouter(router *gin.RouterGroup) {
	group := router.Group("tunnel-sessions")
	{
		group.GET("list", sessionApi.List)
	}
}
