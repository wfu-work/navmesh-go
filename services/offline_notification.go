package services

import (
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"
)

func DeviceOfflineTemplateVariables(device *domains.Device, message string, now int64) map[string]string {
	if device == nil {
		return map[string]string{}
	}
	eventTime := formatTemplateTime(now)
	return map[string]string{
		"eventTitle":    "设备离线",
		"eventMessage":  strings.TrimSpace(message),
		"time":          eventTime,
		"deviceSncode":  strings.TrimSpace(device.Sncode),
		"deviceAlias":   utils.FirstNonEmpty(strings.TrimSpace(device.Alias), strings.TrimSpace(device.Sncode), strings.TrimSpace(device.Hostname), strings.TrimSpace(device.Guid)),
		"deviceType":    strings.TrimSpace(device.DeviceType),
		"hostIp":        strings.TrimSpace(device.HostIP),
		"wanIp":         strings.TrimSpace(device.WanIP),
		"clientVersion": strings.TrimSpace(device.ClientVersion),
		"lastSeenTime":  formatTemplateTime(device.LastSeenTime),
	}
}

func formatTemplateTime(value int64) string {
	if value <= 0 {
		return "-"
	}
	return time.UnixMilli(value).In(time.Local).Format("2006-01-02 15:04:05")
}
