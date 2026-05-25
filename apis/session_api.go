package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type SessionApi struct{}

func (s SessionApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := sessionService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (s SessionApi) Stats(c *gin.Context) {
	params := utils.QueryParams(c)
	stats, err := sessionService.Stats(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(stats, c)
}

func (s SessionApi) Close(c *gin.Context) {
	guid := c.Param("guid")
	if err := sessionService.Close(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "close", Resource: "tunnel_session", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}
