package apis

import (
	"navmesh-go/tunnel"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type TunnelApi struct{}

func (t TunnelApi) Connections(c *gin.Context) {
	response.Ok(tunnel.DefaultManager.List(), c)
}
