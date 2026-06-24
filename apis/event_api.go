package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type EventApi struct{}

func (e EventApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := eventService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (e EventApi) Ack(c *gin.Context) {
	guid := c.Param("guid")
	if err := eventService.Ack(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "ack", Resource: "event", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (e EventApi) AckAll(c *gin.Context) {
	affected, err := eventService.AckAll()
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "ack_all", Resource: "event", ResourceID: "all", SourceIP: c.ClientIP()})
	response.Ok(gin.H{"affected": affected}, c)
}

func (e EventApi) Close(c *gin.Context) {
	guid := c.Param("guid")
	if err := eventService.Close(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "close", Resource: "event", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (e EventApi) Stream(c *gin.Context) {
	services.ServiceGroupApp.EventWebSocket.ServeHTTP(c.Writer, c.Request)
}
