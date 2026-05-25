package domains

type TunnelSession struct {
	ID               uint   `json:"id" gorm:"primaryKey"`
	Guid             string `json:"guid" gorm:"size:64;uniqueIndex;comment:会话ID"`
	DeviceGuid       string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	SessionType      string `json:"sessionType" gorm:"size:32;index;comment:会话类型"`
	SourceIP         string `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	Username         string `json:"username" gorm:"size:128;comment:目标用户名"`
	TargetHost       string `json:"targetHost" gorm:"size:64;comment:目标主机"`
	TargetPort       int    `json:"targetPort" gorm:"comment:目标端口"`
	PublicHost       string `json:"publicHost" gorm:"size:255;index;comment:访问域名"`
	Status           int    `json:"status" gorm:"index;comment:状态"`
	ForceClosed      bool   `json:"forceClosed" gorm:"comment:是否由管理端断开"`
	BytesIn          int64  `json:"bytesIn" gorm:"comment:入站字节"`
	BytesOut         int64  `json:"bytesOut" gorm:"comment:出站字节"`
	StartTime        int64  `json:"startTime" gorm:"index;comment:开始时间"`
	EndTime          int64  `json:"endTime" gorm:"index;comment:结束时间"`
	DisconnectReason string `json:"disconnectReason" gorm:"size:255;comment:断开原因"`
	CreateTime       int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime       int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (TunnelSession) TableName() string { return "navmesh_tunnel_sessions" }
