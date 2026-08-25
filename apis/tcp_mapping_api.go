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
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m TCPMappingApi) Save(c *gin.Context) {
	req, ok := bindJSON[services.SaveTCPMappingRequest](c)
	if !ok {
		return
	}
	item, err := tcpMappingService.Save(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "tcp_mapping", ResourceID: item.Guid, Message: fmt.Sprintf("%s:%d", item.PublicHost, item.PublicPort)})
	response.Ok(item, c)
}

func (m TCPMappingApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, tcpMappingService.Disable(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "tcp_mapping", ResourceID: guid})
	response.Ok(true, c)
}

func (m TCPMappingApi) PortRange(c *gin.Context) {
	response.Ok(services.TCPMappingPortRange(), c)
}
