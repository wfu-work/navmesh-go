package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/gorm"
)

type SSHService struct{}

type SaveSSHEntrypointRequest struct {
	IP         string `json:"ip"`
	DeviceGuid string `json:"deviceGuid"`
	Status     int    `json:"status"`
}

type SaveSSHAliasRequest struct {
	DeviceGuid   string `json:"deviceGuid"`
	Alias        string `json:"alias"`
	Domain       string `json:"domain"`
	EntrypointIP string `json:"entrypointIp"`
	Status       int    `json:"status"`
}

func (s SSHService) ListEntrypoints() ([]domains.SSHEntrypoint, error) {
	var items []domains.SSHEntrypoint
	err := global.NAV_DB.Order("create_time DESC").Find(&items).Error
	return items, err
}

func (s SSHService) SaveEntrypoint(req SaveSSHEntrypointRequest) (*domains.SSHEntrypoint, error) {
	req.IP = strings.TrimSpace(req.IP)
	req.DeviceGuid = strings.TrimSpace(req.DeviceGuid)
	if req.IP == "" {
		return nil, errors.New("ip required")
	}
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	now := domains.NowMilli()
	var row domains.SSHEntrypoint
	err := global.NAV_DB.Where("ip = ?", req.IP).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.SSHEntrypoint{IP: req.IP, CreateTime: now}
	}
	row.DeviceGuid = req.DeviceGuid
	row.Status = req.Status
	row.UpdateTime = now
	return &row, global.NAV_DB.Save(&row).Error
}

func (s SSHService) ListAliases() ([]domains.SSHAlias, error) {
	var items []domains.SSHAlias
	err := global.NAV_DB.Order("create_time DESC").Find(&items).Error
	return items, err
}

func (s SSHService) SaveAlias(req SaveSSHAliasRequest) (*domains.SSHAlias, error) {
	req.DeviceGuid = strings.TrimSpace(req.DeviceGuid)
	req.Alias = strings.TrimSpace(req.Alias)
	req.Domain = strings.TrimSpace(req.Domain)
	req.EntrypointIP = strings.TrimSpace(req.EntrypointIP)
	if req.DeviceGuid == "" {
		return nil, errors.New("deviceGuid required")
	}
	if req.Alias == "" {
		return nil, errors.New("alias required")
	}
	if req.Domain == "" {
		return nil, errors.New("domain required")
	}
	if req.EntrypointIP == "" {
		return nil, errors.New("entrypointIp required")
	}
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	if err := ensureDeviceExists(req.DeviceGuid); err != nil {
		return nil, err
	}
	now := domains.NowMilli()
	var row domains.SSHAlias
	err := global.NAV_DB.Where("domain = ?", req.Domain).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.SSHAlias{CreateTime: now}
	}
	row.DeviceGuid = req.DeviceGuid
	row.Alias = req.Alias
	row.Domain = req.Domain
	row.EntrypointIP = req.EntrypointIP
	row.Status = req.Status
	row.UpdateTime = now
	if err := global.NAV_DB.Save(&row).Error; err != nil {
		return nil, err
	}
	_, _ = s.SaveEntrypoint(SaveSSHEntrypointRequest{IP: req.EntrypointIP, DeviceGuid: req.DeviceGuid, Status: req.Status})
	return &row, nil
}

func (s SSHService) DisableAlias(id uint) error {
	if id == 0 {
		return errors.New("id required")
	}
	return global.NAV_DB.Model(&domains.SSHAlias{}).Where("id = ?", id).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func ensureDeviceExists(guid string) error {
	var count int64
	if err := global.NAV_DB.Model(&domains.Device{}).Where("guid = ? AND status != ?", guid, domains.DeviceStatusDisabled).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("device not found")
	}
	return nil
}
