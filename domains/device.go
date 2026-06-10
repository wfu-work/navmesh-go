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
