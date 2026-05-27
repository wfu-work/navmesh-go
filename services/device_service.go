package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/global"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	commonUtils "github.com/wfu-work/nav-common-go-lib/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DeviceService struct {
	commonServices.CrudService[domains.Device]
}

func (s DeviceService) WithDB(db *gorm.DB) DeviceService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type RegisterDeviceRequest struct {
	Token         string `json:"token"`
	SnCode        string `json:"sncode"`
	DeviceType    string `json:"type"`
	Alias         string `json:"alias"`
	Remark        string `json:"remark"`
	Hostname      string `json:"hostname"`
	HostIP        string `json:"hostIp"`
	ClientVersion string `json:"clientVersion"`
	SSHPort       int    `json:"sshPort"`
	WebPort       int    `json:"webPort"`
	WebDomain     string `json:"webDomain"`
	GroupGuid     string `json:"groupGuid"`
	Tags          string `json:"tags"`
}

type HeartbeatRequest struct {
	Token         string `json:"token"`
	SnCode        string `json:"sncode"`
	Guid          string `json:"guid"`
	HostIP        string `json:"hostIp"`
	Hostname      string `json:"hostname"`
	ClientVersion string `json:"clientVersion"`
}

type UpdateDeviceProfileRequest struct {
	Alias  string `json:"alias"`
	Remark string `json:"remark"`
}

type DeviceRegisterResult struct {
	Device      domains.Device        `json:"device"`
	TypeDefault domains.DeviceGroup   `json:"typeDefault"`
	PublicHost  string                `json:"publicHost"`
	DeviceToken *DeviceTokenResult    `json:"deviceToken,omitempty"`
	SSH         *DeviceSSHAliasResult `json:"ssh,omitempty"`
}

func (s DeviceService) Register(req RegisterDeviceRequest, sourceIP string) (*DeviceRegisterResult, error) {
	req = normalizeRegisterRequest(req)
	if req.SnCode == "" {
		return nil, errors.New("sncode required")
	}
	if req.DeviceType == "" {
		return nil, errors.New("type required")
	}
	if req.Token == "" {
		return nil, errors.New("token required")
	}
	typeDefault, err := ServiceGroupApp.GroupService.GetEnabled(req.DeviceType)
	if err != nil {
		return nil, errors.New("unsupported device group")
	}
	req.DeviceType = typeDefault.Key
	req.GroupGuid = typeDefault.Key
	now := domains.NowMilli()
	var device domains.Device
	err = s.DB().Unscoped().Where("sn_code = ?", req.SnCode).First(&device).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	isNewDevice := errors.Is(err, gorm.ErrRecordNotFound)
	if isNewDevice && req.Token != registerToken() {
		return nil, errors.New("device not found")
	}
	isDeletedDevice := !isNewDevice && device.DeletedTime.Valid
	isBootstrapToken, err := s.validateRegisterToken(req.Token, req.SnCode, &device, isNewDevice)
	if err != nil {
		return nil, err
	}
	if isNewDevice || isDeletedDevice {
		if err := ensureAliasAvailable(req.Alias, ""); err != nil {
			return nil, err
		}
	}
	if isNewDevice {
		device = domains.Device{
			BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now},
			Sncode:         req.SnCode,
			Alias:          req.Alias,
			Remark:         req.Remark,
		}
	}

	device.DeviceType = req.DeviceType
	if isNewDevice || isDeletedDevice {
		device.Alias = req.Alias
		device.Remark = req.Remark
		device.DeletedTime.Valid = false
	}
	device.Hostname = req.Hostname
	device.HostIP = req.HostIP
	device.ClientVersion = req.ClientVersion
	device.SourceIP = sourceIP
	device.SSHPort = req.SSHPort
	device.WebPort = req.WebPort
	device.WebDomain = req.WebDomain
	device.GroupGuid = req.GroupGuid
	device.Tags = normalizeTags(req.Tags)
	if isBootstrapToken {
		device.Status = domains.DeviceStatusRegistered
	} else {
		device.Status = domains.DeviceStatusOnline
		device.LastSeenTime = now
	}
	device.UpdateTime = now

	if err := s.DB().Save(&device).Error; err != nil {
		return nil, err
	}
	sshAlias, err := ServiceGroupApp.SSHService.EnsureDeviceAlias(device)
	if err != nil {
		return nil, err
	}
	var issuedToken *DeviceTokenResult
	if isBootstrapToken {
		if device.Status != domains.DeviceStatusRegistered {
			issuedToken, _ = s.tokenService().EnabledTokenResult(device.Guid)
		}
	}
	if !isBootstrapToken {
		if err := s.recordHeartbeat(device.Guid, sourceIP, req.HostIP, now); err != nil {
			return nil, err
		}
	}

	return &DeviceRegisterResult{
		Device:      device,
		TypeDefault: *typeDefault,
		PublicHost:  publicHost(req.SnCode, req.WebDomain),
		DeviceToken: issuedToken,
		SSH:         sshAlias,
	}, nil
}

