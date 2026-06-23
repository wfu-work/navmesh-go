package domains

type DeviceHeartbeat struct {
	ID            uint    `json:"id" gorm:"primaryKey"`
	DeviceGuid    string  `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	SourceIP      string  `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	HostIP        string  `json:"hostIp" gorm:"size:64;comment:设备本机IP"`
	WanIP         string  `json:"wanIp" gorm:"size:64;comment:设备外网IP"`
	Location      string  `json:"location" gorm:"size:255;comment:设备区域位置"`
	NetworkType   string  `json:"networkType" gorm:"size:32;index;comment:网络类型"`
	NetworkIface  string  `json:"networkIface" gorm:"size:64;comment:网络接口"`
	SignalDBM     int     `json:"signalDbm" gorm:"column:signal_dbm;comment:信号强度dBm"`
	SignalPct     int     `json:"signalPct" gorm:"comment:信号强度百分比"`
	CellularRSRP  int     `json:"cellularRsrp" gorm:"column:cellular_rsrp;comment:4G/5G RSRP"`
	CellularRSRQ  int     `json:"cellularRsrq" gorm:"column:cellular_rsrq;comment:4G/5G RSRQ"`
	CellularSINR  int     `json:"cellularSinr" gorm:"column:cellular_sinr;comment:4G/5G SINR"`
	WifiSSID      string  `json:"wifiSsid" gorm:"column:wifi_ssid;size:128;comment:WiFi SSID"`
	WifiRSSI      int     `json:"wifiRssi" gorm:"column:wifi_rssi;comment:WiFi RSSI"`
	PingTarget    string  `json:"pingTarget" gorm:"size:255;comment:延迟探测目标"`
	PingLatencyMs int64   `json:"pingLatencyMs" gorm:"comment:延迟毫秒"`
	PingLossPct   float64 `json:"pingLossPct" gorm:"comment:丢包率"`
	RXRateBps     int64   `json:"rxRateBps" gorm:"column:rx_rate_bps;comment:接收速率bps"`
	TXRateBps     int64   `json:"txRateBps" gorm:"column:tx_rate_bps;comment:发送速率bps"`
	CreateTime    int64   `json:"createTime" gorm:"index;comment:创建时间"`
}

func (DeviceHeartbeat) TableName() string { return "navmesh_device_heartbeats" }
