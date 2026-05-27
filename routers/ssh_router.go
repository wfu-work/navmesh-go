package routers

import "github.com/gin-gonic/gin"

type SSHRouter struct{}

func (s *SSHRouter) InitSSHRouter(router *gin.RouterGroup) {
	entrypoints := router.Group("ssh-entrypoints")
	{
		entrypoints.GET("list", sshApi.ListEntrypoints)
		entrypoints.POST("", sshApi.SaveEntrypoint)
	}

	aliases := router.Group("ssh-aliases")
	{
		aliases.GET("list", sshApi.ListAliases)
		aliases.POST("", sshApi.SaveAlias)
		aliases.DELETE(":guid", sshApi.DisableAlias)
	}
}