func (s DeviceService) Heartbeat(req HeartbeatRequest, sourceIP string) (*domains.Device, error) {
	req.SnCode = strings.TrimSpace(req.SnCode)
	req.Guid = strings.TrimSpace(req.Guid)
	token := strings.TrimSpace(utils.FirstNonEmpty(req.Token, ""))
	if token == "" {
		return nil, errors.New("token required")
	}
	var device domains.Device
	query := s.DB()
	if req.Guid != "" {
		query = query.Where("guid = ?", req.Guid)
	} else if req.SnCode != "" {
		query = query.Where("sn_code = ?", req.SnCode)
	} else {
		return nil, errors.New("guid or sncode required")
	}
	if err := query.First(&device).Error; err != nil {
		return nil, errors.New("device not found")
	}
	if err := s.tokenService().Validate(device.Guid, token); err != nil {
		return nil, err
	}
	if err := ensureDeviceGroupEnabled(device.DeviceType); err != nil {
		return nil, err
	}
	if device.Status == domains.DeviceStatusRegistered {
		return nil, errors.New("device not activated")
	}
	if device.Status == domains.DeviceStatusDisabled {
		return nil, errors.New("device disabled")
	}
	now := domains.NowMilli()
	updates := map[string]any{
		"status":         domains.DeviceStatusOnline,
		"last_seen_time": now,
		"source_ip":      sourceIP,
		"update_time":    now,
	}
	if strings.TrimSpace(req.HostIP) != "" {
		updates["host_ip"] = strings.TrimSpace(req.HostIP)
	}
	if strings.TrimSpace(req.Hostname) != "" {
		updates["hostname"] = strings.TrimSpace(req.Hostname)
	}
	if strings.TrimSpace(req.ClientVersion) != "" {
		updates["client_version"] = strings.TrimSpace(req.ClientVersion)
	}
	if err := s.DB().Model(&domains.Device{}).Where("guid = ?", device.Guid).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.recordHeartbeat(device.Guid, sourceIP, strings.TrimSpace(req.HostIP), now); err != nil {
		return nil, err
	}
	_ = s.DB().Where("guid = ?", device.Guid).First(&device).Error
	return &device, nil
}

