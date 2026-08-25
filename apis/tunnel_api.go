package apis

import (
	"navmesh-go/httpgateway"
	"navmesh-go/services"
	"navmesh-go/tunnel"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/global"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type TunnelApi struct{}

func (t TunnelApi) Connections(c *gin.Context) {
	response.Ok(tunnel.DefaultManager.List(), c)
}

func (t TunnelApi) RuntimeStats(c *gin.Context) {
	dbStats, _ := services.CollectDatabaseRuntimeStats(global.NAV_DB)
	dbPool := any(nil)
	if global.NAV_DB != nil {
		if sqlDB, err := global.NAV_DB.DB(); err == nil {
			dbPool = sqlDB.Stats()
		}
	}
	response.Ok(gin.H{
		"activeTunnels":  len(tunnel.DefaultManager.List()),
		"dirtyLiveness":  services.DefaultLivenessRegistry.DirtyCount(),
		"httpRoutes":     httpgateway.DefaultRouteStore.Stats(),
		"httpAccessLogs": httpgateway.CurrentAccessLogWriterStats(),
		"notifications":  services.DefaultNotificationRunner.Stats(),
		"database":       dbStats,
		"databasePool":   dbPool,
	}, c)
}
