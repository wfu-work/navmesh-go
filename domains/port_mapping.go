package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

type PortMapping struct {
	commonDomains.BaseDataEntity
	DeviceGuid     string `json:"deviceGuid" gorm:"size:64;index;comment:设备guid"`
	Name           string `json:"name" gorm:"size:128;comment:名称"`
	PublicHost     string `json:"publicHost" gorm:"size:255;uniqueIndex;comment:外部域名"`
	TargetHost     string `json:"targetHost" gorm:"size:64;comment:目标主机"`
	TargetPort     int    `json:"targetPort" gorm:"comment:目标端口"`
	Protocol       string `json:"protocol" gorm:"size:32;comment:协议"`
	IsCustomDomain bool   `json:"isCustomDomain" gorm:"comment:是否自定义域名"`
	Status         int    `json:"status" gorm:"index;comment:状态"`
}

func (PortMapping) TableName() string { return "navmesh_port_mappings" }

func (s PortMapping) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
