package routers

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/middlewares"
)

type EventRouter struct{}

func (e *EventRouter) InitEventRouter(publicGroup *gin.RouterGroup, privateGroup *gin.RouterGroup) {
	streamGroup := publicGroup.Group("events")
	streamGroup.Use(eventWebsocketToken()).Use(middlewares.JWTAuth()).Use(middlewares.CasbinHandler())
	{
		streamGroup.GET("ws", eventApi.Stream)
	}

	group := privateGroup.Group("events")
	{
		group.GET("list", eventApi.List)
		group.POST(":guid/ack", eventApi.Ack)
		group.POST(":guid/close", eventApi.Close)
	}
}

func eventWebsocketToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.Query("token"))
		if token != "" && strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			c.Request.Header.Set("Authorization", "Bearer "+token)
		}
		c.Next()
	}
}
