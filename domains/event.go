package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

type Event struct {
	commonDomains.BaseDataEntity
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;index:idx_event_open,priority:1;comment:设备ID"`
	EventType  string `json:"eventType" gorm:"size:64;index;index:idx_event_open,priority:2;comment:事件类型"`
	Level      string `json:"level" gorm:"size:32;index;comment:级别"`
	Title      string `json:"title" gorm:"size:255;comment:标题"`
	Message    string `json:"message" gorm:"type:text;comment:内容"`
	Status     int    `json:"status" gorm:"index;index:idx_event_open,priority:3;comment:状态"`
}

func (Event) TableName() string { return "navmesh_events" }

func (s Event) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
