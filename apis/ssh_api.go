package apis

import (
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type SSHApi struct{}

func (s SSHApi) ListEntrypoints(c *gin.Context) {
	items, err := sshService.ListEntrypoints()
	if fail(c, err) {
		return
	}
	response.Ok(items, c)
}

func (s SSHApi) SaveEntrypoint(c *gin.Context) {
	req, ok := bindJSON[services.SaveSSHEntrypointRequest](c)
	if !ok {
		return
	}
	item, err := sshService.SaveEntrypoint(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "ssh_entrypoint", ResourceID: item.IP})
	response.Ok(item, c)
}

func (s SSHApi) ListAliases(c *gin.Context) {
	items, err := sshService.ListAliases()
	if fail(c, err) {
		return
	}
	response.Ok(items, c)
}

func (s SSHApi) SaveAlias(c *gin.Context) {
	req, ok := bindJSON[services.SaveSSHAliasRequest](c)
	if !ok {
		return
	}
	item, err := sshService.SaveAlias(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "ssh_alias", ResourceID: item.Domain, Message: item.Alias})
	response.Ok(item, c)
}

func (s SSHApi) DisableAlias(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, sshService.DisableAlias(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "ssh_alias", ResourceID: guid})
	response.Ok(true, c)
}
