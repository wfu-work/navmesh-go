package domains

type Event struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	Guid       string `json:"guid" gorm:"size:64;uniqueIndex;comment:事件ID"`
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	EventType  string `json:"eventType" gorm:"size:64;index;comment:事件类型"`
	Level      string `json:"level" gorm:"size:32;index;comment:级别"`
	Title      string `json:"title" gorm:"size:255;comment:标题"`
	Message    string `json:"message" gorm:"type:text;comment:内容"`
	Status     int    `json:"status" gorm:"index;comment:状态"`
	CreateTime int64  `json:"createTime" gorm:"index;comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (Event) TableName() string { return "navmesh_events" }
