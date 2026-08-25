package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
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
	return queryPage[domains.AccessPolicy](db, params, DefaultMaxPageSize, "update_time DESC")
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
	if err := s.DB().Save(&row).Error; err != nil {
		return nil, err
	}
	triggerHTTPRouteReload()
	return &row, nil
}

func (s AccessPolicyService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	err := s.DB().Model(&domains.AccessPolicy{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
	if err == nil {
		triggerHTTPRouteReload()
	}
	return err
}

func (s AccessPolicyService) IsAllowed(deviceGuid, mappingGuid, protocol string) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	deviceGuid = strings.TrimSpace(deviceGuid)
	mappingGuid = strings.TrimSpace(mappingGuid)
	deviceContext := loadPolicyDeviceContext(s.DB(), deviceGuid)
	var policies []domains.AccessPolicy
	query := s.DB().Where("status = ?", int(domains.StatusEnabled))
	if mappingGuid != "" {
		query = query.Where("scope = ? OR (scope = ? AND target_id = ?) OR (scope = ? AND target_id IN ?) OR (scope = ? AND target_id = ?)", "global", "device", deviceGuid, "group", deviceContext.targets(), "mapping", mappingGuid)
	} else {
		query = query.Where("scope = ? OR (scope = ? AND target_id = ?) OR (scope = ? AND target_id IN ?)", "global", "device", deviceGuid, "group", deviceContext.targets())
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

type policyDeviceContext struct {
	groupGuid  string
	deviceType string
}

func loadPolicyDeviceContext(db *gorm.DB, deviceGuid string) policyDeviceContext {
	if db == nil || strings.TrimSpace(deviceGuid) == "" {
		return policyDeviceContext{}
	}
	var device domains.Device
	if err := db.Select("group_guid", "device_type").Where("guid = ?", deviceGuid).First(&device).Error; err != nil {
		return policyDeviceContext{}
	}
	return policyDeviceContext{
		groupGuid:  strings.TrimSpace(device.GroupGuid),
		deviceType: strings.TrimSpace(device.DeviceType),
	}
}

func (c policyDeviceContext) targets() []string {
	values := make([]string, 0, 2)
	for _, value := range []string{c.groupGuid, c.deviceType} {
		if value == "" {
			continue
		}
		duplicate := false
		for _, existing := range values {
			if existing == value {
				duplicate = true
				break
			}
		}
		if !duplicate {
			values = append(values, value)
		}
	}
	// Keep the IN predicate valid when the device has no group metadata.
	if len(values) == 0 {
		return []string{""}
	}
	return values
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
