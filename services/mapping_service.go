package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/google/uuid"
	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/gorm"
)

type MappingService struct{}

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
	db := global.NAV_DB.Model(&domains.PortMapping{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR public_host LIKE ? OR device_guid LIKE ?", like, like, like)
	}
	if status := utils.Str2Int(params["status"]); status > 0 {
		db = db.Where("status = ?", status)
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
	err := global.NAV_DB.Where("guid = ?", req.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.PortMapping{Guid: uuid.NewString(), CreateTime: now}
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
	if err := global.NAV_DB.Save(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s MappingService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return global.NAV_DB.Model(&domains.PortMapping{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func (s MappingService) AccessLogs(params map[string]string) ([]domains.HTTPAccessLog, int64, error) {
	db := global.NAV_DB.Model(&domains.HTTPAccessLog{})
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
	err := global.NAV_DB.Where("public_host = ?", publicHost).First(&existing).Error
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
