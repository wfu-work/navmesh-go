package services

import (
	"fmt"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"
)

func ReleasePublishedTitle(item *domains.Release) string {
	return ReleaseTypeText(item) + "版本已发布"
}

func ReleasePublishedMessage(item *domains.Release) string {
	if item == nil {
		return ""
	}
	parts := make([]string, 0, 5)
	for _, part := range []string{item.Version, ReleasePlatformText(item), item.DeviceType, item.FileName} {
		if part = strings.TrimSpace(part); part != "" && part != "all" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " · ")
}

func ReleasePublishedTemplateVariables(item *domains.Release, downloadURL string) map[string]string {
	if item == nil {
		return map[string]string{}
	}
	deviceScope := strings.TrimSpace(item.DeviceType)
	if deviceScope == "" || deviceScope == "all" {
		deviceScope = "全部设备"
	}
	changeLog := strings.TrimSpace(item.ChangeLog)
	if changeLog == "" {
		changeLog = "本次发布未填写更新说明"
	}
	publishedAt := time.Now().Format("2006-01-02 15:04:05")
	if item.UpdateTime > 0 {
		publishedAt = time.UnixMilli(item.UpdateTime).Format("2006-01-02 15:04:05")
	}
	title := ReleasePublishedTitle(item)
	message := ReleasePublishedMessage(item)
	return map[string]string{
		"eventTitle":   title,
		"eventMessage": message,
		"time":         publishedAt,
		"releaseType":  ReleaseTypeText(item),
		"version":      strings.TrimSpace(item.Version),
		"platform":     utils.FirstNonEmpty(ReleasePlatformText(item), "全部平台"),
		"os":           utils.FirstNonEmpty(strings.TrimSpace(item.OS), "all"),
		"arch":         utils.FirstNonEmpty(strings.TrimSpace(item.Arch), "all"),
		"deviceScope":  deviceScope,
		"fileName":     strings.TrimSpace(item.FileName),
		"downloadUrl":  utils.FirstNonEmpty(downloadURL, strings.TrimSpace(item.DownloadURL)),
		"changeLog":    changeLog,
		"publishedAt":  publishedAt,
		"releaseGuid":  strings.TrimSpace(item.Guid),
		"releaseSize":  fmt.Sprintf("%d", item.Size),
	}
}

func ReleaseTypeText(item *domains.Release) string {
	if item == nil {
		return "新"
	}
	switch strings.ToLower(strings.TrimSpace(item.ReleaseType)) {
	case domains.ReleaseTypeRain, "device_software":
		return "北斗降雨水位"
	case domains.ReleaseTypeHipnames, "standalone":
		return "单机版"
	case domains.ReleaseTypeDIC:
		return "视觉位移"
	case domains.ReleaseTypeNavmesh, "", "navmesh_client":
		return "边缘客户端"
	default:
		return strings.TrimSpace(item.ReleaseType)
	}
}

func ReleasePlatformText(item *domains.Release) string {
	if item == nil {
		return ""
	}
	osName := strings.TrimSpace(item.OS)
	arch := strings.TrimSpace(item.Arch)
	if osName == "" || osName == "all" {
		return arch
	}
	if arch == "" || arch == "all" {
		return osName
	}
	return osName + "/" + arch
}
