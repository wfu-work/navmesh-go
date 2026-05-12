package domains

type CustomDomain struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Domain      string `json:"domain" gorm:"size:255;uniqueIndex;comment:自定义域名"`
	MappingGuid string `json:"mappingGuid" gorm:"size:64;index;comment:映射ID"`
	VerifyToken string `json:"verifyToken" gorm:"size:128;comment:校验Token"`
	Verified    bool   `json:"verified" gorm:"index;comment:是否已验证"`
	Status      int    `json:"status" gorm:"index;comment:状态"`
	CreateTime  int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime  int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (CustomDomain) TableName() string { return "navmesh_custom_domains" }
