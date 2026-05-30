package services

import (
	"errors"
	"strconv"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

type MappingService struct {
	commonServices.CrudService[domains.PortMapping]
}

func (s MappingService) WithDB(db *gorm.DB) MappingService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type SavePortMappingRequest struct {
	Guid           string `json:"guid"`
	DeviceGuid     string `json:"deviceGuid"`
	Name           string `json:"name"`
	PublicHost     string `json:"publicHost"`
	TargetHost     string `json:"targetHost"`
	TargetPort     int    `json:"targetPort"`
	Protocol       string `json:"protocol"`
	IsCustomDomain bool   `json:"isCustomDomain"`
	Status         int    `json:"status"`
}

func (s MappingService) List(params map[string]string) ([]domains.PortMapping, int64, error) {
	db := s.DB().Model(&domains.PortMapping{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR public_host LIKE ? OR device_guid LIKE ?", like, like, like)
	}
	if status := utils.Str2Int(params["status"]); status > 0 {
		db = db.Where("status = ?", status)
	}
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := utils.Str2Int(params["page"])
	size := utils.Str2Int(params["size"])
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	var items []domains.PortMapping
	err := db.Order("update_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (s MappingService) Save(req SavePortMappingRequest) (*domains.PortMapping, error) {
	req = normalizeMappingRequest(req)
	if req.DeviceGuid == "" {
		return nil, errors.New("deviceGuid required")
	}
	if req.PublicHost == "" {
		return nil, errors.New("publicHost required")
	}
	if req.TargetHost == "" {
		return nil, errors.New("targetHost required")
	}
	if req.TargetPort <= 0 {
		return nil, errors.New("targetPort required")
	}
	if err := ensureDeviceExists(req.DeviceGuid); err != nil {
		return nil, err
	}
	if err := ensurePublicHostAvailable(req.PublicHost, req.Guid); err != nil {
		return nil, err
	}
	now := domains.NowMilli()
	var row domains.PortMapping
	err := s.DB().Where("guid = ?", req.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.PortMapping{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}}
		if req.Guid != "" {
			row.Guid = req.Guid
		}
	}
	row.DeviceGuid = req.DeviceGuid
	row.Name = req.Name
	row.PublicHost = req.PublicHost
	row.TargetHost = req.TargetHost
	row.TargetPort = req.TargetPort
	row.Protocol = req.Protocol
	row.IsCustomDomain = req.IsCustomDomain
	row.Status = req.Status
	row.UpdateTime = now
	if err := s.DB().Save(&row).Error; err != nil {
		return nil, err
	}
	if row.IsCustomDomain {
		if _, err := s.customDomainService().EnsureForMapping(row); err != nil {
			return nil, err
		}
	}
	return &row, nil
}

func (s MappingService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return s.DB().Model(&domains.PortMapping{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func (s MappingService) AccessLogs(params map[string]string) ([]domains.HTTPAccessLog, int64, error) {
	db := s.DB().Model(&domains.HTTPAccessLog{})
	if mappingGuid := strings.TrimSpace(params["mappingGuid"]); mappingGuid != "" {
		db = db.Where("mapping_guid = ?", mappingGuid)
	}
	if host := strings.TrimSpace(params["host"]); host != "" {
		db = db.Where("host = ?", host)
	}
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	if method := strings.TrimSpace(params["method"]); method != "" {
		db = db.Where("method = ?", strings.ToUpper(method))
	}
	if path := strings.TrimSpace(params["path"]); path != "" {
		db = db.Where("path LIKE ?", "%"+path+"%")
	}
	if statusCode := utils.Str2Int(params["statusCode"]); statusCode > 0 {
		db = db.Where("status_code = ?", statusCode)
	}
	if minDurationMs := queryInt64(params, "minDurationMs"); minDurationMs > 0 {
		db = db.Where("duration_ms >= ?", minDurationMs)
	}
	if minTunnelOpenMs := queryInt64(params, "minTunnelOpenMs"); minTunnelOpenMs > 0 {
		db = db.Where("tunnel_open_ms >= ?", minTunnelOpenMs)
	}
	if minUpstreamMs := queryInt64(params, "minUpstreamMs"); minUpstreamMs > 0 {
		db = db.Where("upstream_ms >= ?", minUpstreamMs)
	}
	if minFirstByteMs := queryInt64(params, "minFirstByteMs"); minFirstByteMs > 0 {
		db = db.Where("first_byte_ms >= ?", minFirstByteMs)
	}
	if reusedConn, ok := queryBool(params, "reusedConn"); ok {
		db = db.Where("reused_conn = ?", reusedConn)
	}
	if hasError, ok := queryBool(params, "hasError"); ok {
		if hasError {
			db = db.Where("error_message <> ''")
		} else {
			db = db.Where("error_message = ''")
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := utils.Str2Int(params["page"])
	size := utils.Str2Int(params["size"])
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	var items []domains.HTTPAccessLog
	err := db.Order("create_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func queryInt64(params map[string]string, key string) int64 {
	value := strings.TrimSpace(params[key])
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func queryBool(params map[string]string, key string) (bool, bool) {
	value := strings.ToLower(strings.TrimSpace(params[key]))
	switch value {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func (s MappingService) customDomainService() CustomDomainService {
	return ServiceGroupApp.CustomDomainService.WithDB(s.DB())
}

func normalizeMappingRequest(req SavePortMappingRequest) SavePortMappingRequest {
	req.Guid = strings.TrimSpace(req.Guid)
	req.DeviceGuid = strings.TrimSpace(req.DeviceGuid)
	req.Name = strings.TrimSpace(req.Name)
	req.PublicHost = normalizeHost(req.PublicHost)
	req.TargetHost = strings.TrimSpace(req.TargetHost)
	req.Protocol = strings.TrimSpace(req.Protocol)
	if req.TargetHost == "" {
		req.TargetHost = "127.0.0.1"
	}
	if req.Protocol == "" {
		req.Protocol = "http"
	}
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	return req
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if i := strings.Index(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func ensurePublicHostAvailable(publicHost, currentGuid string) error {
	var existing domains.PortMapping
	err := ServiceGroupApp.MappingService.DB().Where("public_host = ?", publicHost).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.Guid != currentGuid {
		return errors.New("publicHost already exists")
	}
	return nil
}
