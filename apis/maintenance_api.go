package apis

import (
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type MaintenanceApi struct{}

func (m MaintenanceApi) DatabaseStats(c *gin.Context) {
	result, err := maintenanceService.DatabaseStats()
	if fail(c, err) {
		return
	}
	response.Ok(result, c)
}

func (m MaintenanceApi) CleanupRetention(c *gin.Context) {
	result := maintenanceService.CleanupRetention()
	recordAudit(c, services.AuditInput{Action: "cleanup_retention", Resource: "maintenance"})
	response.Ok(result, c)
}
