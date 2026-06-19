package routers

import "github.com/gin-gonic/gin"

type MessageRouter struct{}

func (r *MessageRouter) InitMessageRouter(router *gin.RouterGroup) {
	group := router.Group("messages")
	{
		emailConfigs := group.Group("email-configs")
		{
			emailConfigs.GET("list", messageApi.ListEmailConfigs)
			emailConfigs.POST("", messageApi.SaveEmailConfig)
			emailConfigs.POST(":guid/default", messageApi.SetDefaultEmailConfig)
			emailConfigs.DELETE(":guid", messageApi.DisableEmailConfig)
			emailConfigs.DELETE(":guid/delete", messageApi.DeleteEmailConfig)
		}

		templates := group.Group("templates")
		{
			templates.GET("list", messageApi.ListTemplates)
			templates.GET(":guid", messageApi.GetTemplate)
			templates.POST("", messageApi.SaveTemplate)
			templates.POST("preview", messageApi.PreviewTemplate)
			templates.DELETE(":guid", messageApi.DisableTemplate)
			templates.DELETE(":guid/delete", messageApi.DeleteTemplate)
		}

		recipients := group.Group("recipients")
		{
			recipients.GET("list", messageApi.ListRecipients)
			recipients.POST("", messageApi.SaveRecipient)
			recipients.DELETE(":guid", messageApi.DisableRecipient)
			recipients.DELETE(":guid/delete", messageApi.DeleteRecipient)
		}

		sendRecords := group.Group("send-records")
		{
			sendRecords.GET("list", messageApi.ListSendRecords)
			sendRecords.POST("debug", messageApi.DebugSendTemplate)
			sendRecords.GET(":guid", messageApi.GetSendRecord)
			sendRecords.POST(":guid/retry", messageApi.RetrySendRecord)
			sendRecords.DELETE(":guid/delete", messageApi.DeleteSendRecord)
		}
	}
}
