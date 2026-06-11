package routers

import "github.com/gin-gonic/gin"

type ReleaseRouter struct{}

func (r *ReleaseRouter) InitReleaseRouter(publicGroup *gin.RouterGroup, privateGroup *gin.RouterGroup) {
	downloads := publicGroup.Group("downloads")
	{
		downloads.GET("releases/latest", releaseApi.DownloadLatest)
		downloads.GET("releases/:guid", releaseApi.DownloadByGuid)
		downloads.GET(":fileName", releaseApi.Download)
	}

	releases := privateGroup.Group("releases")
	{
		releases.GET("list", releaseApi.List)
		releases.GET(":guid/upgrade/candidates", releaseApi.UpgradeCandidates)
		releases.GET(":guid/upgrade/batches", releaseApi.ListUpgradeBatches)
		releases.POST(":guid/upgrade/batches", releaseApi.CreateUpgradeBatch)
		releases.GET(":guid/upgrade/batches/:batchGuid/tasks", releaseApi.ListUpgradeBatchTasks)
		releases.GET(":guid", releaseApi.Get)
		releases.POST("upload", releaseApi.Upload)
		releases.PUT(":guid", releaseApi.Update)
		releases.POST(":guid/enable", releaseApi.Enable)
		releases.POST(":guid/disable", releaseApi.Disable)
		releases.DELETE(":guid/delete", releaseApi.Delete)
	}
}
