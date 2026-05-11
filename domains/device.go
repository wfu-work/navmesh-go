package domains

const (
	DeviceStatusRegistered = 1
	DeviceStatusOnline     = 2
	DeviceStatusOffline    = 3
	DeviceStatusDisabled   = 4

	DeviceTokenStatusDisabled = 0
	DeviceTokenStatusEnabled  = 1
)

type Device struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	Guid          string `json:"guid" gorm:"size:64;uniqueIndex;comment:设备唯一ID"`
	SnCode        string `json:"sncode" gorm:"size:128;uniqueIndex;comment:设备序列号"`
	DeviceID      string `json:"deviceId" gorm:"size:128;index;comment:业务系统设备ID"`
	DeviceType    string `json:"deviceType" gorm:"size:64;index;comment:设备类型"`
	Alias         string `json:"alias" gorm:"size:128;uniqueIndex;comment:全局唯一设备别名"`
	Remark        string `json:"remark" gorm:"size:512;comment:中文备注"`
	Hostname      string `json:"hostname" gorm:"size:255;comment:主机名"`
	HostIP        string `json:"hostIp" gorm:"size:64;comment:设备本机IP"`
	ClientVersion string `json:"clientVersion" gorm:"size:64;comment:客户端版本"`
	SourceIP      string `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	SSHPort       int    `json:"sshPort" gorm:"comment:本机SSH端口"`
	WebPort       int    `json:"webPort" gorm:"comment:本机Web端口"`
	WebDomain     string `json:"webDomain" gorm:"size:255;comment:Web映射域名"`
	Status        int    `json:"status" gorm:"index;comment:状态"`
	LastSeenTime  int64  `json:"lastSeenTime" gorm:"index;comment:最后在线时间"`
	CreateTime    int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime    int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (Device) TableName() string { return "navmesh_devices" }

type DeviceToken struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	Guid       string `json:"guid" gorm:"size:64;uniqueIndex;comment:Token ID"`
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	TokenHash  string `json:"-" gorm:"size:255;comment:Token Hash"`
	Name       string `json:"name" gorm:"size:128;comment:名称"`
	Status     int    `json:"status" gorm:"index;comment:状态"`
	ExpireTime int64  `json:"expireTime" gorm:"comment:过期时间"`
	CreateTime int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (DeviceToken) TableName() string { return "navmesh_device_tokens" }

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

type DeviceHeartbeat struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	SourceIP   string `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	HostIP     string `json:"hostIp" gorm:"size:64;comment:设备本机IP"`
	CreateTime int64  `json:"createTime" gorm:"index;comment:创建时间"`
}

func (DeviceHeartbeat) TableName() string { return "navmesh_device_heartbeats" }
