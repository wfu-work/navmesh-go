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
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a AccessPolicyApi) Save(c *gin.Context) {
	var req services.SaveAccessPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := accessPolicyService.Save(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "access_policy", ResourceID: item.Guid, Message: item.Name, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (a AccessPolicyApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if err := accessPolicyService.Disable(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "access_policy", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}
