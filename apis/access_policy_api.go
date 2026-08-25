package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type AccessPolicyApi struct{}

func (a AccessPolicyApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := accessPolicyService.List(params)
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a AccessPolicyApi) Save(c *gin.Context) {
	req, ok := bindJSON[services.SaveAccessPolicyRequest](c)
	if !ok {
		return
	}
	item, err := accessPolicyService.Save(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "access_policy", ResourceID: item.Guid, Message: item.Name})
	response.Ok(item, c)
}

func (a AccessPolicyApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, accessPolicyService.Disable(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "access_policy", ResourceID: guid})
	response.Ok(true, c)
}