func (s DeviceService) UpdateProfile(guid string, req UpdateDeviceProfileRequest) (*domains.Device, error) {
	guid = strings.TrimSpace(guid)
	req.Alias = strings.TrimSpace(req.Alias)
	req.Remark = strings.TrimSpace(req.Remark)
	if guid == "" {
		return nil, errors.New("guid required")
	}
	if req.Alias == "" {
		return nil, errors.New("alias required")
	}
	if err := ensureAliasAvailable(req.Alias, guid); err != nil {
		return nil, err
	}
	now := domains.NowMilli()
	if err := s.DB().Model(&domains.Device{}).Where("guid = ?", guid).Updates(map[string]any{
		"alias":       req.Alias,
		"remark":      req.Remark,
		"update_time": now,
	}).Error; err != nil {
		return nil, err
	}
	var device domains.Device
	if err := s.DB().Where("guid = ?", guid).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (s DeviceService) List(params map[string]string) ([]domains.Device, int64, error) {
	pageInfo := commonUtils.ToPageInfo(params)
	if pageInfo.Page <= 0 {
		pageInfo.Page = 1
	}
	if pageInfo.Size <= 0 {
		pageInfo.Size = 20
	}
	db := s.DB().Model(&domains.Device{})
	keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"]))
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("sn_code LIKE ? OR alias LIKE ? OR remark LIKE ?", like, like, like)
	}
	if status := utils.Str2Int(params["status"]); status > 0 {
		db = db.Where("status = ?", status)
	}
	if deviceType := strings.TrimSpace(params["type"]); deviceType != "" {
		db = db.Where("device_type = ?", deviceType)
	}
	if groupGuid := strings.TrimSpace(params["groupGuid"]); groupGuid != "" {
		db = db.Where("group_guid = ?", groupGuid)
	}
	if tag := strings.TrimSpace(params["tag"]); tag != "" {
		db = db.Where("tags = ? OR tags LIKE ? OR tags LIKE ? OR tags LIKE ?", tag, tag+",%", "%,"+tag, "%,"+tag+",%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []domains.Device
	err := db.Order("update_time DESC").Limit(pageInfo.Size).Offset((pageInfo.Page - 1) * pageInfo.Size).Find(&items).Error
	return items, total, err
}

func (s DeviceService) Get(guid string) (*domains.Device, []domains.DeviceToken, error) {
	var device domains.Device
	if err := s.DB().Where("guid = ?", strings.TrimSpace(guid)).First(&device).Error; err != nil {
		return nil, nil, errors.New("device not found")
	}
	var tokens []domains.DeviceToken
	tokens, _ = s.tokenService().ListByDevice(device.Guid)
	return &device, tokens, nil
}

func (s DeviceService) Delete(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	closeRuntimeDeviceConnection(guid, "device deleted")
	s.closeDeviceRuntimeSessions(guid)
	return s.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("device_guid = ?", guid).Delete(&domains.DeviceToken{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("device_guid = ?", guid).Delete(&domains.SSHAlias{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("device_guid = ?", guid).Delete(&domains.PortMapping{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&domains.SSHEntrypoint{}).Where("device_guid = ?", guid).Updates(map[string]any{
			"device_guid": "",
			"update_time": domains.NowMilli(),
		}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("guid = ?", guid).Delete(&domains.Device{}).Error
	})
}

func (s DeviceService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	now := domains.NowMilli()
	return s.DB().Model(&domains.Device{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      domains.DeviceStatusDisabled,
		"update_time": now,
	}).Error
}

func (s DeviceService) Enable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	now := domains.NowMilli()
	var device domains.Device
	if err := s.DB().Where("guid = ?", guid).First(&device).Error; err != nil {
		return errors.New("device not found")
	}
	if _, err := s.tokenService().CreateToken(device.Guid, CreateDeviceTokenRequest{Name: device.Sncode}); err != nil {
		return err
	}
	return s.DB().Model(&domains.Device{}).
		Where("guid = ? AND status IN ?", guid, []int{domains.DeviceStatusRegistered, domains.DeviceStatusDisabled}).
		Updates(map[string]any{
			"status":      domains.DeviceStatusOffline,
			"update_time": now,
		}).Error
}

func (s DeviceService) TypeDefaults() []domains.DeviceGroup {
	items, _, _ := ServiceGroupApp.GroupService.List(map[string]string{"all": "true", "status": "1"})
	return items
}

func (s DeviceService) Authenticate(token, guid, sncode, sourceIP, hostIP, hostname, clientVersion string) (*domains.Device, error) {
	req := HeartbeatRequest{
		Token:         strings.TrimSpace(token),
		Guid:          strings.TrimSpace(guid),
		SnCode:        strings.TrimSpace(sncode),
		HostIP:        strings.TrimSpace(hostIP),
		Hostname:      strings.TrimSpace(hostname),
		ClientVersion: strings.TrimSpace(clientVersion),
	}
	return s.Heartbeat(req, sourceIP)
}

func (s DeviceService) MarkOnlineDevicesOffline() (int64, error) {
	now := domains.NowMilli()
	result := s.DB().Model(&domains.Device{}).
		Where("status = ?", domains.DeviceStatusOnline).
		Updates(map[string]any{"status": domains.DeviceStatusOffline, "update_time": now})
	if result.Error != nil {
		return result.RowsAffected, result.Error
	}
	if err := s.closeActiveDeviceConnections(now); err != nil {
		return result.RowsAffected, err
	}
	return result.RowsAffected, result.Error
}

func (s DeviceService) MarkStaleOnlineDevicesOffline(timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	now := domains.NowMilli()
	cutoff := now - timeout.Milliseconds()
	result := s.DB().Model(&domains.Device{}).
		Where("status = ?", domains.DeviceStatusOnline).
		Where("last_seen_time = 0 OR last_seen_time < ?", cutoff).
		Updates(map[string]any{"status": domains.DeviceStatusOffline, "update_time": now})
	if result.Error != nil || result.RowsAffected == 0 {
		return result.RowsAffected, result.Error
	}
	if err := s.closeStaleDeviceConnections(cutoff, now); err != nil {
		return result.RowsAffected, err
	}
	return result.RowsAffected, result.Error
}

func (s DeviceService) StartOfflineCleaner(ctx context.Context) {
	timeout := deviceHeartbeatTimeout()
	interval := deviceOfflineCheckInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			affected, err := s.MarkStaleOnlineDevicesOffline(timeout)
			if err != nil {
				global.NAV_LOG.Warn("mark stale online devices offline failed", zap.Error(err))
				continue
			}
			if affected > 0 {
				global.NAV_LOG.Info("mark stale online devices offline", zap.Int64("affected", affected), zap.Duration("timeout", timeout))
			}
		}
	}
}

func normalizeRegisterRequest(req RegisterDeviceRequest) RegisterDeviceRequest {
	req.Token = strings.TrimSpace(req.Token)
	req.SnCode = strings.TrimSpace(req.SnCode)
	req.DeviceType = strings.TrimSpace(req.DeviceType)
	req.Alias = strings.TrimSpace(req.Alias)
	req.Remark = strings.TrimSpace(req.Remark)
	req.Hostname = strings.TrimSpace(req.Hostname)
	req.HostIP = strings.TrimSpace(req.HostIP)
	req.ClientVersion = strings.TrimSpace(req.ClientVersion)
	req.WebDomain = strings.TrimSpace(req.WebDomain)
	req.GroupGuid = strings.TrimSpace(req.GroupGuid)
	req.Tags = normalizeTags(req.Tags)
	if req.Alias == "" {
		req.Alias = req.SnCode
	}
	if req.SSHPort <= 0 {
		req.SSHPort = 22
	}
	if def, err := ServiceGroupApp.GroupService.GetEnabled(req.DeviceType); err == nil {
		req.DeviceType = def.Key
		req.GroupGuid = def.Key
		if req.WebPort <= 0 {
			req.WebPort = def.DefaultWebPort
		}
		if req.WebDomain == "" {
			req.WebDomain = def.DefaultDomain
		}
	}
	return req
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "nmt_" + hex.EncodeToString(buf), nil
}

func ensureAliasAvailable(alias, currentGuid string) error {
	if alias == "" {
		return errors.New("alias required")
	}
	var existing domains.Device
	err := ServiceGroupApp.DeviceService.DB().Where("alias = ?", alias).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.Guid != currentGuid {
		return errors.New("alias already exists")
	}
	return nil
}

func ensureDeviceGroupEnabled(deviceType string) error {
	if _, err := ServiceGroupApp.GroupService.GetEnabled(strings.TrimSpace(deviceType)); err != nil {
		return errors.New("unsupported device group")
	}
	return nil
}

func (s DeviceService) validateRegisterToken(token, sncode string, device *domains.Device, isNewDevice bool) (bool, error) {
	bootstrap := registerToken()
	if token == bootstrap {
		if isNewDevice {
			return true, nil
		}
		if !s.tokenService().HasEnabled(device.Guid) {
			device.Status = domains.DeviceStatusRegistered
			return true, nil
		}
		return true, nil
	}
	if isNewDevice {
		return false, errors.New("invalid register token")
	}
	return false, s.tokenService().Validate(device.Guid, token)
}

func (s DeviceService) tokenService() DeviceTokenService {
	return ServiceGroupApp.DeviceTokenService.WithDB(s.DB())
}

func (s DeviceService) recordHeartbeat(deviceGuid, sourceIP, hostIP string, now int64) error {
	return s.DB().Create(&domains.DeviceHeartbeat{
		DeviceGuid: deviceGuid,
		SourceIP:   sourceIP,
		HostIP:     hostIP,
		CreateTime: now,
	}).Error
}

func (s DeviceService) closeActiveDeviceConnections(now int64) error {
	return s.DB().Model(&domains.DeviceConnection{}).
		Where("status = ?", int(domains.StatusEnabled)).
		Updates(map[string]any{"status": int(domains.StatusDisabled), "update_time": now}).Error
}

func (s DeviceService) closeStaleDeviceConnections(cutoff, now int64) error {
	return s.DB().Model(&domains.DeviceConnection{}).
		Where("status = ? AND last_active_time < ?", int(domains.StatusEnabled), cutoff).
		Updates(map[string]any{"status": int(domains.StatusDisabled), "update_time": now}).Error
}

func (s DeviceService) closeDeviceRuntimeSessions(deviceGuid string) {
	var sessions []domains.TunnelSession
	if err := s.DB().
		Where("device_guid = ? AND status = ?", deviceGuid, int(domains.StatusEnabled)).
		Find(&sessions).Error; err != nil {
		return
	}
	now := domains.NowMilli()
	for _, session := range sessions {
		DefaultSessionRegistry.CloseSession(session.Guid)
	}
	_ = s.DB().Model(&domains.TunnelSession{}).
		Where("device_guid = ? AND status = ?", deviceGuid, int(domains.StatusEnabled)).
		Updates(map[string]any{
			"status":            int(domains.StatusDisabled),
			"force_closed":      true,
			"disconnect_reason": "device_deleted",
			"end_time":          now,
			"update_time":       now,
		}).Error
}

func getSettingValue(key, def string) string {
	var row domains.Setting
	if err := global.NAV_DB.Where("key = ?", key).First(&row).Error; err == nil && strings.TrimSpace(row.Value) != "" {
		return strings.TrimSpace(row.Value)
	}
	return def
}

func deviceSettingDuration(key string, def time.Duration) time.Duration {
	value := strings.TrimSpace(getSettingValue(key, ""))
	if value == "" {
		return def
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return def
	}
	return duration
}

func deviceHeartbeatTimeout() time.Duration {
	if global.NAV_VIPER != nil {
		if value := strings.TrimSpace(global.NAV_VIPER.GetString("navmesh.heartbeat-timeout")); value != "" {
			if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
				return duration
			}
		}
		if value := strings.TrimSpace(global.NAV_VIPER.GetString("navmesh.heartbeat_timeout")); value != "" {
			if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
				return duration
			}
		}
	}
	return deviceSettingDuration("device_heartbeat_timeout", 90*time.Second)
}

func deviceOfflineCheckInterval() time.Duration {
	return deviceSettingDuration("device_offline_check_interval", 30*time.Second)
}

func DefaultDeviceRegisterToken() string {
	if value, ok := configuredDeviceRegisterToken(); ok {
		return value
	}
	return "navfirst@2020"
}

func registerToken() string {
	if value, ok := configuredDeviceRegisterToken(); ok {
		return value
	}
	return getSettingValue("device_register_token", "navfirst@2020")
}

func configuredDeviceRegisterToken() (string, bool) {
	if global.NAV_VIPER != nil {
		if value := strings.TrimSpace(global.NAV_VIPER.GetString("navmesh.device-register-token")); value != "" {
			return value, true
		}
		if value := strings.TrimSpace(global.NAV_VIPER.GetString("navmesh.device_register_token")); value != "" {
			return value, true
		}
	}
	return "", false
}

func publicHost(sncode, domain string) string {
	sncode = strings.TrimSpace(sncode)
	domain = strings.TrimSpace(domain)
	if sncode == "" || domain == "" {
		return ""
	}
	return sncode + "." + domain
}
