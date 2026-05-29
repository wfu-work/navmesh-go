package httpgateway

import (
	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
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

var accessLogQueue = make(chan mappingLogInput, 2048)

func init() {
	go func() {
		for input := range accessLogQueue {
			insertAccessLog(input)
		}
	}()
}

func writeAccessLog(input mappingLogInput) {
	select {
	case accessLogQueue <- input:
	default:
		global.NAV_LOG.Warn("drop http access log because queue is full")
	}
}

func insertAccessLog(input mappingLogInput) {
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
	if err := global.NAV_DB.Create(&row).Error; err != nil {
		global.NAV_LOG.Warn("write http access log failed", zap.Error(err))
	}
}
