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
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (e EventApi) Ack(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, eventService.Ack(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "ack", Resource: "event", ResourceID: guid})
	response.Ok(true, c)
}

func (e EventApi) AckAll(c *gin.Context) {
	affected, err := eventService.AckAll()
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "ack_all", Resource: "event", ResourceID: "all"})
	response.Ok(gin.H{"affected": affected}, c)
}

func (e EventApi) Close(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, eventService.Close(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "close", Resource: "event", ResourceID: guid})
	response.Ok(true, c)
}

func (e EventApi) Stream(c *gin.Context) {
	services.ServiceGroupApp.EventWebSocket.ServeHTTP(c.Writer, c.Request)
}
