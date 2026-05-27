package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

const (
	DeviceTokenStatusDisabled = 0
	DeviceTokenStatusEnabled  = 1
)

type DeviceToken struct {
	commonDomains.BaseDataEntity
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	Token      string `json:"token" gorm:"size:255;comment:Token"`
	TokenHash  string `json:"-" gorm:"size:255;comment:Token Hash"`
	Name       string `json:"name" gorm:"size:128;comment:名称"`
	Status     int    `json:"status" gorm:"index;comment:状态"`
	ExpireTime int64  `json:"expireTime" gorm:"comment:过期时间"`
}

func (DeviceToken) TableName() string { return "navmesh_device_tokens" }

func (s DeviceToken) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
