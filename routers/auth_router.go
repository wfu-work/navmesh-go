package routers

import "github.com/gin-gonic/gin"

type AuthRouter struct{}

func (a *AuthRouter) InitAuthRouter(publicGroup *gin.RouterGroup, privateGroup *gin.RouterGroup) {
	public := publicGroup.Group("navmesh-auth")
	{
		public.POST("login", authApi.Login)
	}
	private := privateGroup.Group("navmesh-auth")
	{
		private.GET("profile", authApi.Profile)
		private.PUT("password", authApi.ChangePassword)
	}
}
