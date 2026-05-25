package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/google/uuid"
	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/global"
	commonUtils "github.com/wfu-work/nav-common-go-lib/utils"
	"gorm.io/gorm"
)

type DeviceService struct{}

type RegisterDeviceRequest struct {
	Token         string `json:"token"`
	SnCode        string `json:"sncode"`
	DeviceID      string `json:"deviceId"`
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
	DeviceID      string `json:"deviceId"`
	HostIP        string `json:"hostIp"`
	Hostname      string `json:"hostname"`
	ClientVersion string `json:"clientVersion"`
}

type DeviceRegisterResult struct {
	Device      domains.Device    `json:"device"`
	TypeDefault DeviceTypeDefault `json:"typeDefault"`
	PublicHost  string            `json:"publicHost"`
}

type CreateDeviceTokenRequest struct {
	Name       string `json:"name"`
	ExpireTime int64  `json:"expireTime"`
}

type DeviceTokenResult struct {
	Token string              `json:"token"`
	Item  domains.DeviceToken `json:"item"`
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
	typeDefault, ok := GetDeviceTypeDefault(req.DeviceType)
	if !ok {
		return nil, errors.New("unsupported device type")
	}
	if err := s.validateRegisterToken(req.Token, req.SnCode); err != nil {
		return nil, err
	}

	now := domains.NowMilli()
	var device domains.Device
	err := global.NAV_DB.Where("sn_code = ?", req.SnCode).First(&device).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := ensureAliasAvailable(req.Alias, ""); err != nil {
			return nil, err
		}
		device = domains.Device{
			Guid:       uuid.NewString(),
			SnCode:     req.SnCode,
			Alias:      req.Alias,
			CreateTime: now,
		}
	} else if err := ensureAliasAvailable(req.Alias, device.Guid); err != nil {
		return nil, err
	}

	device.DeviceID = req.DeviceID
	device.DeviceType = req.DeviceType
	device.Alias = req.Alias
	device.Remark = req.Remark
	device.Hostname = req.Hostname
	device.HostIP = req.HostIP
	device.ClientVersion = req.ClientVersion
	device.SourceIP = sourceIP
	device.SSHPort = req.SSHPort
	device.WebPort = req.WebPort
	device.WebDomain = req.WebDomain
	device.GroupGuid = req.GroupGuid
	device.Tags = normalizeTags(req.Tags)
	device.Status = domains.DeviceStatusOnline
	device.LastSeenTime = now
	device.UpdateTime = now

	if err := global.NAV_DB.Save(&device).Error; err != nil {
		return nil, err
	}
	if err := s.ensureDeviceToken(device.Guid, req.Token, req.SnCode); err != nil {
		return nil, err
	}
	if err := s.recordHeartbeat(device.Guid, sourceIP, req.HostIP, now); err != nil {
		return nil, err
	}

	return &DeviceRegisterResult{
		Device:      device,
		TypeDefault: typeDefault,
		PublicHost:  publicHost(req.SnCode, req.WebDomain),
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
	query := global.NAV_DB
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
	if err := s.validateDeviceToken(device.Guid, token); err != nil {
		return nil, err
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
	if err := global.NAV_DB.Model(&domains.Device{}).Where("guid = ?", device.Guid).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.recordHeartbeat(device.Guid, sourceIP, strings.TrimSpace(req.HostIP), now); err != nil {
		return nil, err
	}
	_ = global.NAV_DB.Where("guid = ?", device.Guid).First(&device).Error
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
	db := global.NAV_DB.Model(&domains.Device{})
	keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"]))
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("sn_code LIKE ? OR alias LIKE ? OR remark LIKE ? OR device_id LIKE ?", like, like, like, like)
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
	if err := global.NAV_DB.Where("guid = ?", strings.TrimSpace(guid)).First(&device).Error; err != nil {
		return nil, nil, errors.New("device not found")
	}
	var tokens []domains.DeviceToken
	_ = global.NAV_DB.Where("device_guid = ?", device.Guid).Order("create_time DESC").Find(&tokens).Error
	return &device, tokens, nil
}

func (s DeviceService) Delete(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	now := domains.NowMilli()
	return global.NAV_DB.Model(&domains.Device{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      domains.DeviceStatusDisabled,
		"update_time": now,
	}).Error
}

func (s DeviceService) DisableToken(deviceGuid, tokenGuid string) error {
	return s.SetTokenStatus(deviceGuid, tokenGuid, domains.DeviceTokenStatusDisabled)
}

func (s DeviceService) EnableToken(deviceGuid, tokenGuid string) error {
	return s.SetTokenStatus(deviceGuid, tokenGuid, domains.DeviceTokenStatusEnabled)
}

func (s DeviceService) SetTokenStatus(deviceGuid, tokenGuid string, status int) error {
	deviceGuid = strings.TrimSpace(deviceGuid)
	tokenGuid = strings.TrimSpace(tokenGuid)
	if deviceGuid == "" || tokenGuid == "" {
		return errors.New("deviceGuid and tokenGuid required")
	}
	if status != domains.DeviceTokenStatusDisabled && status != domains.DeviceTokenStatusEnabled {
		return errors.New("unsupported token status")
	}
	return global.NAV_DB.Model(&domains.DeviceToken{}).Where("device_guid = ? AND guid = ?", deviceGuid, tokenGuid).Updates(map[string]any{
		"status":      status,
		"update_time": domains.NowMilli(),
	}).Error
}

func (s DeviceService) CreateToken(deviceGuid string, req CreateDeviceTokenRequest) (*DeviceTokenResult, error) {
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return nil, errors.New("deviceGuid required")
	}
	if err := ensureDeviceExists(deviceGuid); err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	now := domains.NowMilli()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "manual"
	}
	row := domains.DeviceToken{
		Guid:       uuid.NewString(),
		DeviceGuid: deviceGuid,
		TokenHash:  utils.HashToken(token),
		Name:       name,
		Status:     domains.DeviceTokenStatusEnabled,
		ExpireTime: req.ExpireTime,
		CreateTime: now,
		UpdateTime: now,
	}
	if err := global.NAV_DB.Create(&row).Error; err != nil {
		return nil, err
	}
	return &DeviceTokenResult{Token: token, Item: row}, nil
}

