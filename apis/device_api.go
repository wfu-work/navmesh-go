package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type DeviceApi struct{}

func (d DeviceApi) Register(c *gin.Context) {
	var req services.RegisterDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	result, err := deviceService.Register(req, c.ClientIP())
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(result, c)
}

func (d DeviceApi) Heartbeat(c *gin.Context) {
	var req services.HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if req.Token == "" {
		req.Token = utils.BearerToken(c)
	}
	device, err := deviceService.Heartbeat(req, c.ClientIP())
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(device, c)
}

func (d DeviceApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := deviceService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (d DeviceApi) Get(c *gin.Context) {
	device, tokens, err := deviceService.Get(c.Param("guid"))
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(gin.H{"device": device, "tokens": tokens}, c)
}

func (d DeviceApi) Delete(c *gin.Context) {
	guid := c.Param("guid")
	if err := deviceService.Delete(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "device", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (d DeviceApi) DisableToken(c *gin.Context) {
	guid := c.Param("guid")
	tokenGuid := c.Param("tokenGuid")
	if err := deviceService.DisableToken(guid, tokenGuid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "device_token", ResourceID: tokenGuid, Message: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (d DeviceApi) TypeDefaults(c *gin.Context) {
	response.Ok(deviceService.TypeDefaults(), c)
}
