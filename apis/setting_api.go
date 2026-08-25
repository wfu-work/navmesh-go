package apis

import (
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type SettingApi struct{}

func (s SettingApi) List(c *gin.Context) {
	items, err := settingService.List()
	if fail(c, err) {
		return
	}
	response.Ok(items, c)
}

func (s SettingApi) Save(c *gin.Context) {
	type saveSettingRequest struct {
		Value string `json:"value"`
	}
	req, ok := bindJSON[saveSettingRequest](c)
	if !ok {
		return
	}
	item, err := settingService.Save(c.Param("key"), req.Value)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "setting", ResourceID: c.Param("key")})
	response.Ok(item, c)
}