func (s DeviceService) RotateToken(deviceGuid, tokenGuid string) (*DeviceTokenResult, error) {
	if err := s.DisableToken(deviceGuid, tokenGuid); err != nil {
		return nil, err
	}
	var old domains.DeviceToken
	_ = global.NAV_DB.Where("device_guid = ? AND guid = ?", strings.TrimSpace(deviceGuid), strings.TrimSpace(tokenGuid)).First(&old).Error
	return s.CreateToken(deviceGuid, CreateDeviceTokenRequest{Name: old.Name, ExpireTime: old.ExpireTime})
}

func (s DeviceService) TypeDefaults() []DeviceTypeDefault {
	return ListDeviceTypeDefaults()
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

func normalizeRegisterRequest(req RegisterDeviceRequest) RegisterDeviceRequest {
	req.Token = strings.TrimSpace(req.Token)
	req.SnCode = strings.TrimSpace(req.SnCode)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
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
	if def, ok := GetDeviceTypeDefault(req.DeviceType); ok {
		if req.WebPort <= 0 {
			req.WebPort = def.WebPort
		}
		if req.WebDomain == "" {
			req.WebDomain = def.WebDomain
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
	err := global.NAV_DB.Where("alias = ?", alias).First(&existing).Error
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

func (s DeviceService) validateRegisterToken(token, sncode string) error {
	bootstrap := registerToken()
	if token == bootstrap {
		return nil
	}
	var device domains.Device
	if err := global.NAV_DB.Where("sn_code = ?", sncode).First(&device).Error; err != nil {
		return errors.New("invalid register token")
	}
	return s.validateDeviceToken(device.Guid, token)
}

func (s DeviceService) validateDeviceToken(deviceGuid, token string) error {
	var count int64
	err := global.NAV_DB.Model(&domains.DeviceToken{}).
		Where("device_guid = ? AND token_hash = ? AND status = ?", deviceGuid, utils.HashToken(token), domains.DeviceTokenStatusEnabled).
		Where("expire_time = 0 OR expire_time > ?", domains.NowMilli()).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("invalid device token")
	}
	return nil
}

func (s DeviceService) ensureDeviceToken(deviceGuid, token, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "device"
	}
	hash := utils.HashToken(token)
	var row domains.DeviceToken
	err := global.NAV_DB.Where("device_guid = ? AND token_hash = ?", deviceGuid, hash).First(&row).Error
	if err == nil {
		return global.NAV_DB.Model(&row).Updates(map[string]any{
			"name":        name,
			"status":      domains.DeviceTokenStatusEnabled,
			"update_time": domains.NowMilli(),
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	now := domains.NowMilli()
	row = domains.DeviceToken{
		Guid:       uuid.NewString(),
		DeviceGuid: deviceGuid,
		TokenHash:  hash,
		Name:       name,
		Status:     domains.DeviceTokenStatusEnabled,
		CreateTime: now,
		UpdateTime: now,
	}
	return global.NAV_DB.Create(&row).Error
}

func (s DeviceService) recordHeartbeat(deviceGuid, sourceIP, hostIP string, now int64) error {
	return global.NAV_DB.Create(&domains.DeviceHeartbeat{
		DeviceGuid: deviceGuid,
		SourceIP:   sourceIP,
		HostIP:     hostIP,
		CreateTime: now,
	}).Error
}

func getSettingValue(key, def string) string {
	var row domains.Setting
	if err := global.NAV_DB.Where("key = ?", key).First(&row).Error; err == nil && strings.TrimSpace(row.Value) != "" {
		return strings.TrimSpace(row.Value)
	}
	return def
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

func PageResult(items any, total int64, params map[string]string) commonDomains.PageResult {
	page := utils.Str2Int(params["page"])
	size := utils.Str2Int(params["size"])
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	return commonDomains.PageResult{
		Data:  items,
		Total: total,
		Page:  page,
		Size:  size,
	}
}
