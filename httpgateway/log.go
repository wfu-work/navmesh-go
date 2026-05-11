package httpgateway

import (
	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
)

type mappingLogInput struct {
	Mapping      domains.PortMapping
	Device       domains.Device
	Host         string
	Method       string
	Path         string
	SourceIP     string
	StatusCode   int
	DurationMs   int64
	BytesIn      int64
	BytesOut     int64
	ErrorMessage string
}

func writeAccessLog(input mappingLogInput) {
	row := domains.HTTPAccessLog{
		MappingGuid:  input.Mapping.Guid,
		DeviceGuid:   input.Device.Guid,
		Host:         input.Host,
		Method:       input.Method,
		Path:         input.Path,
		SourceIP:     input.SourceIP,
		StatusCode:   input.StatusCode,
		DurationMs:   input.DurationMs,
		BytesIn:      input.BytesIn,
		BytesOut:     input.BytesOut,
		ErrorMessage: input.ErrorMessage,
		CreateTime:   domains.NowMilli(),
	}
	_ = global.NAV_DB.Create(&row).Error
}
