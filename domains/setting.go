package domains

type Setting struct {
	Key        string `json:"key" gorm:"primaryKey;size:128;comment:配置键"`
	Value      string `json:"value" gorm:"type:text;comment:配置值"`
	CreateTime int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (Setting) TableName() string { return "navmesh_settings" }
