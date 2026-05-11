package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type MappingApi struct{}

func (m MappingApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := mappingService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m MappingApi) Save(c *gin.Context) {
	var req services.SavePortMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := mappingService.Save(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "port_mapping", ResourceID: item.Guid, Message: item.PublicHost, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (m MappingApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if err := mappingService.Disable(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "port_mapping", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (m MappingApi) AccessLogs(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := mappingService.AccessLogs(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}
