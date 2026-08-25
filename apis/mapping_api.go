package apis

import (
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type MappingApi struct{}

func (m MappingApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := mappingService.List(params)
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m MappingApi) Save(c *gin.Context) {
	req, ok := bindJSON[services.SavePortMappingRequest](c)
	if !ok {
		return
	}
	item, err := mappingService.Save(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "port_mapping", ResourceID: item.Guid, Message: item.PublicHost})
	response.Ok(item, c)
}

func (m MappingApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if fail(c, mappingService.Disable(guid)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "port_mapping", ResourceID: guid})
	response.Ok(true, c)
}

func (m MappingApi) AccessLogs(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := mappingService.AccessLogs(params)
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m MappingApi) CustomDomains(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := customDomainService.List(params)
	if fail(c, err) {
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m MappingApi) SaveCustomDomain(c *gin.Context) {
	req, ok := bindJSON[services.SaveCustomDomainRequest](c)
	if !ok {
		return
	}
	item, err := customDomainService.Save(req)
	if fail(c, err) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "save", Resource: "custom_domain", ResourceID: item.Domain, Message: item.MappingGuid})
	response.Ok(item, c)
}

func (m MappingApi) VerifyCustomDomain(c *gin.Context) {
	type verifyRequest struct {
		Token string `json:"token"`
	}
	req, ok := bindJSON[verifyRequest](c)
	if !ok {
		return
	}
	domain := c.Param("domain")
	if fail(c, customDomainService.Verify(domain, req.Token)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "verify", Resource: "custom_domain", ResourceID: domain})
	response.Ok(true, c)
}

func (m MappingApi) DisableCustomDomain(c *gin.Context) {
	domain := c.Param("domain")
	if fail(c, customDomainService.Disable(domain)) {
		return
	}
	recordAudit(c, services.AuditInput{Action: "disable", Resource: "custom_domain", ResourceID: domain})
	response.Ok(true, c)
}
