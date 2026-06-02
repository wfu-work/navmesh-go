package apis

import (
	"net/http"
	"strings"

	deployAssets "navmesh-go/deploy"
	"navmesh-go/domains"
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type ReleaseApi struct{}

func (a ReleaseApi) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := releaseService.Upload(file, services.UploadReleaseRequest{
		ReleaseType: c.PostForm("releaseType"),
		DeviceType:  c.PostForm("deviceType"),
		Version:     c.PostForm("version"),
		OS:          c.PostForm("os"),
		Arch:        c.PostForm("arch"),
		DownloadURL: c.PostForm("downloadUrl"),
		ChangeLog:   c.PostForm("changeLog"),
	})
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "upload", Resource: "release", ResourceID: item.Guid, Message: item.FileName, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (a ReleaseApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := releaseService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a ReleaseApi) Get(c *gin.Context) {
	item, err := releaseService.Get(c.Param("guid"))
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(item, c)
}

func (a ReleaseApi) Update(c *gin.Context) {
	file, _ := c.FormFile("file")
	item, err := releaseService.Update(c.Param("guid"), file, services.UploadReleaseRequest{
		ReleaseType: c.PostForm("releaseType"),
		DeviceType:  c.PostForm("deviceType"),
		Version:     c.PostForm("version"),
		OS:          c.PostForm("os"),
		Arch:        c.PostForm("arch"),
		DownloadURL: c.PostForm("downloadUrl"),
		ChangeLog:   c.PostForm("changeLog"),
	})
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "update", Resource: "release", ResourceID: item.Guid, Message: item.FileName, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (a ReleaseApi) Enable(c *gin.Context) {
	if err := releaseService.SetStatus(c.Param("guid"), domains.ReleaseStatusEnabled); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "enable", Resource: "release", ResourceID: c.Param("guid"), SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a ReleaseApi) Disable(c *gin.Context) {
	if err := releaseService.SetStatus(c.Param("guid"), domains.ReleaseStatusDisabled); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "disable", Resource: "release", ResourceID: c.Param("guid"), SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a ReleaseApi) Delete(c *gin.Context) {
	if err := releaseService.Delete(c.Param("guid")); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "delete", Resource: "release", ResourceID: c.Param("guid"), SourceIP: c.ClientIP()})
	response.Ok(true, c)
}

func (a ReleaseApi) Download(c *gin.Context) {
	fileName := strings.TrimSpace(c.Param("fileName"))
	if fileName == "install-client.sh" {
		c.Header("Content-Disposition", `attachment; filename="install-client.sh"`)
		c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", deployAssets.InstallClientScript)
		return
	}
	item, err := releaseService.FindDownload(fileName)
	if err != nil {
		c.String(http.StatusNotFound, err.Error())
		return
	}
	c.FileAttachment(item.FilePath, item.FileName)
}

func (a ReleaseApi) DownloadByGuid(c *gin.Context) {
	item, err := releaseService.FindDownloadByGuid(c.Param("guid"))
	if err != nil {
		c.String(http.StatusNotFound, err.Error())
		return
	}
	c.FileAttachment(item.FilePath, item.FileName)
}

func (a ReleaseApi) DownloadLatest(c *gin.Context) {
	item, err := releaseService.FindLatestDownload(utils.QueryParams(c))
	if err != nil {
		c.String(http.StatusNotFound, err.Error())
		return
	}
	c.FileAttachment(item.FilePath, item.FileName)
}

func publicDownloadURL(c *gin.Context, fileName string) string {
	base := strings.TrimSpace(settingService.Value("client_download_base", ""))
	if base == "" {
		scheme := firstForwardedValue(c.GetHeader("X-Forwarded-Proto"))
		if scheme == "" {
			if c.Request.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		host := firstForwardedValue(c.GetHeader("X-Forwarded-Host"))
		if host == "" {
			host = c.Request.Host
		}
		base = scheme + "://" + host + "/api/downloads"
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(fileName, "/")
}

func publicReleaseDownloadURL(c *gin.Context, item *domains.Release) string {
	if item == nil {
		return ""
	}
	base := strings.TrimSpace(settingService.Value("client_download_base", ""))
	if base == "" {
		scheme := firstForwardedValue(c.GetHeader("X-Forwarded-Proto"))
		if scheme == "" {
			if c.Request.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		host := firstForwardedValue(c.GetHeader("X-Forwarded-Host"))
		if host == "" {
			host = c.Request.Host
		}
		base = scheme + "://" + host + "/api/downloads"
	}
	if item.Guid != "" {
		return strings.TrimRight(base, "/") + "/releases/" + strings.TrimLeft(item.Guid, "/")
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(item.FileName, "/")
}

func firstForwardedValue(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, ","); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}
