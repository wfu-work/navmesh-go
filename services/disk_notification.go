package services

import (
	"fmt"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"
)

func DiskUsageHighTemplateVariables(device *domains.Device, req HeartbeatRequest, message string, now int64) map[string]string {
	if device == nil {
		return map[string]string{}
	}
	eventTime := time.Now().Format("2006-01-02 15:04:05")
	if now > 0 {
		eventTime = time.UnixMilli(now).In(time.Local).Format("2006-01-02 15:04:05")
	}
	diskTotal := firstPositiveInt64(req.DiskTotal, device.DiskTotal)
	diskUsed := firstPositiveInt64(req.DiskUsed, device.DiskUsed)
	diskFree := device.DiskFree
	if req.DiskFree > 0 || req.DiskTotal > 0 || req.DiskUsed > 0 {
		diskFree = req.DiskFree
	}
	if diskFree < 0 {
		diskFree = 0
	}
	if diskUsed <= 0 && diskTotal > 0 {
		diskUsed = diskTotal - diskFree
		if diskUsed < 0 {
			diskUsed = 0
		}
	}
	diskUsedPct := req.DiskUsedPct
	if diskUsedPct <= 0 {
		diskUsedPct = device.DiskUsedPct
	}
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("磁盘使用率 %.1f%%，已达到 %.0f%% 告警阈值", diskUsedPct, diskUsageHighThreshold)
	}
	return map[string]string{
		"eventTitle":    "磁盘空间不足",
		"eventMessage":  strings.TrimSpace(message),
		"time":          eventTime,
		"deviceSncode":  strings.TrimSpace(device.Sncode),
		"deviceAlias":   utils.FirstNonEmpty(strings.TrimSpace(device.Alias), strings.TrimSpace(device.Sncode), strings.TrimSpace(req.Hostname), strings.TrimSpace(device.Hostname), strings.TrimSpace(device.Guid)),
		"deviceType":    strings.TrimSpace(device.DeviceType),
		"hostIp":        utils.FirstNonEmpty(strings.TrimSpace(req.HostIP), strings.TrimSpace(device.HostIP)),
		"wanIp":         utils.FirstNonEmpty(strings.TrimSpace(req.WanIP), strings.TrimSpace(device.WanIP)),
		"clientVersion": utils.FirstNonEmpty(strings.TrimSpace(req.ClientVersion), strings.TrimSpace(device.ClientVersion)),
		"diskUsedPct":   fmt.Sprintf("%.1f", diskUsedPct),
		"diskThreshold": fmt.Sprintf("%.0f", diskUsageHighThreshold),
		"diskFree":      formatStorageSize(diskFree),
		"diskTotal":     formatStorageSize(diskTotal),
		"diskUsed":      formatStorageSize(diskUsed),
	}
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
