package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type MessageApi struct{}

func (a MessageApi) ListEmailConfigs(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := messageService.ListEmailConfigs(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a MessageApi) SaveEmailConfig(c *gin.Context) {
	var req services.SaveMessageEmailConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := messageService.SaveEmailConfig(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "message_email_config", ResourceID: item.Guid, Message: item.Name, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (a MessageApi) SetDefaultEmailConfig(c *gin.Context) {
	guid := c.Param("guid")
	if err := messageService.SetDefaultEmailConfig(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "default", Resource: "message_email_config", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a MessageApi) DisableEmailConfig(c *gin.Context) {
	guid := c.Param("guid")
	if err := messageService.DisableEmailConfig(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "message_email_config", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a MessageApi) DeleteEmailConfig(c *gin.Context) {
	guid := c.Param("guid")
	if err := messageService.DeleteEmailConfig(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "delete", Resource: "message_email_config", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a MessageApi) ListTemplates(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := messageService.ListTemplates(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a MessageApi) GetTemplate(c *gin.Context) {
	item, err := messageService.GetTemplate(c.Param("guid"))
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(item, c)
}

func (a MessageApi) SaveTemplate(c *gin.Context) {
	var req services.SaveMessageTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := messageService.SaveTemplate(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "message_template", ResourceID: item.Guid, Message: item.Name, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (a MessageApi) PreviewTemplate(c *gin.Context) {
	var req services.EmailTemplatePreviewInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	result, err := emailService.PreviewTemplate(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(result, c)
}

func (a MessageApi) DisableTemplate(c *gin.Context) {
	guid := c.Param("guid")
	if err := messageService.DisableTemplate(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "message_template", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a MessageApi) DeleteTemplate(c *gin.Context) {
	guid := c.Param("guid")
	if err := messageService.DeleteTemplate(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "delete", Resource: "message_template", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a MessageApi) ListRecipients(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := messageService.ListRecipients(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a MessageApi) SaveRecipient(c *gin.Context) {
	var req services.SaveMessageRecipientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := messageService.SaveRecipient(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "message_recipient", ResourceID: item.Guid, Message: item.Name, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (a MessageApi) DisableRecipient(c *gin.Context) {
	guid := c.Param("guid")
	if err := messageService.DisableRecipient(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "message_recipient", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a MessageApi) DeleteRecipient(c *gin.Context) {
	guid := c.Param("guid")
	if err := messageService.DeleteRecipient(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "delete", Resource: "message_recipient", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a MessageApi) ListSendRecords(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := messageService.ListSendRecords(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a MessageApi) GetSendRecord(c *gin.Context) {
	item, err := messageService.GetSendRecord(c.Param("guid"))
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(item, c)
}

func (a MessageApi) RetrySendRecord(c *gin.Context) {
	guid := c.Param("guid")
	item, err := messageService.RetrySendRecord(guid)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "retry", Resource: "message_send_record", ResourceID: guid, Message: item.RecipientEmail, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (a MessageApi) DeleteSendRecord(c *gin.Context) {
	guid := c.Param("guid")
	if err := messageService.DeleteSendRecord(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "delete", Resource: "message_send_record", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a MessageApi) DebugSendTemplate(c *gin.Context) {
	var req services.DebugEmailTemplateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	result, err := emailService.DebugSendTemplate(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "debug_send", Resource: "message_send_record", ResourceID: req.TemplateCode, Message: result.Subject, SourceIP: c.ClientIP()})
	response.Ok(result, c)
}
