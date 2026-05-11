package apis

import (
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type MaintenanceApi struct{}

func (m MaintenanceApi) CleanupRetention(c *gin.Context) {
	result := maintenanceService.CleanupRetention()
	auditService.Record(services.AuditInput{
		Actor:    actorName(c),
		Action:   "cleanup_retention",
		Resource: "maintenance",
		SourceIP: c.ClientIP(),
	})
	response.Ok(result, c)
}
