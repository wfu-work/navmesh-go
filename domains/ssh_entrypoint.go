package domains

type SSHEntrypoint struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	IP         string `json:"ip" gorm:"size:64;uniqueIndex;comment:入口IPv4或IPv6"`
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;comment:绑定设备ID"`
	Status     int    `json:"status" gorm:"index;comment:状态"`
	CreateTime int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (SSHEntrypoint) TableName() string { return "navmesh_ssh_entrypoints" }
