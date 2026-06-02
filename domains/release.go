package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

const (
	ReleaseStatusDisabled = 0
	ReleaseStatusEnabled  = 1

	ReleaseTypeRain     = "rain"
	ReleaseTypeHipnames = "hipnames"
	ReleaseTypeDIC      = "dic"
	ReleaseTypeNavmesh  = "navmesh"
)

type Release struct {
	commonDomains.BaseDataEntity
	ReleaseType string `json:"releaseType" gorm:"size:32;index;comment:版本类型"`
	DeviceType  string `json:"deviceType" gorm:"size:64;index;comment:设备类型"`
	Version     string `json:"version" gorm:"size:64;index;comment:版本号"`
	OS          string `json:"os" gorm:"size:32;index;comment:目标系统"`
	Arch        string `json:"arch" gorm:"size:32;index;comment:目标架构"`
	FileName    string `json:"fileName" gorm:"size:255;index;comment:文件名"`
	FilePath    string `json:"filePath" gorm:"size:512;comment:本地文件路径"`
	Sha256      string `json:"sha256" gorm:"size:64;comment:SHA256"`
	Size        int64  `json:"size" gorm:"comment:文件大小"`
	DownloadURL string `json:"downloadUrl" gorm:"size:512;comment:自定义下载地址"`
	ChangeLog   string `json:"changeLog" gorm:"type:text;comment:更新内容"`
	Status      int    `json:"status" gorm:"index;comment:状态"`
}

func (Release) TableName() string { return "navmesh_releases" }

func (s Release) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}

func (s *Release) NormalizeDefaults() {
	if s.ReleaseType == "" {
		s.ReleaseType = ReleaseTypeNavmesh
	}
	if s.DeviceType == "" {
		s.DeviceType = "all"
	}
}
