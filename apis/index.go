package apis

import (
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

var ApiGroupApp = new(ApiGroup)

type ApiGroup struct {
	AccessPolicyApi
	AuditApi
	ReleaseApi
	DeviceApi
	EventApi
	GroupApi
	MappingApi
	MaintenanceApi
	MessageApi
	SessionApi
	SettingApi
	SSHApi
	TCPMappingApi
	TunnelApi
}

var (
	accessPolicyService  = services.ServiceGroupApp.AccessPolicyService
	auditService         = services.ServiceGroupApp.AuditService
	releaseService       = services.ServiceGroupApp.ReleaseService
	customDomainService  = services.ServiceGroupApp.CustomDomainService
	deviceService        = services.ServiceGroupApp.DeviceService
	deviceTokenService   = services.ServiceGroupApp.DeviceTokenService
	deviceTrafficService = services.ServiceGroupApp.DeviceTrafficService
	deviceUpgradeService = services.ServiceGroupApp.DeviceUpgradeService
	eventService         = services.ServiceGroupApp.EventService
	emailService         = services.ServiceGroupApp.EmailService
	groupService         = services.ServiceGroupApp.GroupService
	mappingService       = services.ServiceGroupApp.MappingService
	maintenanceService   = services.ServiceGroupApp.MaintenanceService
	messageService       = services.ServiceGroupApp.MessageService
	sessionService       = services.ServiceGroupApp.SessionService
	settingService       = services.ServiceGroupApp.SettingService
	sshService           = services.ServiceGroupApp.SSHService
	tcpMappingService    = services.ServiceGroupApp.TCPMappingService
)

// bindJSON centralizes request decoding and keeps malformed JSON responses
// consistent across API handlers.
func bindJSON[T any](c *gin.Context) (T, bool) {
	var request T
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FailWithMessage(err.Error(), c)
		return request, false
	}
	return request, true
}

// fail writes the standard API error response and returns true when an error
// is present, allowing handlers to use a compact early-return pattern.
func fail(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	response.FailWithMessage(err.Error(), c)
	return true
}

// recordAudit fills request-scoped audit metadata in one place. Callers only
// need to provide the action/resource-specific fields.
func recordAudit(c *gin.Context, input services.AuditInput) {
	if c == nil {
		return
	}
	input.Actor = actorName(c)
	input.SourceIP = c.ClientIP()
	auditService.Record(input)
}
