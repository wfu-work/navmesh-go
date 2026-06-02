package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type GroupApi struct{}

func (g GroupApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := groupService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (g GroupApi) Save(c *gin.Context) {
	var req services.SaveDeviceGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := groupService.Save(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "device_group", ResourceID: item.Guid, Message: item.Name, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (g GroupApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if err := groupService.Disable(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "device_group", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (g GroupApi) Delete(c *gin.Context) {
	guid := c.Param("guid")
	if err := groupService.Delete(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "delete", Resource: "device_group", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (g GroupApi) AssignDevice(c *gin.Context) {
	var req services.AssignDeviceGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	deviceGuid := c.Param("guid")
	if err := groupService.AssignDevice(deviceGuid, req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "assign", Resource: "device_group", ResourceID: req.GroupGuid, Message: deviceGuid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}
