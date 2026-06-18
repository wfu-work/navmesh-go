package domains

type DeviceTrafficState struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	DeviceGuid    string `json:"deviceGuid" gorm:"size:64;uniqueIndex:idx_device_traffic_state;comment:设备ID"`
	Interface     string `json:"iface" gorm:"column:iface;size:64;uniqueIndex:idx_device_traffic_state;comment:统计网卡"`
	TrafficBootID string `json:"trafficBootId" gorm:"size:128;comment:启动标识"`
	RXBytes       int64  `json:"rxBytes" gorm:"comment:最近累计接收字节"`
	TXBytes       int64  `json:"txBytes" gorm:"comment:最近累计发送字节"`
	SampleTime    int64  `json:"sampleTime" gorm:"index;comment:最近采样时间"`
	CreateTime    int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime    int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (DeviceTrafficState) TableName() string { return "navmesh_device_traffic_states" }

type DeviceTrafficDaily struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	DeviceGuid    string `json:"deviceGuid" gorm:"size:64;uniqueIndex:idx_device_traffic_daily;index;comment:设备ID"`
	Interface     string `json:"iface" gorm:"column:iface;size:64;uniqueIndex:idx_device_traffic_daily;comment:统计网卡"`
	Day           string `json:"day" gorm:"size:10;uniqueIndex:idx_device_traffic_daily;index;comment:统计日期"`
	RXBytes       int64  `json:"rxBytes" gorm:"comment:当日接收字节增量"`
	TXBytes       int64  `json:"txBytes" gorm:"comment:当日发送字节增量"`
	TotalBytes    int64  `json:"totalBytes" gorm:"comment:当日总字节增量"`
	SampleCount   int64  `json:"sampleCount" gorm:"comment:有效采样次数"`
	ResetCount    int64  `json:"resetCount" gorm:"comment:计数器重置次数"`
	FirstSeenTime int64  `json:"firstSeenTime" gorm:"index;comment:当日首次采样时间"`
	LastSeenTime  int64  `json:"lastSeenTime" gorm:"index;comment:当日最近采样时间"`
	CreateTime    int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime    int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (DeviceTrafficDaily) TableName() string { return "navmesh_device_traffic_daily" }
