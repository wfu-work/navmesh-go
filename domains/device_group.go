package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

type DeviceGroup struct {
	commonDomains.BaseDataEntity
	Key            string `json:"key" gorm:"column:group_key;size:64;uniqueIndex;comment:唯一标识"`
	Name           string `json:"name" gorm:"size:128;comment:分组名称"`
	DefaultWebPort int    `json:"defaultWebPort" gorm:"comment:默认 Web 端口"`
	DefaultDomain  string `json:"defaultDomain" gorm:"size:255;comment:默认映射域名"`
	Sort           int    `json:"sort" gorm:"index;comment:排序"`
	Remark         string `json:"remark" gorm:"size:512;comment:备注"`
	Status         int    `json:"status" gorm:"index;comment:状态"`
}

func (DeviceGroup) TableName() string { return "navmesh_device_groups" }

func (s DeviceGroup) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
