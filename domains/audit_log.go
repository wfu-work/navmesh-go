package domains

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
