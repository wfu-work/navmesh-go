package domains

const (
	DeviceTokenStatusDisabled = 0
	DeviceTokenStatusEnabled  = 1
)

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
