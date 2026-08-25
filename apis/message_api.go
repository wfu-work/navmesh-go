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
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a MessageApi) SaveEmailConfig(c *gin.Context) {
	req, ok := bindJSON[services.SaveMessageEmailConfigRequest](c)
	if !ok {
		return
	}
	item, err := messageService.SaveEmailConfig(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "message_email_config", ResourceID: item.Guid, Message: item.Name})
	response.Ok(item, c)
}

func (a MessageApi) SetDefaultEmailConfig(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, messageService.SetDefaultEmailConfig(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "default", Resource: "message_email_config", ResourceID: guid})
	response.Ok(true, c)
}

func (a MessageApi) DisableEmailConfig(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, messageService.DisableEmailConfig(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "message_email_config", ResourceID: guid})
	response.Ok(true, c)
}

func (a MessageApi) DeleteEmailConfig(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, messageService.DeleteEmailConfig(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "delete", Resource: "message_email_config", ResourceID: guid})
	response.Ok(true, c)
}

func (a MessageApi) ListTemplates(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := messageService.ListTemplates(params)
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a MessageApi) GetTemplate(c *gin.Context) {
	item, err := messageService.GetTemplate(c.Param("guid"))
	if fail(c, err) {
		return
	}
	response.Ok(item, c)
}

func (a MessageApi) SaveTemplate(c *gin.Context) {
	req, ok := bindJSON[services.SaveMessageTemplateRequest](c)
	if !ok {
		return
	}
	item, err := messageService.SaveTemplate(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "message_template", ResourceID: item.Guid, Message: item.Name})
	response.Ok(item, c)
}

func (a MessageApi) PreviewTemplate(c *gin.Context) {
	req, ok := bindJSON[services.EmailTemplatePreviewInput](c)
	if !ok {
		return
	}
	result, err := emailService.PreviewTemplate(req)
	if fail(c, err) {
		return
	}
	response.Ok(result, c)
}

func (a MessageApi) DisableTemplate(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, messageService.DisableTemplate(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "message_template", ResourceID: guid})
	response.Ok(true, c)
}

func (a MessageApi) DeleteTemplate(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, messageService.DeleteTemplate(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "delete", Resource: "message_template", ResourceID: guid})
	response.Ok(true, c)
}

func (a MessageApi) ListRecipients(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := messageService.ListRecipients(params)
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a MessageApi) SaveRecipient(c *gin.Context) {
	req, ok := bindJSON[services.SaveMessageRecipientRequest](c)
	if !ok {
		return
	}
	item, err := messageService.SaveRecipient(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "message_recipient", ResourceID: item.Guid, Message: item.Name})
	response.Ok(item, c)
}

func (a MessageApi) DisableRecipient(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, messageService.DisableRecipient(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "message_recipient", ResourceID: guid})
	response.Ok(true, c)
}

func (a MessageApi) DeleteRecipient(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, messageService.DeleteRecipient(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "delete", Resource: "message_recipient", ResourceID: guid})
	response.Ok(true, c)
}

func (a MessageApi) ListSendRecords(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := messageService.ListSendRecords(params)
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a MessageApi) GetSendRecord(c *gin.Context) {
	item, err := messageService.GetSendRecord(c.Param("guid"))
	if fail(c, err) {
		return
	}
	response.Ok(item, c)
}

func (a MessageApi) RetrySendRecord(c *gin.Context) {
	guid := c.Param("guid")
	item, err := messageService.RetrySendRecord(guid)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "retry", Resource: "message_send_record", ResourceID: guid, Message: item.RecipientEmail})
	response.Ok(item, c)
}

func (a MessageApi) DeleteSendRecord(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, messageService.DeleteSendRecord(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "delete", Resource: "message_send_record", ResourceID: guid})
	response.Ok(true, c)
}

func (a MessageApi) DebugSendTemplate(c *gin.Context) {
	req, ok := bindJSON[services.DebugEmailTemplateInput](c)
	if !ok {
		return
	}
	result, err := emailService.DebugSendTemplate(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "debug_send", Resource: "message_send_record", ResourceID: req.TemplateCode, Message: result.Subject})
	response.Ok(result, c)
}
