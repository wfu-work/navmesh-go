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
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (s SessionApi) Stats(c *gin.Context) {
	params := utils.QueryParams(c)
	stats, err := sessionService.Stats(params)
	if fail(c, err) {
		return
	}
	response.Ok(stats, c)
}

func (s SessionApi) Close(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, sessionService.Close(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "close", Resource: "tunnel_session", ResourceID: guid})
	response.Ok(true, c)
}
