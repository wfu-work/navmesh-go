package apis

import (
	"fmt"
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type TCPMappingApi struct{}

func (m TCPMappingApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := tcpMappingService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m TCPMappingApi) Save(c *gin.Context) {
	var req services.SaveTCPMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := tcpMappingService.Save(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{
		Actor:      actorName(c),
		Action:     "save",
		Resource:   "tcp_mapping",
		ResourceID: item.Guid,
		Message:    fmt.Sprintf("%s:%d", item.PublicHost, item.PublicPort),
		SourceIP:   c.ClientIP(),
	})
	response.Ok(item, c)
}

func (m TCPMappingApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if err := tcpMappingService.Disable(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "tcp_mapping", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (m TCPMappingApi) PortRange(c *gin.Context) {
	response.Ok(services.TCPMappingPortRange(), c)
}
