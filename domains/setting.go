package domains

type Setting struct {
	Key        string `json:"key" gorm:"primaryKey;size:128;comment:配置键"`
	Value      string `json:"value" gorm:"type:text;comment:配置值"`
	CreateTime int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (Setting) TableName() string { return "navmesh_settings" }

type AuditLog struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	Actor      string `json:"actor" gorm:"size:128;index;comment:操作者"`
	Action     string `json:"action" gorm:"size:128;index;comment:动作"`
	Resource   string `json:"resource" gorm:"size:128;index;comment:资源"`
	ResourceID string `json:"resourceId" gorm:"size:128;index;comment:资源ID"`
	Message    string `json:"message" gorm:"type:text;comment:说明"`
	SourceIP   string `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	CreateTime int64  `json:"createTime" gorm:"index;comment:创建时间"`
}

func (AuditLog) TableName() string { return "navmesh_audit_logs" }

type AccessPolicy struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	Guid       string `json:"guid" gorm:"size:64;uniqueIndex;comment:策略ID"`
	Name       string `json:"name" gorm:"size:128;comment:名称"`
	Scope      string `json:"scope" gorm:"size:32;index;comment:策略范围"`
	TargetID   string `json:"targetId" gorm:"size:128;index;comment:目标ID"`
	AllowSSH   bool   `json:"allowSsh" gorm:"comment:允许SSH"`
	AllowHTTP  bool   `json:"allowHttp" gorm:"comment:允许HTTP"`
	Status     int    `json:"status" gorm:"index;comment:状态"`
	CreateTime int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (AccessPolicy) TableName() string { return "navmesh_access_policies" }

type DeviceGroup struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	Guid       string `json:"guid" gorm:"size:64;uniqueIndex;comment:分组ID"`
	Name       string `json:"name" gorm:"size:128;uniqueIndex;comment:分组名称"`
	Remark     string `json:"remark" gorm:"size:512;comment:备注"`
	CreateTime int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (DeviceGroup) TableName() string { return "navmesh_device_groups" }
