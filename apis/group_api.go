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
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (g GroupApi) Save(c *gin.Context) {
	req, ok := bindJSON[services.SaveDeviceGroupRequest](c)
	if !ok {
		return
	}
	item, err := groupService.Save(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "device_group", ResourceID: item.Guid, Message: item.Name})
	response.Ok(item, c)
}

func (g GroupApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, groupService.Disable(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "device_group", ResourceID: guid})
	response.Ok(true, c)
}

func (g GroupApi) Delete(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, groupService.Delete(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "delete", Resource: "device_group", ResourceID: guid})
	response.Ok(true, c)
}

func (g GroupApi) AssignDevice(c *gin.Context) {
	req, ok := bindJSON[services.AssignDeviceGroupRequest](c)
	if !ok {
		return
	}
	deviceGuid := c.Param("guid")
	if fail(c, groupService.AssignDevice(deviceGuid, req)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "assign", Resource: "device_group", ResourceID: req.GroupGuid, Message: deviceGuid})
	response.Ok(true, c)
}
