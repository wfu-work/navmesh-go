package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

type SSHService struct {
	AliasCrud commonServices.CrudService[domains.SSHAlias]
}

func (s SSHService) WithDB(db *gorm.DB) SSHService {
	s.AliasCrud = *s.AliasCrud.WithDB(db)
	return s
}

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

type DeviceSSHAliasResult struct {
	Alias        domains.SSHAlias `json:"alias"`
	EntrypointIP string           `json:"entrypointIp"`
	Ready        bool             `json:"ready"`
}

func (s SSHService) ListEntrypoints() ([]domains.SSHEntrypoint, error) {
	var items []domains.SSHEntrypoint
	err := s.AliasCrud.DB().Order("create_time DESC").Find(&items).Error
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
	err := s.AliasCrud.DB().Where("ip = ?", req.IP).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.SSHEntrypoint{IP: req.IP, CreateTime: now}
	}
	row.DeviceGuid = req.DeviceGuid
	row.Status = req.Status
	row.UpdateTime = now
	return &row, s.AliasCrud.DB().Save(&row).Error
}

func (s SSHService) ListAliases() ([]domains.SSHAlias, error) {
	var items []domains.SSHAlias
	err := s.AliasCrud.DB().Order("create_time DESC").Find(&items).Error
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
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	if err := ensureDeviceExists(req.DeviceGuid); err != nil {
		return nil, err
	}
	now := domains.NowMilli()
	var row domains.SSHAlias
	err := s.AliasCrud.DB().Where("domain = ?", req.Domain).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.SSHAlias{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}}
	}
	if req.EntrypointIP == "" {
		req.EntrypointIP = strings.TrimSpace(row.EntrypointIP)
	}
	if req.EntrypointIP == "" {
		req.EntrypointIP = s.deviceEntrypointIP(req.DeviceGuid)
	}
	row.DeviceGuid = req.DeviceGuid
	row.Alias = req.Alias
	row.Domain = req.Domain
	row.EntrypointIP = req.EntrypointIP
	row.Status = req.Status
	row.UpdateTime = now
	if err := s.AliasCrud.DB().Save(&row).Error; err != nil {
		return nil, err
	}
	if req.EntrypointIP != "" {
		_, _ = s.SaveEntrypoint(SaveSSHEntrypointRequest{IP: req.EntrypointIP, DeviceGuid: req.DeviceGuid, Status: req.Status})
	}
	return &row, nil
}

func (s SSHService) EnsureDeviceAlias(device domains.Device) (*DeviceSSHAliasResult, error) {
	device.Guid = strings.TrimSpace(device.Guid)
	device.Alias = strings.TrimSpace(device.Alias)
	device.Sncode = strings.TrimSpace(device.Sncode)
	if device.Guid == "" {
		return nil, errors.New("device guid required")
	}
	alias := device.Sncode
	if alias == "" {
		return nil, errors.New("device sncode required")
	}
	domain := sshAliasDomain(alias)
	now := domains.NowMilli()
	entrypointIP := s.deviceEntrypointIP(device.Guid)

	var row domains.SSHAlias
	err := s.AliasCrud.DB().Where("device_guid = ?", device.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := s.AliasCrud.DB().Where("(alias = ? OR domain = ?) AND device_guid != ?", alias, domain, device.Guid).First(&domains.SSHAlias{}).Error; err == nil {
			return nil, errors.New("ssh alias already exists")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		row = domains.SSHAlias{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}}
	}
	if row.EntrypointIP != "" {
		entrypointIP = row.EntrypointIP
	}
	row.DeviceGuid = device.Guid
	row.Alias = alias
	row.Domain = domain
	row.EntrypointIP = entrypointIP
	row.Status = int(domains.StatusEnabled)
	row.UpdateTime = now
	if err := s.AliasCrud.DB().Save(&row).Error; err != nil {
		return nil, err
	}
	if entrypointIP != "" {
		_, _ = s.SaveEntrypoint(SaveSSHEntrypointRequest{IP: entrypointIP, DeviceGuid: device.Guid, Status: int(domains.StatusEnabled)})
	}
	return &DeviceSSHAliasResult{Alias: row, EntrypointIP: entrypointIP, Ready: entrypointIP != ""}, nil
}

func (s SSHService) DisableAlias(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return s.AliasCrud.DB().Model(&domains.SSHAlias{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func (s SSHService) deviceEntrypointIP(deviceGuid string) string {
	var bound domains.SSHEntrypoint
	if err := s.AliasCrud.DB().
		Where("device_guid = ? AND status = ?", deviceGuid, int(domains.StatusEnabled)).
		Order("update_time DESC, id ASC").
		Limit(1).
		Find(&bound).Error; err == nil && bound.IP != "" {
		return strings.TrimSpace(bound.IP)
	}
	var free domains.SSHEntrypoint
	if err := s.AliasCrud.DB().
		Where("(device_guid = '' OR device_guid IS NULL) AND status = ?", int(domains.StatusEnabled)).
		Order("id ASC").
		Limit(1).
		Find(&free).Error; err == nil && free.IP != "" {
		return strings.TrimSpace(free.IP)
	}
	return ""
}

func sshAliasDomain(alias string) string {
	alias = strings.TrimSpace(alias)
	publicDomain := strings.TrimSpace(getSettingValue("ssh_gateway_domain", "ssh.navfirst.com"))
	if publicDomain == "" {
		return alias
	}
	return alias + "." + strings.TrimPrefix(publicDomain, ".")
}

func ensureDeviceExists(guid string) error {
	var count int64
	if err := ServiceGroupApp.DeviceService.DB().Model(&domains.Device{}).Where("guid = ? AND status != ?", guid, domains.DeviceStatusDisabled).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("device not found")
	}
	return nil
}
