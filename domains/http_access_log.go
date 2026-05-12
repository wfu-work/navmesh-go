package domains

type HTTPAccessLog struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	MappingGuid  string `json:"mappingGuid" gorm:"size:64;index;comment:映射ID"`
	DeviceGuid   string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	Host         string `json:"host" gorm:"size:255;index;comment:Host"`
	Method       string `json:"method" gorm:"size:16;comment:HTTP方法"`
	Path         string `json:"path" gorm:"size:1024;comment:路径"`
	SourceIP     string `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	StatusCode   int    `json:"statusCode" gorm:"comment:状态码"`
	DurationMs   int64  `json:"durationMs" gorm:"comment:耗时"`
	BytesIn      int64  `json:"bytesIn" gorm:"comment:入站字节"`
	BytesOut     int64  `json:"bytesOut" gorm:"comment:出站字节"`
	ErrorMessage string `json:"errorMessage" gorm:"size:512;comment:错误信息"`
	CreateTime   int64  `json:"createTime" gorm:"index;comment:创建时间"`
}

func (HTTPAccessLog) TableName() string { return "navmesh_http_access_logs" }
