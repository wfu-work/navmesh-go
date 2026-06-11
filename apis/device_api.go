package apis

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"navmesh-go/services"
	"navmesh-go/tunnel"
	"navmesh-go/utils"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type DeviceApi struct{}

var serviceLogNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]+\.service$`)

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
	payload := map[string]any{}
	if data, err := json.Marshal(device); err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	upgradeSent := false
	if strings.ToLower(settingService.Value("client_upgrade_enabled", "true")) != "false" {
		if upgrade, err := deviceUpgradeService.PendingCommand(device.Guid); err == nil && upgrade != nil {
			payload["upgrade"] = upgrade
			upgradeSent = true
		}
	}
	if !upgradeSent {
		if command, err := deviceService.TakeVPNRestartCommand(device.Guid); err == nil && command != nil {
			payload["vpnRestart"] = command
		}
	}
	response.Ok(payload, c)
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

func (d DeviceApi) Stats(c *gin.Context) {
	params := utils.QueryParams(c)
	stats, err := deviceService.Stats(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(stats, c)
}

func (d DeviceApi) Get(c *gin.Context) {
	device, tokens, err := deviceService.Get(c.Param("guid"))
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(gin.H{"device": device, "tokens": tokens}, c)
}

func (d DeviceApi) Update(c *gin.Context) {
	var req services.UpdateDeviceProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	guid := c.Param("guid")
	device, err := deviceService.UpdateProfile(guid, req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "update", Resource: "device", ResourceID: guid, Message: req.Alias, SourceIP: c.ClientIP()})
	response.Ok(device, c)
}

func (d DeviceApi) Delete(c *gin.Context) {
	guid := c.Param("guid")
	if err := deviceService.Delete(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "delete", Resource: "device", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (d DeviceApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if err := deviceService.Disable(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "device", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (d DeviceApi) Enable(c *gin.Context) {
	guid := c.Param("guid")
	if err := deviceService.Enable(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "enable", Resource: "device", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (d DeviceApi) RestartVPN(c *gin.Context) {
	guid := c.Param("guid")
	command, err := deviceService.RequestVPNRestart(guid)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "restart", Resource: "device_vpn", ResourceID: guid, Message: "VPN 重启指令", SourceIP: c.ClientIP()})
	eventService.Record(services.EventInput{DeviceGuid: guid, EventType: "vpn_restart", Level: "info", Title: "VPN 重启指令已创建", Message: "等待客户端心跳执行"})
	response.Ok(command, c)
}

func (d DeviceApi) StreamServiceLogs(c *gin.Context) {
	guid := strings.TrimSpace(c.Param("guid"))
	serviceName, tail, err := normalizeServiceLogQuery(c.Query("service"), c.Query("tail"))
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if _, _, err := deviceService.Get(guid); err != nil {
		c.String(http.StatusNotFound, err.Error())
		return
	}
	openCtx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	stream, err := tunnel.DefaultManager.OpenServiceLogStream(openCtx, guid, serviceName, tail)
	cancel()
	if err != nil {
		c.String(http.StatusBadGateway, err.Error())
		return
	}
	defer stream.Close()

	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "view", Resource: "device_service_log", ResourceID: guid, Message: serviceName, SourceIP: c.ClientIP()})

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	done := make(chan struct{})
	go func() {
		select {
		case <-c.Request.Context().Done():
			_ = stream.Close()
		case <-done:
		}
	}()
	defer close(done)

	buf := make([]byte, 32*1024)
	for {
		n, readErr := stream.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				return
			}
			c.Writer.Flush()
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return
			}
			return
		}
	}
}

func normalizeServiceLogQuery(serviceName string, tailText string) (string, int, error) {
	serviceName = strings.TrimSpace(serviceName)
	if !serviceLogNamePattern.MatchString(serviceName) {
		return "", 0, errors.New("invalid service name")
	}
	tail := utils.Str2Int(tailText)
	if tail <= 0 {
		tail = 200
	}
	if tail > 2000 {
		tail = 2000
	}
	return serviceName, tail, nil
}

func (d DeviceApi) DisableToken(c *gin.Context) {
	guid := c.Param("guid")
	tokenGuid := c.Param("tokenGuid")
	if err := deviceTokenService.Disable(guid, tokenGuid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "device_token", ResourceID: tokenGuid, Message: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (d DeviceApi) CreateToken(c *gin.Context) {
	var req services.CreateDeviceTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	guid := c.Param("guid")
	result, err := deviceTokenService.CreateToken(guid, req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "create", Resource: "device_token", ResourceID: result.Item.Guid, Message: guid, SourceIP: c.ClientIP()})
	response.Ok(result, c)
}

func (d DeviceApi) RotateToken(c *gin.Context) {
	guid := c.Param("guid")
	tokenGuid := c.Param("tokenGuid")
	result, err := deviceTokenService.Rotate(guid, tokenGuid)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "rotate", Resource: "device_token", ResourceID: tokenGuid, Message: result.Item.Guid, SourceIP: c.ClientIP()})
	response.Ok(result, c)
}

func (d DeviceApi) EnableToken(c *gin.Context) {
	guid := c.Param("guid")
	tokenGuid := c.Param("tokenGuid")
	if err := deviceTokenService.Enable(guid, tokenGuid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "enable", Resource: "device_token", ResourceID: tokenGuid, Message: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (d DeviceApi) TypeDefaults(c *gin.Context) {
	response.Ok(deviceService.TypeDefaults(), c)
}

func (d DeviceApi) CreateUpgradeTask(c *gin.Context) {
	var req services.CreateDeviceUpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	guid := c.Param("guid")
	release, err := releaseService.GetEnabled(req.ReleaseGuid)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	task, err := deviceUpgradeService.CreateTask(guid, req, publicReleaseDownloadURL(c, release))
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "create", Resource: "device_upgrade", ResourceID: task.Guid, Message: guid, SourceIP: c.ClientIP()})
	response.Ok(task, c)
}

func (d DeviceApi) ListUpgradeTasks(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := deviceUpgradeService.List(c.Param("guid"), params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (d DeviceApi) ReportUpgrade(c *gin.Context) {
	var req services.DeviceUpgradeReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if req.Token == "" {
		req.Token = utils.BearerToken(c)
	}
	if err := deviceUpgradeService.Report(req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(true, c)
}
