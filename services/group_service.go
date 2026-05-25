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

type GroupService struct{}

type SaveDeviceGroupRequest struct {
	Guid   string `json:"guid"`
	Name   string `json:"name"`
	Remark string `json:"remark"`
	Status int    `json:"status"`
}

type AssignDeviceGroupRequest struct {
	GroupGuid string `json:"groupGuid"`
	Tags      string `json:"tags"`
}

func (s GroupService) List(params map[string]string) ([]domains.DeviceGroup, int64, error) {
	db := global.NAV_DB.Model(&domains.DeviceGroup{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		db = db.Where("name LIKE ? OR remark LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
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
	var items []domains.DeviceGroup
	err := db.Order("update_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (s GroupService) Save(req SaveDeviceGroupRequest) (*domains.DeviceGroup, error) {
	req.Guid = strings.TrimSpace(req.Guid)
	req.Name = strings.TrimSpace(req.Name)
	req.Remark = strings.TrimSpace(req.Remark)
	if req.Name == "" {
		return nil, errors.New("name required")
	}
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	now := domains.NowMilli()
	var row domains.DeviceGroup
	err := global.NAV_DB.Where("guid = ?", req.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.DeviceGroup{Guid: uuid.NewString(), CreateTime: now}
		if req.Guid != "" {
			row.Guid = req.Guid
		}
	}
	row.Name = req.Name
	row.Remark = req.Remark
	row.Status = req.Status
	row.UpdateTime = now
	return &row, global.NAV_DB.Save(&row).Error
}

func (s GroupService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return global.NAV_DB.Model(&domains.DeviceGroup{}).Where("guid = ?", guid).Updates(map[string]any{
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
		var count int64
		if err := global.NAV_DB.Model(&domains.DeviceGroup{}).Where("guid = ? AND status = ?", req.GroupGuid, int(domains.StatusEnabled)).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("device group not found")
		}
	}
	return global.NAV_DB.Model(&domains.Device{}).Where("guid = ?", deviceGuid).Updates(map[string]any{
		"group_guid":  req.GroupGuid,
		"tags":        req.Tags,
		"update_time": domains.NowMilli(),
	}).Error
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
