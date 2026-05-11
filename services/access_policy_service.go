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

type AccessPolicyService struct{}

type SaveAccessPolicyRequest struct {
	Guid      string `json:"guid"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	TargetID  string `json:"targetId"`
	AllowSSH  bool   `json:"allowSsh"`
	AllowHTTP bool   `json:"allowHttp"`
	Status    int    `json:"status"`
}

func (s AccessPolicyService) List(params map[string]string) ([]domains.AccessPolicy, int64, error) {
	db := global.NAV_DB.Model(&domains.AccessPolicy{})
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
	err := global.NAV_DB.Where("guid = ?", req.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.AccessPolicy{Guid: uuid.NewString(), CreateTime: now}
		if req.Guid != "" {
			row.Guid = req.Guid
		}
	}
	row.Name = req.Name
	row.Scope = req.Scope
	row.TargetID = req.TargetID
	row.AllowSSH = req.AllowSSH
	row.AllowHTTP = req.AllowHTTP
	row.Status = req.Status
	row.UpdateTime = now
	return &row, global.NAV_DB.Save(&row).Error
}

func (s AccessPolicyService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return global.NAV_DB.Model(&domains.AccessPolicy{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func (s AccessPolicyService) IsAllowed(deviceGuid, mappingGuid, protocol string) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	var policies []domains.AccessPolicy
	deviceGuid = strings.TrimSpace(deviceGuid)
	mappingGuid = strings.TrimSpace(mappingGuid)
	query := global.NAV_DB.Where("status = ?", int(domains.StatusEnabled))
	if mappingGuid != "" {
		query = query.Where("scope = ? OR (scope = ? AND target_id = ?) OR (scope = ? AND target_id = ?)", "global", "device", deviceGuid, "mapping", mappingGuid)
	} else {
		query = query.Where("scope = ? OR (scope = ? AND target_id = ?)", "global", "device", deviceGuid)
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
		}
	}
	return allowed
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
