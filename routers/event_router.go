package routers

import "github.com/gin-gonic/gin"

type EventRouter struct{}

func (e *EventRouter) InitEventRouter(router *gin.RouterGroup) {
	group := router.Group("events")
	{
		group.GET("list", eventApi.List)
		group.POST(":guid/ack", eventApi.Ack)
		group.POST(":guid/close", eventApi.Close)
	}
}
