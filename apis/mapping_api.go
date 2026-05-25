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
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m MappingApi) Save(c *gin.Context) {
	var req services.SavePortMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := mappingService.Save(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "port_mapping", ResourceID: item.Guid, Message: item.PublicHost, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (m MappingApi) Disable(c *gin.Context) {
	guid := c.Param("guid")
	if err := mappingService.Disable(guid); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "port_mapping", ResourceID: guid, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (m MappingApi) AccessLogs(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := mappingService.AccessLogs(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m MappingApi) CustomDomains(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := mappingService.CustomDomains(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (m MappingApi) SaveCustomDomain(c *gin.Context) {
	var req services.SaveCustomDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := mappingService.SaveCustomDomain(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "save", Resource: "custom_domain", ResourceID: item.Domain, Message: item.MappingGuid, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (m MappingApi) VerifyCustomDomain(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	domain := c.Param("domain")
	if err := mappingService.VerifyCustomDomain(domain, req.Token); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "verify", Resource: "custom_domain", ResourceID: domain, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (m MappingApi) DisableCustomDomain(c *gin.Context) {
	domain := c.Param("domain")
	if err := mappingService.DisableCustomDomain(domain); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "custom_domain", ResourceID: domain, SourceIP: c.ClientIP()})
	response.Ok(true, c)
}
