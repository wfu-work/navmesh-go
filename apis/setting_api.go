package apis

import (
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type SettingApi struct{}

func (s SettingApi) List(c *gin.Context) {
	items, err := settingService.List()
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(items, c)
}

func (s SettingApi) Save(c *gin.Context) {
	var req struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := settingService.Save(c.Param("key"), req.Value)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "setting", ResourceID: c.Param("key"), SourceIP: c.ClientIP()})
	response.Ok(item, c)
}
