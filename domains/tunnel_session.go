package domains

type TunnelSession struct {
	ID               uint   `json:"id" gorm:"primaryKey;index:idx_session_device_time_id,priority:3,sort:desc;index:idx_session_device_status_time_id,priority:4,sort:desc;index:idx_session_type_time_id,priority:3,sort:desc;index:idx_session_public_time_id,priority:3,sort:desc;index:idx_session_status_time_id,priority:3,sort:desc;index:idx_session_status_end_time,priority:3"`
	Guid             string `json:"guid" gorm:"size:64;uniqueIndex;comment:会话ID"`
	DeviceGuid       string `json:"deviceGuid" gorm:"size:64;index:idx_session_device_time_id,priority:1;index:idx_session_device_status_time_id,priority:1;comment:设备ID"`
	SessionType      string `json:"sessionType" gorm:"size:32;index:idx_session_type_time_id,priority:1;comment:会话类型"`
	SourceIP         string `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	Username         string `json:"username" gorm:"size:128;comment:目标用户名"`
	TargetHost       string `json:"targetHost" gorm:"size:64;comment:目标主机"`
	TargetPort       int    `json:"targetPort" gorm:"comment:目标端口"`
	PublicHost       string `json:"publicHost" gorm:"size:255;index:idx_session_public_time_id,priority:1;comment:访问域名"`
	Status           int    `json:"status" gorm:"index:idx_session_device_status_time_id,priority:2;index:idx_session_status_time_id,priority:1;index:idx_session_status_end_time,priority:1;comment:状态"`
	ForceClosed      bool   `json:"forceClosed" gorm:"comment:是否由管理端断开"`
	BytesIn          int64  `json:"bytesIn" gorm:"comment:入站字节"`
	BytesOut         int64  `json:"bytesOut" gorm:"comment:出站字节"`
	StartTime        int64  `json:"startTime" gorm:"index;index:idx_session_device_time_id,priority:2,sort:desc;index:idx_session_device_status_time_id,priority:3,sort:desc;index:idx_session_type_time_id,priority:2,sort:desc;index:idx_session_public_time_id,priority:2,sort:desc;index:idx_session_status_time_id,priority:2,sort:desc;comment:开始时间"`
	EndTime          int64  `json:"endTime" gorm:"index;index:idx_session_status_end_time,priority:2;comment:结束时间"`
	DisconnectReason string `json:"disconnectReason" gorm:"size:255;comment:断开原因"`
	CreateTime       int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime       int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (TunnelSession) TableName() string { return "navmesh_tunnel_sessions" }
