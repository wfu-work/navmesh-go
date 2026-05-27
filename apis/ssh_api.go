package apis

import (
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type SSHApi struct{}

func (s SSHApi) ListEntrypoints(c *gin.Context) {
	items, err := sshService.ListEntrypoints()
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(items, c)
}

func (s SSHApi) SaveEntrypoint(c *gin.Context) {
	var req services.SaveSSHEntrypointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := sshService.SaveEntrypoint(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "ssh_entrypoint", ResourceID: item.IP, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (s SSHApi) ListAliases(c *gin.Context) {
	items, err := sshService.ListAliases()
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(items, c)
}

func (s SSHApi) SaveAlias(c *gin.Context) {
	var req services.SaveSSHAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := sshService.SaveAlias(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "ssh_alias", ResourceID: item.Domain, Message: item.Alias, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (s SSHApi) DisableAlias(c *gin.Context) {
	guid := c.Param("guid")
	if err := sshService.DisableAlias(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "ssh_alias", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}
