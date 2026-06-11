package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/global"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

type AccessPolicyService struct {
	commonServices.CrudService[domains.AccessPolicy]
}

func (s AccessPolicyService) WithDB(db *gorm.DB) AccessPolicyService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type SaveAccessPolicyRequest struct {
	Guid      string `json:"guid"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	TargetID  string `json:"targetId"`
	AllowSSH  bool   `json:"allowSsh"`
	AllowHTTP bool   `json:"allowHttp"`
	AllowTCP  bool   `json:"allowTcp"`
	Status    int    `json:"status"`
}

func (s AccessPolicyService) List(params map[string]string) ([]domains.AccessPolicy, int64, error) {
	db := s.DB().Model(&domains.AccessPolicy{})
	if scope := strings.TrimSpace(params["scope"]); scope != "" {
		db = db.Where("scope = ?", scope)
	}
	if targetID := strings.TrimSpace(params["targetId"]); targetID != "" {
		db = db.Where("target_id = ?", targetID)
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
	var items []domains.AccessPolicy
	err := db.Order("update_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (s AccessPolicyService) Save(req SaveAccessPolicyRequest) (*domains.AccessPolicy, error) {
	req = normalizePolicyRequest(req)
	if req.Name == "" {
		return nil, errors.New("name required")
	}
	if req.Scope == "" {
		return nil, errors.New("scope required")
	}
	if req.Scope != "global" && req.TargetID == "" {
		return nil, errors.New("targetId required")
	}
	now := domains.NowMilli()
	var row domains.AccessPolicy
	err := s.DB().Where("guid = ?", req.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.AccessPolicy{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}}
		if req.Guid != "" {
			row.Guid = req.Guid
		}
	}
	row.Name = req.Name
	row.Scope = req.Scope
	row.TargetID = req.TargetID
	row.AllowSSH = req.AllowSSH
	row.AllowHTTP = req.AllowHTTP
	row.AllowTCP = req.AllowTCP
	row.Status = req.Status
	row.UpdateTime = now
	return &row, s.DB().Save(&row).Error
}

func (s AccessPolicyService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return s.DB().Model(&domains.AccessPolicy{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func (s AccessPolicyService) IsAllowed(deviceGuid, mappingGuid, protocol string) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	var policies []domains.AccessPolicy
	deviceGuid = strings.TrimSpace(deviceGuid)
	mappingGuid = strings.TrimSpace(mappingGuid)
	query := s.DB().Where("status = ?", int(domains.StatusEnabled))
	if mappingGuid != "" {
		groupGuid := deviceGroupGuid(deviceGuid)
		deviceType := deviceTypeGroup(deviceGuid)
		query = query.Where("scope = ? OR (scope = ? AND target_id = ?) OR (scope = ? AND target_id IN ?) OR (scope = ? AND target_id = ?)", "global", "device", deviceGuid, "group", []string{groupGuid, deviceType}, "mapping", mappingGuid)
	} else {
		groupGuid := deviceGroupGuid(deviceGuid)
		deviceType := deviceTypeGroup(deviceGuid)
		query = query.Where("scope = ? OR (scope = ? AND target_id = ?) OR (scope = ? AND target_id IN ?)", "global", "device", deviceGuid, "group", []string{groupGuid, deviceType})
	}
	if err := query.Find(&policies).Error; err != nil || len(policies) == 0 {
		return true
	}
	allowed := true
	for _, policy := range policies {
		switch protocol {
		case "ssh":
			allowed = allowed && policy.AllowSSH
		case "http", "https":
			allowed = allowed && policy.AllowHTTP
		case "tcp":
			allowed = allowed && policy.AllowTCP
		}
	}
	return allowed
}

func deviceGroupGuid(deviceGuid string) string {
	var device domains.Device
	if err := global.NAV_DB.Select("group_guid").Where("guid = ?", strings.TrimSpace(deviceGuid)).First(&device).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(device.GroupGuid)
}

func deviceTypeGroup(deviceGuid string) string {
	var device domains.Device
	if err := global.NAV_DB.Select("device_type").Where("guid = ?", strings.TrimSpace(deviceGuid)).First(&device).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(device.DeviceType)
}

func normalizePolicyRequest(req SaveAccessPolicyRequest) SaveAccessPolicyRequest {
	req.Guid = strings.TrimSpace(req.Guid)
	req.Name = strings.TrimSpace(req.Name)
	req.Scope = strings.ToLower(strings.TrimSpace(req.Scope))
	req.TargetID = strings.TrimSpace(req.TargetID)
	if req.Scope == "" {
		req.Scope = "device"
	}
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	return req
}
