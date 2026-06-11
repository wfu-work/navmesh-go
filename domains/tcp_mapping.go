package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

type TCPMapping struct {
	commonDomains.BaseDataEntity
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;comment:设备guid"`
	Name       string `json:"name" gorm:"size:128;comment:名称"`
	PublicHost string `json:"publicHost" gorm:"size:255;index;comment:外部域名"`
	PublicPort int    `json:"publicPort" gorm:"index;comment:外部TCP端口"`
	TargetHost string `json:"targetHost" gorm:"size:64;comment:目标主机"`
	TargetPort int    `json:"targetPort" gorm:"comment:目标端口"`
	Remark     string `json:"remark" gorm:"size:512;comment:备注"`
	Status     int    `json:"status" gorm:"index;comment:状态"`
}

func (TCPMapping) TableName() string { return "navmesh_tcp_mappings" }

func (s TCPMapping) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
