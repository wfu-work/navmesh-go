package domains

type DeviceConnection struct {
	ID             uint   `json:"id" gorm:"primaryKey"`
	DeviceGuid     string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	ConnectionID   string `json:"connectionId" gorm:"size:64;uniqueIndex;comment:连接ID"`
	Protocol       string `json:"protocol" gorm:"size:32;comment:协议"`
	RemoteAddr     string `json:"remoteAddr" gorm:"size:128;comment:远端地址"`
	Status         int    `json:"status" gorm:"index;comment:状态"`
	ConnectedTime  int64  `json:"connectedTime" gorm:"index;comment:连接时间"`
	LastActiveTime int64  `json:"lastActiveTime" gorm:"index;comment:最后活动时间"`
	CreateTime     int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime     int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (DeviceConnection) TableName() string { return "navmesh_device_connections" }
