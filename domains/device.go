package domains

const (
	DeviceStatusRegistered = 1
	DeviceStatusOnline     = 2
	DeviceStatusOffline    = 3
	DeviceStatusDisabled   = 4
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
	GroupGuid     string `json:"groupGuid" gorm:"size:64;index;comment:设备分组ID"`
	Tags          string `json:"tags" gorm:"size:512;comment:设备标签，逗号分隔"`
	Status        int    `json:"status" gorm:"index;comment:状态"`
	LastSeenTime  int64  `json:"lastSeenTime" gorm:"index;comment:最后在线时间"`
	CreateTime    int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime    int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (Device) TableName() string { return "navmesh_devices" }
