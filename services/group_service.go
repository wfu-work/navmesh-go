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

type GroupService struct {
	commonServices.CrudService[domains.DeviceGroup]
}

func (s GroupService) WithDB(db *gorm.DB) GroupService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type SaveDeviceGroupRequest struct {
	Guid           string `json:"guid"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	Icon           string `json:"icon"`
	DefaultWebPort int    `json:"defaultWebPort"`
	DefaultDomain  string `json:"defaultDomain"`
	Sort           int    `json:"sort"`
	Remark         string `json:"remark"`
	Status         *int   `json:"status"`
}

type AssignDeviceGroupRequest struct {
	GroupGuid string `json:"groupGuid"`
	Tags      string `json:"tags"`
}

func (s GroupService) List(params map[string]string) ([]domains.DeviceGroup, int64, error) {
	db := s.DB().Model(&domains.DeviceGroup{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("group_key LIKE ? OR guid LIKE ? OR name LIKE ? OR remark LIKE ? OR default_domain LIKE ?", like, like, like, like, like)
	}
	if statusParam, ok := params["status"]; ok && strings.TrimSpace(statusParam) != "" {
		status := utils.Str2Int(statusParam)
		db = db.Where("status = ?", status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if allParam(params) {
		var items []domains.DeviceGroup
		err := db.Order("sort ASC, update_time DESC").Find(&items).Error
		return items, total, err
	}
	page := utils.Str2Int(params["page"])
	size := utils.Str2Int(params["size"])
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	var items []domains.DeviceGroup
	err := db.Order("sort ASC, update_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (s GroupService) Save(req SaveDeviceGroupRequest) (*domains.DeviceGroup, error) {
	req.Guid = strings.TrimSpace(req.Guid)
	req.Key = strings.TrimSpace(req.Key)
	req.Name = strings.TrimSpace(req.Name)
	req.Icon = strings.TrimSpace(req.Icon)
	req.DefaultDomain = strings.TrimSpace(req.DefaultDomain)
	req.Remark = strings.TrimSpace(req.Remark)
	if req.Key == "" {
		req.Key = req.Guid
	}
	if req.Key == "" {
		return nil, errors.New("key required")
	}
	if req.Name == "" {
		req.Name = req.Key
	}
	if err := s.ensureKeyAvailable(req.Key, req.Guid); err != nil {
		return nil, err
	}
	status := int(domains.StatusEnabled)
	if req.Status != nil {
		status = *req.Status
	}
	now := domains.NowMilli()
	var row domains.DeviceGroup
	var err error
	if req.Guid != "" {
		err = s.DB().Where("guid = ?", req.Guid).First(&row).Error
	} else {
		err = s.DB().Where("group_key = ? OR guid = ?", req.Key, req.Key).First(&row).Error
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.DeviceGroup{BaseDataEntity: commonDomains.BaseDataEntity{Guid: req.Key, CreateTime: now}}
	} else if row.Key != "" && row.Key != req.Key {
		return nil, errors.New("key cannot be modified")
	}
	row.Key = req.Key
	row.Name = req.Name
	row.Icon = req.Icon
	row.DefaultWebPort = req.DefaultWebPort
	row.DefaultDomain = req.DefaultDomain
	row.Sort = req.Sort
	row.Remark = req.Remark
	row.Status = status
	row.UpdateTime = now
	return &row, s.DB().Save(&row).Error
}

func (s GroupService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return s.DB().Model(&domains.DeviceGroup{}).Where("guid = ? OR group_key = ?", guid, guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func (s GroupService) AssignDevice(deviceGuid string, req AssignDeviceGroupRequest) error {
	deviceGuid = strings.TrimSpace(deviceGuid)
	req.GroupGuid = strings.TrimSpace(req.GroupGuid)
	req.Tags = normalizeTags(req.Tags)
	if deviceGuid == "" {
		return errors.New("deviceGuid required")
	}
	if req.GroupGuid != "" {
		group, err := s.GetEnabled(req.GroupGuid)
		if err != nil {
			return err
		}
		req.GroupGuid = group.Key
	}
	return s.DB().Model(&domains.Device{}).Where("guid = ?", deviceGuid).Updates(map[string]any{
		"group_guid":  req.GroupGuid,
		"device_type": req.GroupGuid,
		"tags":        req.Tags,
		"update_time": domains.NowMilli(),
	}).Error
}

func (s GroupService) GetEnabled(guid string) (*domains.DeviceGroup, error) {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return nil, errors.New("device group required")
	}
	var row domains.DeviceGroup
	if err := s.DB().Where("(group_key = ? OR guid = ?) AND status = ?", guid, guid, int(domains.StatusEnabled)).First(&row).Error; err != nil {
		return nil, errors.New("device group not found")
	}
	if row.Key == "" {
		row.Key = row.Guid
	}
	return &row, nil
}

func (s GroupService) SeedDefaults() {
	now := domains.NowMilli()
	s.backfillGroupKeys()
	for _, item := range defaultDeviceGroups {
		var count int64
		if item.Key == "" {
			item.Key = item.Guid
		}
		if err := s.DB().Model(&domains.DeviceGroup{}).Where("group_key = ? OR guid = ?", item.Key, item.Guid).Count(&count).Error; err != nil || count > 0 {
			continue
		}
		item.CreateTime = now
		item.UpdateTime = now
		_ = s.Create(item)
	}
	s.backfillGroupIcons()
}

func (s GroupService) backfillGroupKeys() {
	_ = s.DB().Model(&domains.DeviceGroup{}).Where("group_key = '' OR group_key IS NULL").Update("group_key", gorm.Expr("guid")).Error
}

func (s GroupService) backfillGroupIcons() {
	now := domains.NowMilli()
	for _, item := range defaultDeviceGroups {
		if strings.TrimSpace(item.Icon) == "" {
			continue
		}
		_ = s.DB().Model(&domains.DeviceGroup{}).
			Where("(group_key = ? OR guid = ?) AND (icon = '' OR icon IS NULL)", item.Key, item.Guid).
			Updates(map[string]any{"icon": item.Icon, "update_time": now}).Error
	}
}

func (s GroupService) ensureKeyAvailable(key string, currentGuid string) error {
	var existing domains.DeviceGroup
	err := s.DB().Where("group_key = ?", key).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if currentGuid != "" && existing.Guid == currentGuid {
		return nil
	}
	return errors.New("key already exists")
}

func allParam(params map[string]string) bool {
	value := strings.ToLower(strings.TrimSpace(utils.FirstNonEmpty(params["all"], params["noPage"])))
	return value == "1" || value == "true" || value == "yes"
}

func normalizeTags(tags string) string {
	parts := strings.Split(tags, ",")
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return strings.Join(out, ",")
}
