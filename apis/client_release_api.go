package apis

import (
	"net/http"
	"strings"

	deployAssets "navmesh-go/deploy"
	"navmesh-go/services"
	"navmesh-go/utils"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/response"
)

type ClientReleaseApi struct{}

func (a ClientReleaseApi) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	item, err := clientReleaseService.Upload(file, services.UploadClientReleaseRequest{
		Version:     c.PostForm("version"),
		OS:          c.PostForm("os"),
		Arch:        c.PostForm("arch"),
		DownloadURL: c.PostForm("downloadUrl"),
	})
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	auditService.Record(services.AuditInput{Actor: actorName(c), Action: "upload", Resource: "client_release", ResourceID: item.Guid, Message: item.FileName, SourceIP: c.ClientIP()})
	response.Ok(item, c)
}

func (a ClientReleaseApi) List(c *gin.Context) {
	params := utils.QueryParams(c)
	items, total, err := clientReleaseService.List(params)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.Ok(services.PageResult(items, total, params), c)
}

func (a ClientReleaseApi) Download(c *gin.Context) {
	fileName := strings.TrimSpace(c.Param("fileName"))
	if fileName == "install-client.sh" {
		c.Header("Content-Disposition", `attachment; filename="install-client.sh"`)
		c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", deployAssets.InstallClientScript)
		return
	}
	item, err := clientReleaseService.FindDownload(fileName)
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

func firstForwardedValue(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, ","); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}
