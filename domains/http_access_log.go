package domains

type HttpAccessLog struct {
	ID           uint   `json:"id" gorm:"primaryKey;index:idx_http_log_mapping_time_id,priority:3,sort:desc;index:idx_http_log_device_time_id,priority:3,sort:desc;index:idx_http_log_host_time_id,priority:3,sort:desc"`
	MappingGuid  string `json:"mappingGuid" gorm:"size:64;index:idx_http_log_mapping_time_id,priority:1;comment:映射ID"`
	DeviceGuid   string `json:"deviceGuid" gorm:"size:64;index:idx_http_log_device_time_id,priority:1;comment:设备ID"`
	Host         string `json:"host" gorm:"size:255;index:idx_http_log_host_time_id,priority:1;comment:Host"`
	Method       string `json:"method" gorm:"size:16;comment:HTTP方法"`
	Path         string `json:"path" gorm:"size:1024;comment:路径"`
	SourceIP     string `json:"sourceIp" gorm:"size:64;comment:来源IP"`
	StatusCode   int    `json:"statusCode" gorm:"comment:状态码"`
	DurationMs   int64  `json:"durationMs" gorm:"comment:耗时"`
	TunnelOpenMs int64  `json:"tunnelOpenMs" gorm:"comment:打开隧道耗时"`
	UpstreamMs   int64  `json:"upstreamMs" gorm:"comment:上游响应头耗时"`
	FirstByteMs  int64  `json:"firstByteMs" gorm:"comment:首字节耗时"`
	ReusedConn   bool   `json:"reusedConn" gorm:"comment:是否复用连接"`
	BytesIn      int64  `json:"bytesIn" gorm:"comment:入站字节"`
	BytesOut     int64  `json:"bytesOut" gorm:"comment:出站字节"`
	ErrorMessage string `json:"errorMessage" gorm:"size:512;comment:错误信息"`
	CreateTime   int64  `json:"createTime" gorm:"index;index:idx_http_log_mapping_time_id,priority:2,sort:desc;index:idx_http_log_device_time_id,priority:2,sort:desc;index:idx_http_log_host_time_id,priority:2,sort:desc;comment:创建时间"`
}

func (HttpAccessLog) TableName() string { return "navmesh_http_access_logs" }

// HTTPAccessLog is kept as a compatibility alias for external callers.
// Deprecated: use HttpAccessLog.
type HTTPAccessLog = HttpAccessLog
