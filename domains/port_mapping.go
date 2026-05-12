package domains

type PortMapping struct {
	ID             uint   `json:"id" gorm:"primaryKey"`
	Guid           string `json:"guid" gorm:"size:64;uniqueIndex;comment:映射ID"`
	DeviceGuid     string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	Name           string `json:"name" gorm:"size:128;comment:名称"`
	PublicHost     string `json:"publicHost" gorm:"size:255;uniqueIndex;comment:外部域名"`
	TargetHost     string `json:"targetHost" gorm:"size:64;comment:目标主机"`
	TargetPort     int    `json:"targetPort" gorm:"comment:目标端口"`
	Protocol       string `json:"protocol" gorm:"size:32;comment:协议"`
	IsCustomDomain bool   `json:"isCustomDomain" gorm:"comment:是否自定义域名"`
	Status         int    `json:"status" gorm:"index;comment:状态"`
	CreateTime     int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime     int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (PortMapping) TableName() string { return "navmesh_port_mappings" }
