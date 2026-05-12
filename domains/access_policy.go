package domains

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
