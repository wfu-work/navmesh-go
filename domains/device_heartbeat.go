package domains

type DeviceHeartbeat struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	SourceIP   string `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	HostIP     string `json:"hostIp" gorm:"size:64;comment:设备本机IP"`
	WanIP      string `json:"wanIp" gorm:"size:64;comment:设备外网IP"`
	Location   string `json:"location" gorm:"size:255;comment:设备区域位置"`
	CreateTime int64  `json:"createTime" gorm:"index;comment:创建时间"`
}

func (DeviceHeartbeat) TableName() string { return "navmesh_device_heartbeats" }
