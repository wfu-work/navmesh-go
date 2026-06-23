package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

const (
	DeviceStatusRegistered = 1
	DeviceStatusOnline     = 2
	DeviceStatusOffline    = 3
	DeviceStatusDisabled   = 4
)

type Device struct {
	commonDomains.BaseDataEntity
	Sncode                string  `json:"sncode" gorm:"column:sn_code;size:128;uniqueIndex;comment:设备序列号"`
	DeviceType            string  `json:"deviceType" gorm:"size:64;index;comment:设备类型"`
	Alias                 string  `json:"alias" gorm:"size:128;uniqueIndex;comment:全局唯一设备别名"`
	Remark                string  `json:"remark" gorm:"size:512;comment:中文备注"`
	Hostname              string  `json:"hostname" gorm:"size:255;comment:主机名"`
	HostIP                string  `json:"hostIp" gorm:"size:64;comment:设备本机IP"`
	WanIP                 string  `json:"wanIp" gorm:"size:64;comment:设备外网IP"`
	Location              string  `json:"location" gorm:"size:255;comment:设备区域位置"`
	ClientVersion         string  `json:"clientVersion" gorm:"size:64;comment:客户端版本"`
	SourceIP              string  `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	SSHPort               int     `json:"sshPort" gorm:"comment:本机SSH端口"`
	WebPort               int     `json:"webPort" gorm:"comment:本机Web端口"`
	WebDomain             string  `json:"webDomain" gorm:"size:255;comment:Web映射域名"`
	OS                    string  `json:"os" gorm:"size:64;comment:操作系统"`
	OSVersion             string  `json:"osVersion" gorm:"size:128;comment:系统版本"`
	Kernel                string  `json:"kernel" gorm:"size:128;comment:内核版本"`
	Arch                  string  `json:"arch" gorm:"size:64;comment:CPU架构"`
	MemoryTotal           int64   `json:"memoryTotal" gorm:"comment:内存总量字节"`
	MemoryUsed            int64   `json:"memoryUsed" gorm:"comment:内存已用字节"`
	MemoryFree            int64   `json:"memoryFree" gorm:"comment:内存空闲字节"`
	DiskTotal             int64   `json:"diskTotal" gorm:"comment:磁盘总量字节"`
	DiskUsed              int64   `json:"diskUsed" gorm:"comment:磁盘已用字节"`
	DiskFree              int64   `json:"diskFree" gorm:"comment:磁盘空闲字节"`
	DiskUsedPct           float64 `json:"diskUsedPct" gorm:"comment:磁盘使用率"`
	NetworkType           string  `json:"networkType" gorm:"size:32;index;comment:网络类型"`
	NetworkIface          string  `json:"networkIface" gorm:"size:64;comment:网络接口"`
	SignalDBM             int     `json:"signalDbm" gorm:"column:signal_dbm;comment:信号强度dBm"`
	SignalPct             int     `json:"signalPct" gorm:"comment:信号强度百分比"`
	CellularRSRP          int     `json:"cellularRsrp" gorm:"column:cellular_rsrp;comment:4G/5G RSRP"`
	CellularRSRQ          int     `json:"cellularRsrq" gorm:"column:cellular_rsrq;comment:4G/5G RSRQ"`
	CellularSINR          int     `json:"cellularSinr" gorm:"column:cellular_sinr;comment:4G/5G SINR"`
	WifiSSID              string  `json:"wifiSsid" gorm:"column:wifi_ssid;size:128;comment:WiFi SSID"`
	WifiRSSI              int     `json:"wifiRssi" gorm:"column:wifi_rssi;comment:WiFi RSSI"`
	PingTarget            string  `json:"pingTarget" gorm:"size:255;comment:延迟探测目标"`
	PingLatencyMs         int64   `json:"pingLatencyMs" gorm:"comment:延迟毫秒"`
	PingLossPct           float64 `json:"pingLossPct" gorm:"comment:丢包率"`
	RXRateBps             int64   `json:"rxRateBps" gorm:"column:rx_rate_bps;comment:接收速率bps"`
	TXRateBps             int64   `json:"txRateBps" gorm:"column:tx_rate_bps;comment:发送速率bps"`
	LastMetricAt          int64   `json:"lastMetricAt" gorm:"index;comment:最后指标上报时间"`
	GroupGuid             string  `json:"groupGuid" gorm:"size:64;index;comment:设备分组ID"`
	Tags                  string  `json:"tags" gorm:"size:512;comment:设备标签，逗号分隔"`
	Status                int     `json:"status" gorm:"index;comment:状态"`
	LastSeenTime          int64   `json:"lastSeenTime" gorm:"index;comment:最后在线时间"`
	VPNRestartRequestedAt int64   `json:"vpnRestartRequestedAt" gorm:"column:vpn_restart_requested_at;index;comment:VPN重启请求时间"`
	VPNRestartDeliveredAt int64   `json:"vpnRestartDeliveredAt" gorm:"column:vpn_restart_delivered_at;index;comment:VPN重启指令下发时间"`
}

func (Device) TableName() string { return "navmesh_devices" }

func (s Device) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
