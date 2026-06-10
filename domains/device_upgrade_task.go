package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

const (
	DeviceUpgradeStatusPending  = 1
	DeviceUpgradeStatusRunning  = 2
	DeviceUpgradeStatusSuccess  = 3
	DeviceUpgradeStatusFailed   = 4
	DeviceUpgradeStatusCanceled = 5
)

type DeviceUpgradeTask struct {
	commonDomains.BaseDataEntity
	DeviceGuid     string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	ReleaseGuid    string `json:"releaseGuid" gorm:"size:64;index;comment:客户端版本ID"`
	Version        string `json:"version" gorm:"size:64;comment:目标版本"`
	OS             string `json:"os" gorm:"size:32;comment:目标系统"`
	Arch           string `json:"arch" gorm:"size:32;comment:目标架构"`
	FileName       string `json:"fileName" gorm:"size:255;comment:文件名"`
	DownloadURL    string `json:"downloadUrl" gorm:"size:512;comment:下载地址"`
	Sha256         string `json:"sha256" gorm:"size:64;comment:SHA256"`
	Size           int64  `json:"size" gorm:"comment:文件大小"`
	FromVersion    string `json:"fromVersion" gorm:"size:64;comment:升级前版本"`
	CurrentVersion string `json:"currentVersion" gorm:"size:64;comment:上报版本"`
	Status         int    `json:"status" gorm:"index;comment:状态"`
	Progress       int    `json:"progress" gorm:"comment:升级进度百分比"`
	DownloadedSize int64  `json:"downloadedSize" gorm:"comment:已下载大小"`
	Message        string `json:"message" gorm:"type:text;comment:消息"`
	ErrorMessage   string `json:"errorMessage" gorm:"type:text;comment:错误消息"`
	StartTime      int64  `json:"startTime" gorm:"index;comment:开始时间"`
	FinishTime     int64  `json:"finishTime" gorm:"index;comment:完成时间"`
}

func (DeviceUpgradeTask) TableName() string { return "navmesh_device_upgrade_tasks" }

func (s DeviceUpgradeTask) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
