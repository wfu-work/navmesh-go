package domains

type DeviceGroup struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	Guid       string `json:"guid" gorm:"size:64;uniqueIndex;comment:分组ID"`
	Name       string `json:"name" gorm:"size:128;uniqueIndex;comment:分组名称"`
	Remark     string `json:"remark" gorm:"size:512;comment:备注"`
	CreateTime int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (DeviceGroup) TableName() string { return "navmesh_device_groups" }
