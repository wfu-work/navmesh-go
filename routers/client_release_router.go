package routers

import "github.com/gin-gonic/gin"

type ClientReleaseRouter struct{}

func (r *ClientReleaseRouter) InitClientReleaseRouter(publicGroup *gin.RouterGroup, privateGroup *gin.RouterGroup) {
	downloads := publicGroup.Group("downloads")
	{
		downloads.GET(":fileName", clientReleaseApi.Download)
	}

	releases := privateGroup.Group("client-releases")
	{
		releases.GET("list", clientReleaseApi.List)
		releases.POST("upload", clientReleaseApi.Upload)
	}
}
