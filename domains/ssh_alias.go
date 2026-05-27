package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

type SSHAlias struct {
	commonDomains.BaseDataEntity
	DeviceGuid   string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	Alias        string `json:"alias" gorm:"size:128;uniqueIndex;comment:设备别名"`
	Domain       string `json:"domain" gorm:"size:255;uniqueIndex;comment:SSH 域名"`
	EntrypointIP string `json:"entrypointIp" gorm:"size:64;index;comment:入口IP"`
	Status       int    `json:"status" gorm:"index;comment:状态"`
}

func (SSHAlias) TableName() string { return "navmesh_ssh_aliases" }

func (s SSHAlias) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
