package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type AuditApi struct{}

func (a AuditApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := auditService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func actorName(c *gin.Context) string {
	if username := c.GetString("userName"); username != "" {
		return username
	}
	return "admin"
}
