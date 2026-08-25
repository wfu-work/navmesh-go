package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

const (
	DeviceTokenStatusDisabled = 0
	DeviceTokenStatusEnabled  = 1
)

type DeviceToken struct {
	commonDomains.BaseDataEntity
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;index:idx_device_token_validate,priority:1;comment:设备ID"`
	Token      string `json:"-" gorm:"size:255;comment:Token（仅通过创建/激活结果返回）"`
	TokenHash  string `json:"-" gorm:"size:255;index:idx_device_token_validate,priority:2;comment:Token Hash"`
	Name       string `json:"name" gorm:"size:128;comment:名称"`
	Status     int    `json:"status" gorm:"index;index:idx_device_token_validate,priority:3;comment:状态"`
	LastUsedAt int64  `json:"lastUsedAt" gorm:"index;comment:最后使用时间"`
	ExpireTime int64  `json:"expireTime" gorm:"index:idx_device_token_validate,priority:4;comment:过期时间"`
}

func (DeviceToken) TableName() string { return "navmesh_device_tokens" }

func (s DeviceToken) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
