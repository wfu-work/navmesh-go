package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

const (
	ClientReleaseStatusDisabled = 0
	ClientReleaseStatusEnabled  = 1
)

type ClientRelease struct {
	commonDomains.BaseDataEntity
	Version     string `json:"version" gorm:"size:64;index;comment:客户端版本"`
	OS          string `json:"os" gorm:"size:32;index;comment:目标系统"`
	Arch        string `json:"arch" gorm:"size:32;index;comment:目标架构"`
	FileName    string `json:"fileName" gorm:"size:255;uniqueIndex;comment:文件名"`
	FilePath    string `json:"filePath" gorm:"size:512;comment:本地文件路径"`
	Sha256      string `json:"sha256" gorm:"size:64;comment:SHA256"`
	Size        int64  `json:"size" gorm:"comment:文件大小"`
	DownloadURL string `json:"downloadUrl" gorm:"size:512;comment:自定义下载地址"`
	Status      int    `json:"status" gorm:"index;comment:状态"`
}

func (ClientRelease) TableName() string { return "navmesh_client_releases" }

func (s ClientRelease) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
