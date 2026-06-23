package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/global"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	commonUtils "github.com/wfu-work/nav-common-go-lib/utils"
	"github.com/wfu-work/nav-common-go-lib/utils/ip2geo"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DeviceService struct {
	commonServices.CrudService[domains.Device]
}

const (
	diskUsageHighEventType              = "disk_usage_high"
	diskUsageHighThreshold              = 90.0
	deviceOfflineEventType              = "device_offline"
	deviceOfflineEventSuppressionWindow = 6 * time.Hour
	deviceOnlineTouchMinInterval        = 15 * time.Second
)

func (s DeviceService) WithDB(db *gorm.DB) DeviceService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type RegisterDeviceRequest struct {
	Token             string  `json:"token"`
	Guid              string  `json:"guid"`
	SnCode            string  `json:"sncode"`
	DeviceType        string  `json:"type"`
	Alias             string  `json:"alias"`
	Remark            string  `json:"remark"`
	Hostname          string  `json:"hostname"`
	HostIP            string  `json:"hostIp"`
	WanIP             string  `json:"wanIp"`
	ClientVersion     string  `json:"clientVersion"`
	OS                string  `json:"os"`
	OSVersion         string  `json:"osVersion"`
	Kernel            string  `json:"kernel"`
	Arch              string  `json:"arch"`
	MemoryTotal       int64   `json:"memoryTotal"`
	MemoryUsed        int64   `json:"memoryUsed"`
	MemoryFree        int64   `json:"memoryFree"`
	DiskTotal         int64   `json:"diskTotal"`
	DiskUsed          int64   `json:"diskUsed"`
	DiskFree          int64   `json:"diskFree"`
	DiskUsedPct       float64 `json:"diskUsedPct"`
	NetworkType       string  `json:"networkType"`
	NetworkIface      string  `json:"networkIface"`
	SignalDBM         int     `json:"signalDbm"`
	SignalPct         int     `json:"signalPct"`
	CellularRSRP      int     `json:"cellularRsrp"`
	CellularRSRQ      int     `json:"cellularRsrq"`
	CellularSINR      int     `json:"cellularSinr"`
	WifiSSID          string  `json:"wifiSsid"`
	WifiRSSI          int     `json:"wifiRssi"`
	PingTarget        string  `json:"pingTarget"`
	PingLatencyMs     int64   `json:"pingLatencyMs"`
	PingLossPct       float64 `json:"pingLossPct"`
	RXRateBps         int64   `json:"rxRateBps"`
	TXRateBps         int64   `json:"txRateBps"`
	TrafficIface      string  `json:"trafficIface"`
	TrafficRXBytes    int64   `json:"trafficRxBytes"`
	TrafficTXBytes    int64   `json:"trafficTxBytes"`
	TrafficSampleTime int64   `json:"trafficSampleTime"`
	TrafficBootID     string  `json:"trafficBootId"`
	SSHPort           int     `json:"sshPort"`
	WebPort           int     `json:"webPort"`
	WebDomain         string  `json:"webDomain"`
	GroupGuid         string  `json:"groupGuid"`
	Tags              string  `json:"tags"`
}

type HeartbeatRequest struct {
	Token             string  `json:"token"`
	SnCode            string  `json:"sncode"`
	Guid              string  `json:"guid"`
	HostIP            string  `json:"hostIp"`
	WanIP             string  `json:"wanIp"`
	Hostname          string  `json:"hostname"`
	ClientVersion     string  `json:"clientVersion"`
	OS                string  `json:"os"`
	OSVersion         string  `json:"osVersion"`
	Kernel            string  `json:"kernel"`
	Arch              string  `json:"arch"`
	MemoryTotal       int64   `json:"memoryTotal"`
	MemoryUsed        int64   `json:"memoryUsed"`
	MemoryFree        int64   `json:"memoryFree"`
	DiskTotal         int64   `json:"diskTotal"`
	DiskUsed          int64   `json:"diskUsed"`
	DiskFree          int64   `json:"diskFree"`
	DiskUsedPct       float64 `json:"diskUsedPct"`
	NetworkType       string  `json:"networkType"`
	NetworkIface      string  `json:"networkIface"`
	SignalDBM         int     `json:"signalDbm"`
	SignalPct         int     `json:"signalPct"`
	CellularRSRP      int     `json:"cellularRsrp"`
	CellularRSRQ      int     `json:"cellularRsrq"`
	CellularSINR      int     `json:"cellularSinr"`
	WifiSSID          string  `json:"wifiSsid"`
	WifiRSSI          int     `json:"wifiRssi"`
	PingTarget        string  `json:"pingTarget"`
	PingLatencyMs     int64   `json:"pingLatencyMs"`
	PingLossPct       float64 `json:"pingLossPct"`
	RXRateBps         int64   `json:"rxRateBps"`
	TXRateBps         int64   `json:"txRateBps"`
	TrafficIface      string  `json:"trafficIface"`
	TrafficRXBytes    int64   `json:"trafficRxBytes"`
	TrafficTXBytes    int64   `json:"trafficTxBytes"`
	TrafficSampleTime int64   `json:"trafficSampleTime"`
	TrafficBootID     string  `json:"trafficBootId"`
}

type UpdateDeviceProfileRequest struct {
	SnCode     string `json:"sncode"`
	DeviceType string `json:"type"`
	Alias      string `json:"alias"`
	Remark     string `json:"remark"`
}

type DeviceStats struct {
	Total      int64 `json:"total"`
	Registered int64 `json:"registered"`
	Online     int64 `json:"online"`
	Offline    int64 `json:"offline"`
	Disabled   int64 `json:"disabled"`
}

type DeviceVPNRestartCommand struct {
	RequestedAt int64  `json:"requestedAt"`
	Message     string `json:"message"`
}

type DeviceDetailDevice struct {
	domains.Device
	WebDomains []string `json:"webDomains"`
}

type DeviceDetailResult struct {
	Device DeviceDetailDevice    `json:"device"`
	Tokens []domains.DeviceToken `json:"tokens"`
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
	sourceIP = normalizeIP(sourceIP)
	if req.Token == "" {
		return nil, errors.New("token required")
	}
	now := domains.NowMilli()
	var device domains.Device
	query := s.DB().Unscoped()
	if req.Guid != "" {
		query = query.Where("guid = ?", req.Guid)
	} else if req.SnCode != "" {
		query = query.Where("sn_code = ?", req.SnCode)
	} else {
		return nil, errors.New("sncode or guid required")
	}
	err := query.First(&device).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	isNewDevice := errors.Is(err, gorm.ErrRecordNotFound)
	if isNewDevice && req.SnCode == "" {
		return nil, errors.New("sncode required")
	}
	deviceType := req.DeviceType
	if !isNewDevice && strings.TrimSpace(device.DeviceType) != "" {
		deviceType = device.DeviceType
	}
	if deviceType == "" {
		deviceType = "ssh"
	}
	typeDefault, err := ServiceGroupApp.GroupService.GetEnabled(deviceType)
	if err != nil {
		return nil, errors.New("unsupported device group")
	}
	req.DeviceType = typeDefault.Key
	req.GroupGuid = typeDefault.Key
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

	if isNewDevice || isDeletedDevice || strings.TrimSpace(device.DeviceType) == "" {
		device.DeviceType = req.DeviceType
		device.GroupGuid = req.GroupGuid
	}
	if isNewDevice || isDeletedDevice {
		device.Sncode = req.SnCode
		device.Alias = req.Alias
		device.Remark = req.Remark
		device.DeletedTime.Valid = false
	}
	device.Hostname = req.Hostname
	device.HostIP = req.HostIP
	if wanIP := resolveDeviceWanIP(req.WanIP, sourceIP); wanIP != "" || normalizeIPv4(device.WanIP) == "" {
		device.WanIP = wanIP
		if wanIP == "" {
			device.Location = ""
		}
	}
	if location := resolveDeviceLocation(device.WanIP); location != "" {
		device.Location = location
	}
	device.ClientVersion = req.ClientVersion
	device.OS = req.OS
	device.OSVersion = req.OSVersion
	device.Kernel = req.Kernel
	device.Arch = req.Arch
	device.MemoryTotal = req.MemoryTotal
	device.MemoryUsed = req.MemoryUsed
	device.MemoryFree = req.MemoryFree
	device.DiskTotal = req.DiskTotal
	device.DiskUsed = req.DiskUsed
	device.DiskFree = req.DiskFree
	device.DiskUsedPct = req.DiskUsedPct
	applyNetworkSnapshotToDevice(&device, networkSnapshotFromRegister(req))
	if hasRegisterMetrics(req) {
		device.LastMetricAt = now
	}
	device.SourceIP = sourceIP
	device.SSHPort = req.SSHPort
	device.WebPort = req.WebPort
	device.WebDomain = req.WebDomain
	device.Tags = normalizeTags(req.Tags)
	if isBootstrapToken && (isNewDevice || isDeletedDevice || !s.tokenService().HasEnabled(device.Guid)) {
		device.Status = domains.DeviceStatusRegistered
	} else if !isBootstrapToken {
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
		if err := s.recordHeartbeat(device.Guid, sourceIP, req.HostIP, device.WanIP, device.Location, networkSnapshotFromRegister(req), now); err != nil {
			return nil, err
		}
		if err := ServiceGroupApp.DeviceTrafficService.WithDB(s.DB()).RecordHeartbeatSample(device.Guid, HeartbeatRequest{
			TrafficIface:      req.TrafficIface,
			TrafficRXBytes:    req.TrafficRXBytes,
			TrafficTXBytes:    req.TrafficTXBytes,
			TrafficSampleTime: req.TrafficSampleTime,
			TrafficBootID:     req.TrafficBootID,
		}, now); err != nil {
			global.NAV_LOG.Warn("record device traffic register sample failed", zap.String("deviceGuid", device.Guid), zap.Error(err))
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
	sourceIP = normalizeIP(sourceIP)
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
	wanIP := resolveDeviceWanIP(req.WanIP, sourceIP)
	location := ""
	if wanIP != "" {
		updates["wan_ip"] = wanIP
		location = resolveDeviceLocation(wanIP)
		if location != "" {
			updates["location"] = location
		}
	} else if strings.TrimSpace(device.WanIP) != "" && normalizeIPv4(device.WanIP) == "" {
		updates["wan_ip"] = ""
		updates["location"] = ""
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
	if strings.TrimSpace(req.OS) != "" {
		updates["os"] = strings.TrimSpace(req.OS)
	}
	if strings.TrimSpace(req.OSVersion) != "" {
		updates["os_version"] = strings.TrimSpace(req.OSVersion)
	}
	if strings.TrimSpace(req.Kernel) != "" {
		updates["kernel"] = strings.TrimSpace(req.Kernel)
	}
	if strings.TrimSpace(req.Arch) != "" {
		updates["arch"] = strings.TrimSpace(req.Arch)
	}
	if req.MemoryTotal > 0 {
		updates["memory_total"] = req.MemoryTotal
	}
	if req.MemoryUsed > 0 {
		updates["memory_used"] = req.MemoryUsed
	}
	if req.MemoryFree > 0 {
		updates["memory_free"] = req.MemoryFree
	}
	if req.DiskTotal > 0 {
		updates["disk_total"] = req.DiskTotal
	}
	if req.DiskUsed > 0 {
		updates["disk_used"] = req.DiskUsed
	}
	if req.DiskFree > 0 {
		updates["disk_free"] = req.DiskFree
	}
	if req.DiskUsedPct > 0 {
		updates["disk_used_pct"] = req.DiskUsedPct
	}
	networkSnapshot := networkSnapshotFromHeartbeat(req)
	addNetworkSnapshotUpdates(updates, networkSnapshot)
	if hasHeartbeatMetrics(req, networkSnapshot) {
		updates["last_metric_at"] = now
	}
	if err := s.DB().Model(&domains.Device{}).Where("guid = ?", device.Guid).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.recordHeartbeat(device.Guid, sourceIP, strings.TrimSpace(req.HostIP), wanIP, location, networkSnapshot, now); err != nil {
		return nil, err
	}
	s.recordDailyDiskUsageEvent(device.Guid, req, now)
	if err := ServiceGroupApp.DeviceTrafficService.WithDB(s.DB()).RecordHeartbeatSample(device.Guid, req, now); err != nil {
		global.NAV_LOG.Warn("record device traffic sample failed", zap.String("deviceGuid", device.Guid), zap.Error(err))
	}
	_ = s.DB().Where("guid = ?", device.Guid).First(&device).Error
	return &device, nil
}

func (s DeviceService) UpdateProfile(guid string, req UpdateDeviceProfileRequest) (*domains.Device, error) {
	guid = strings.TrimSpace(guid)
	req.SnCode = strings.TrimSpace(req.SnCode)
	req.DeviceType = strings.TrimSpace(req.DeviceType)
	req.Alias = strings.TrimSpace(req.Alias)
	req.Remark = strings.TrimSpace(req.Remark)
	if guid == "" {
		return nil, errors.New("guid required")
	}
	updates := map[string]any{"update_time": domains.NowMilli()}
	if req.SnCode != "" {
		if err := ensureSncodeAvailable(req.SnCode, guid); err != nil {
			return nil, err
		}
		updates["sn_code"] = req.SnCode
	}
	if req.DeviceType != "" {
		group, err := ServiceGroupApp.GroupService.GetEnabled(req.DeviceType)
		if err != nil {
			return nil, err
		}
		updates["device_type"] = group.Key
		updates["group_guid"] = group.Key
	}
	if req.Alias != "" {
		if err := ensureAliasAvailable(req.Alias, guid); err != nil {
			return nil, err
		}
		updates["alias"] = req.Alias
	}
	if req.Remark != "" {
		updates["remark"] = req.Remark
	}
	if err := s.DB().Model(&domains.Device{}).Where("guid = ?", guid).Updates(updates).Error; err != nil {
		return nil, err
	}
	var device domains.Device
	if err := s.DB().Where("guid = ?", guid).First(&device).Error; err != nil {
		return nil, err
	}
	if req.SnCode != "" {
		if _, err := ServiceGroupApp.SSHService.EnsureDeviceAlias(device); err != nil {
			global.NAV_LOG.Warn("refresh ssh alias after sncode update failed", zap.String("deviceGuid", guid), zap.Error(err))
		}
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
	db := s.deviceListQuery(params, true)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []domains.Device
	err := db.Order("update_time DESC").Limit(pageInfo.Size).Offset((pageInfo.Page - 1) * pageInfo.Size).Find(&items).Error
	return items, total, err
}

func (s DeviceService) Stats(params map[string]string) (*DeviceStats, error) {
	stats := &DeviceStats{}
	if err := s.deviceListQuery(params, true).Count(&stats.Total).Error; err != nil {
		return nil, err
	}
	countStatus := func(status int) int64 {
		var count int64
		_ = s.deviceListQuery(params, true).Where("status = ?", status).Count(&count).Error
		return count
	}
	stats.Registered = countStatus(domains.DeviceStatusRegistered)
	stats.Online = countStatus(domains.DeviceStatusOnline)
	stats.Offline = countStatus(domains.DeviceStatusOffline)
	stats.Disabled = countStatus(domains.DeviceStatusDisabled)
	return stats, nil
}

func (s DeviceService) RequestVPNRestart(guid string) (*DeviceVPNRestartCommand, error) {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return nil, errors.New("guid required")
	}
	var device domains.Device
	if err := s.DB().Where("guid = ?", guid).First(&device).Error; err != nil {
		return nil, errors.New("device not found")
	}
	if device.Status != domains.DeviceStatusOnline {
		return nil, errors.New("device is not online")
	}
	now := domains.NowMilli()
	if err := s.DB().Model(&domains.Device{}).Where("guid = ?", guid).Updates(map[string]any{
		"vpn_restart_requested_at": now,
		"update_time":              now,
	}).Error; err != nil {
		return nil, err
	}
	return &DeviceVPNRestartCommand{RequestedAt: now, Message: "restart vpn tunnel"}, nil
}

func (s DeviceService) TakeVPNRestartCommand(guid string) (*DeviceVPNRestartCommand, error) {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return nil, nil
	}
	var device domains.Device
	if err := s.DB().Select("guid", "vpn_restart_requested_at", "vpn_restart_delivered_at").Where("guid = ?", guid).First(&device).Error; err != nil {
		return nil, err
	}
	if device.VPNRestartRequestedAt <= 0 || device.VPNRestartRequestedAt <= device.VPNRestartDeliveredAt {
		return nil, nil
	}
	now := domains.NowMilli()
	if err := s.DB().Model(&domains.Device{}).Where("guid = ?", guid).Updates(map[string]any{
		"vpn_restart_delivered_at": now,
		"update_time":              now,
	}).Error; err != nil {
		return nil, err
	}
	return &DeviceVPNRestartCommand{RequestedAt: device.VPNRestartRequestedAt, Message: "restart vpn tunnel"}, nil
}

func (s DeviceService) deviceListQuery(params map[string]string, includeStatus bool) *gorm.DB {
	db := s.DB().Model(&domains.Device{})
	keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"]))
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("sn_code LIKE ? OR alias LIKE ? OR remark LIKE ?", like, like, like)
	}
	if includeStatus {
		if status := utils.Str2Int(params["status"]); status > 0 {
			db = db.Where("status = ?", status)
		}
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
	return db
}

func (s DeviceService) Get(guid string) (*DeviceDetailResult, error) {
	var device domains.Device
	if err := s.DB().Where("guid = ?", strings.TrimSpace(guid)).First(&device).Error; err != nil {
		return nil, errors.New("device not found")
	}
	s.hydrateDeviceDetail(&device)
	var tokens []domains.DeviceToken
	tokens, _ = s.tokenService().ListByDevice(device.Guid)
	return &DeviceDetailResult{
		Device: DeviceDetailDevice{
			Device:     device,
			WebDomains: splitDeviceWebDomains(device.WebDomain),
		},
		Tokens: tokens,
	}, nil
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
		if err := tx.Unscoped().Where("device_guid = ?", guid).Delete(&domains.DeviceTrafficState{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("device_guid = ?", guid).Delete(&domains.DeviceTrafficDaily{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("device_guid = ?", guid).Delete(&domains.SSHAlias{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("device_guid = ?", guid).Delete(&domains.PortMapping{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("device_guid = ?", guid).Delete(&domains.TCPMapping{}).Error; err != nil {
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
	if device.Status == domains.DeviceStatusRegistered || device.Status == domains.DeviceStatusDisabled {
		if _, err := s.tokenService().CreateToken(device.Guid, CreateDeviceTokenRequest{Name: device.Sncode}); err != nil {
			return err
		}
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

func (s DeviceService) hydrateDeviceDetail(device *domains.Device) {
	if device == nil || strings.TrimSpace(device.Guid) == "" {
		return
	}
	if webDomain := s.deviceWebMappingDomain(*device); webDomain != "" {
		device.WebDomain = webDomain
	}
	if device.LastMetricAt <= 0 {
		device.LastMetricAt = s.lastMetricAt(device.Guid)
	}
}

func (s DeviceService) deviceWebMappingDomain(device domains.Device) string {
	var mappings []domains.PortMapping
	if err := s.DB().
		Where("device_guid = ? AND status = ?", device.Guid, int(domains.StatusEnabled)).
		Order("update_time DESC, id DESC").
		Find(&mappings).Error; err != nil {
		return firstDeviceText(publicHost(device.Sncode, device.WebDomain), device.WebDomain)
	}
	hosts := make([]string, 0, len(mappings))
	seen := map[string]struct{}{}
	for _, mapping := range mappings {
		host := strings.TrimSpace(mapping.PublicHost)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}
	if len(hosts) > 0 {
		return strings.Join(hosts, ", ")
	}
	return firstDeviceText(publicHost(device.Sncode, device.WebDomain), device.WebDomain)
}

func (s DeviceService) lastMetricAt(deviceGuid string) int64 {
	var lastHeartbeat domains.DeviceHeartbeat
	last := int64(0)
	if err := s.DB().
		Where("device_guid = ?", deviceGuid).
		Order("create_time DESC").
		Limit(1).
		First(&lastHeartbeat).Error; err == nil && lastHeartbeat.CreateTime > last {
		last = lastHeartbeat.CreateTime
	}
	var trafficState domains.DeviceTrafficState
	if err := s.DB().
		Where("device_guid = ?", deviceGuid).
		Order("sample_time DESC, update_time DESC").
		Limit(1).
		First(&trafficState).Error; err == nil {
		if trafficState.SampleTime > last {
			last = trafficState.SampleTime
		}
		if trafficState.UpdateTime > last {
			last = trafficState.UpdateTime
		}
	}
	return last
}

func splitDeviceWebDomains(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n' || r == '\t'
	})
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func firstDeviceText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func (s DeviceService) Authenticate(token, guid, sncode, sourceIP, hostIP, wanIP, hostname, clientVersion string) (*domains.Device, error) {
	req := HeartbeatRequest{
		Token:         strings.TrimSpace(token),
		Guid:          strings.TrimSpace(guid),
		SnCode:        strings.TrimSpace(sncode),
		HostIP:        strings.TrimSpace(hostIP),
		WanIP:         strings.TrimSpace(wanIP),
		Hostname:      strings.TrimSpace(hostname),
		ClientVersion: strings.TrimSpace(clientVersion),
	}
	return s.Heartbeat(req, sourceIP)
}

func (s DeviceService) TouchOnline(guid string) {
	guid = strings.TrimSpace(guid)
	if guid == "" || s.DB() == nil {
		return
	}
	now := domains.NowMilli()
	cutoff := now - deviceOnlineTouchMinInterval.Milliseconds()
	_ = s.DB().Model(&domains.Device{}).
		Where("guid = ? AND status <> ?", guid, domains.DeviceStatusDisabled).
		Where("status <> ? OR last_seen_time = 0 OR last_seen_time < ?", domains.DeviceStatusOnline, cutoff).
		Updates(map[string]any{
			"status":         domains.DeviceStatusOnline,
			"last_seen_time": now,
			"update_time":    now,
		}).Error
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
		timeout = deviceHeartbeatTimeout()
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

func (s DeviceService) RecordStaleOfflineEvents(timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	now := domains.NowMilli()
	cutoff := now - timeout.Milliseconds()
	var devices []domains.Device
	if err := s.DB().
		Where("status = ?", domains.DeviceStatusOffline).
		Where("last_seen_time = 0 OR last_seen_time < ?", cutoff).
		Find(&devices).Error; err != nil {
		return 0, err
	}
	var created int64
	for _, device := range devices {
		if strings.TrimSpace(device.Guid) == "" || s.hasDeviceOfflineEventSinceLastSeen(device.Guid, device.LastSeenTime) {
			continue
		}
		message := "设备心跳超过 300 秒未更新，请检查设备网络或 navmesh-client 运行状态。"
		if timeout != 300*time.Second {
			message = fmt.Sprintf("设备心跳超过 %s 未更新，请检查设备网络或 navmesh-client 运行状态。", timeout)
		}
		if !ServiceGroupApp.EventService.WithDB(s.DB()).RecordSuppressed(EventInput{
			DeviceGuid: device.Guid,
			EventType:  deviceOfflineEventType,
			Level:      "warn",
			Title:      "device heartbeat offline",
			Message:    message,
		}, deviceOfflineEventSuppressionWindow) {
			continue
		}
		created++
	}
	return created, nil
}

func (s DeviceService) StartOfflineCleaner(ctx context.Context) {
	timeout := deviceHeartbeatTimeout()
	eventDelay := deviceOfflineEventDelay()
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
			created, err := s.RecordStaleOfflineEvents(eventDelay)
			if err != nil {
				global.NAV_LOG.Warn("record stale offline device events failed", zap.Error(err))
				continue
			}
			if created > 0 {
				global.NAV_LOG.Info("record stale offline device events", zap.Int64("created", created), zap.Duration("delay", eventDelay))
			}
		}
	}
}

func normalizeRegisterRequest(req RegisterDeviceRequest) RegisterDeviceRequest {
	req.Token = strings.TrimSpace(req.Token)
	req.Guid = strings.TrimSpace(req.Guid)
	req.SnCode = strings.TrimSpace(req.SnCode)
	req.DeviceType = strings.TrimSpace(req.DeviceType)
	req.Alias = strings.TrimSpace(req.Alias)
	req.Remark = strings.TrimSpace(req.Remark)
	req.Hostname = strings.TrimSpace(req.Hostname)
	req.HostIP = strings.TrimSpace(req.HostIP)
	req.WanIP = normalizeIPv4(req.WanIP)
	req.ClientVersion = strings.TrimSpace(req.ClientVersion)
	req.OS = strings.TrimSpace(req.OS)
	req.OSVersion = strings.TrimSpace(req.OSVersion)
	req.Kernel = strings.TrimSpace(req.Kernel)
	req.Arch = strings.TrimSpace(req.Arch)
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

func resolveDeviceWanIP(wanIP, sourceIP string) string {
	if ip := normalizeIPv4(wanIP); ip != "" {
		return ip
	}
	return normalizeIPv4(sourceIP)
}

func normalizeIPv4(value string) string {
	ip := net.ParseIP(normalizeIP(value))
	if ip == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String()
	}
	return ""
}

func normalizeIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func resolveDeviceLocation(ip string) string {
	ip = normalizeIP(ip)
	if ip == "" {
		return ""
	}
	domestic, province, globalLocation, err := ip2geo.GetIpLocation(ip)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, value := range []string{globalLocation, province, domestic} {
		value = strings.TrimSpace(value)
		if value == "" || value == "0" || value == "未知" {
			continue
		}
		if len(parts) == 0 || parts[len(parts)-1] != value {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " ")
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

func ensureSncodeAvailable(sncode, currentGuid string) error {
	if sncode == "" {
		return errors.New("sncode required")
	}
	var existing domains.Device
	err := ServiceGroupApp.DeviceService.DB().Where("sn_code = ?", sncode).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.Guid != currentGuid {
		return errors.New("sncode already exists")
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

type heartbeatNetworkSnapshot struct {
	NetworkType   string
	NetworkIface  string
	SignalDBM     int
	SignalPct     int
	CellularRSRP  int
	CellularRSRQ  int
	CellularSINR  int
	WifiSSID      string
	WifiRSSI      int
	PingTarget    string
	PingLatencyMs int64
	PingLossPct   float64
	RXRateBps     int64
	TXRateBps     int64
}

func networkSnapshotFromRegister(req RegisterDeviceRequest) heartbeatNetworkSnapshot {
	return normalizeHeartbeatNetworkSnapshot(heartbeatNetworkSnapshot{
		NetworkType:   req.NetworkType,
		NetworkIface:  req.NetworkIface,
		SignalDBM:     req.SignalDBM,
		SignalPct:     req.SignalPct,
		CellularRSRP:  req.CellularRSRP,
		CellularRSRQ:  req.CellularRSRQ,
		CellularSINR:  req.CellularSINR,
		WifiSSID:      req.WifiSSID,
		WifiRSSI:      req.WifiRSSI,
		PingTarget:    req.PingTarget,
		PingLatencyMs: req.PingLatencyMs,
		PingLossPct:   req.PingLossPct,
		RXRateBps:     req.RXRateBps,
		TXRateBps:     req.TXRateBps,
	})
}

func networkSnapshotFromHeartbeat(req HeartbeatRequest) heartbeatNetworkSnapshot {
	return normalizeHeartbeatNetworkSnapshot(heartbeatNetworkSnapshot{
		NetworkType:   req.NetworkType,
		NetworkIface:  req.NetworkIface,
		SignalDBM:     req.SignalDBM,
		SignalPct:     req.SignalPct,
		CellularRSRP:  req.CellularRSRP,
		CellularRSRQ:  req.CellularRSRQ,
		CellularSINR:  req.CellularSINR,
		WifiSSID:      req.WifiSSID,
		WifiRSSI:      req.WifiRSSI,
		PingTarget:    req.PingTarget,
		PingLatencyMs: req.PingLatencyMs,
		PingLossPct:   req.PingLossPct,
		RXRateBps:     req.RXRateBps,
		TXRateBps:     req.TXRateBps,
	})
}

func normalizeHeartbeatNetworkSnapshot(snapshot heartbeatNetworkSnapshot) heartbeatNetworkSnapshot {
	snapshot.NetworkType = normalizeNetworkType(snapshot.NetworkType)
	snapshot.NetworkIface = truncateString(strings.TrimSpace(snapshot.NetworkIface), 64)
	snapshot.WifiSSID = truncateString(strings.TrimSpace(snapshot.WifiSSID), 128)
	snapshot.PingTarget = truncateString(strings.TrimSpace(snapshot.PingTarget), 255)
	if snapshot.SignalPct < 0 {
		snapshot.SignalPct = 0
	}
	if snapshot.SignalPct > 100 {
		snapshot.SignalPct = 100
	}
	if snapshot.PingLatencyMs < 0 {
		snapshot.PingLatencyMs = 0
	}
	if snapshot.PingLossPct < 0 {
		snapshot.PingLossPct = 0
	}
	if snapshot.PingLossPct > 100 {
		snapshot.PingLossPct = 100
	}
	if snapshot.RXRateBps < 0 {
		snapshot.RXRateBps = 0
	}
	if snapshot.TXRateBps < 0 {
		snapshot.TXRateBps = 0
	}
	return snapshot
}

func normalizeNetworkType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "cell", "cellular", "4g", "5g", "lte", "wwan", "mobile":
		return "cellular"
	case "wifi", "wi-fi", "wlan", "wireless":
		return "wifi"
	case "ethernet", "eth", "lan", "wired":
		return "ethernet"
	case "unknown":
		return "unknown"
	default:
		return ""
	}
}

func truncateString(value string, max int) string {
	value = strings.TrimSpace(value)
	if max > 0 && len(value) > max {
		return value[:max]
	}
	return value
}

func applyNetworkSnapshotToDevice(device *domains.Device, snapshot heartbeatNetworkSnapshot) {
	if device == nil {
		return
	}
	device.NetworkType = snapshot.NetworkType
	device.NetworkIface = snapshot.NetworkIface
	device.SignalDBM = snapshot.SignalDBM
	device.SignalPct = snapshot.SignalPct
	device.CellularRSRP = snapshot.CellularRSRP
	device.CellularRSRQ = snapshot.CellularRSRQ
	device.CellularSINR = snapshot.CellularSINR
	device.WifiSSID = snapshot.WifiSSID
	device.WifiRSSI = snapshot.WifiRSSI
	device.PingTarget = snapshot.PingTarget
	device.PingLatencyMs = snapshot.PingLatencyMs
	device.PingLossPct = snapshot.PingLossPct
	device.RXRateBps = snapshot.RXRateBps
	device.TXRateBps = snapshot.TXRateBps
}

func addNetworkSnapshotUpdates(updates map[string]any, snapshot heartbeatNetworkSnapshot) {
	hasSnapshot := hasHeartbeatNetworkSnapshot(snapshot)
	if snapshot.NetworkType != "" {
		updates["network_type"] = snapshot.NetworkType
	}
	if snapshot.NetworkIface != "" {
		updates["network_iface"] = snapshot.NetworkIface
	}
	if snapshot.SignalDBM != 0 {
		updates["signal_dbm"] = snapshot.SignalDBM
	}
	if snapshot.SignalPct > 0 {
		updates["signal_pct"] = snapshot.SignalPct
	}
	if snapshot.CellularRSRP != 0 {
		updates["cellular_rsrp"] = snapshot.CellularRSRP
	}
	if snapshot.CellularRSRQ != 0 {
		updates["cellular_rsrq"] = snapshot.CellularRSRQ
	}
	if snapshot.CellularSINR != 0 {
		updates["cellular_sinr"] = snapshot.CellularSINR
	}
	if snapshot.WifiSSID != "" {
		updates["wifi_ssid"] = snapshot.WifiSSID
	}
	if snapshot.WifiRSSI != 0 {
		updates["wifi_rssi"] = snapshot.WifiRSSI
	}
	if snapshot.PingTarget != "" {
		updates["ping_target"] = snapshot.PingTarget
	}
	if hasSnapshot {
		updates["ping_latency_ms"] = snapshot.PingLatencyMs
		updates["ping_loss_pct"] = snapshot.PingLossPct
		updates["rx_rate_bps"] = snapshot.RXRateBps
		updates["tx_rate_bps"] = snapshot.TXRateBps
	}
}

func hasHeartbeatNetworkSnapshot(snapshot heartbeatNetworkSnapshot) bool {
	return snapshot.NetworkType != "" ||
		snapshot.NetworkIface != "" ||
		snapshot.SignalDBM != 0 ||
		snapshot.SignalPct != 0 ||
		snapshot.CellularRSRP != 0 ||
		snapshot.CellularRSRQ != 0 ||
		snapshot.CellularSINR != 0 ||
		snapshot.WifiSSID != "" ||
		snapshot.WifiRSSI != 0 ||
		snapshot.PingTarget != "" ||
		snapshot.PingLatencyMs != 0 ||
		snapshot.PingLossPct != 0 ||
		snapshot.RXRateBps != 0 ||
		snapshot.TXRateBps != 0
}

func hasRegisterMetrics(req RegisterDeviceRequest) bool {
	return req.MemoryTotal > 0 ||
		req.MemoryUsed > 0 ||
		req.MemoryFree > 0 ||
		req.DiskTotal > 0 ||
		req.DiskUsed > 0 ||
		req.DiskFree > 0 ||
		req.DiskUsedPct > 0 ||
		req.TrafficRXBytes > 0 ||
		req.TrafficTXBytes > 0 ||
		req.TrafficSampleTime > 0 ||
		hasHeartbeatNetworkSnapshot(networkSnapshotFromRegister(req))
}

func hasHeartbeatMetrics(req HeartbeatRequest, snapshot heartbeatNetworkSnapshot) bool {
	return req.MemoryTotal > 0 ||
		req.MemoryUsed > 0 ||
		req.MemoryFree > 0 ||
		req.DiskTotal > 0 ||
		req.DiskUsed > 0 ||
		req.DiskFree > 0 ||
		req.DiskUsedPct > 0 ||
		req.TrafficRXBytes > 0 ||
		req.TrafficTXBytes > 0 ||
		req.TrafficSampleTime > 0 ||
		hasHeartbeatNetworkSnapshot(snapshot)
}

func (s DeviceService) recordHeartbeat(deviceGuid, sourceIP, hostIP, wanIP, location string, network heartbeatNetworkSnapshot, now int64) error {
	return s.DB().Create(&domains.DeviceHeartbeat{
		DeviceGuid:    deviceGuid,
		SourceIP:      sourceIP,
		HostIP:        hostIP,
		WanIP:         wanIP,
		Location:      location,
		NetworkType:   network.NetworkType,
		NetworkIface:  network.NetworkIface,
		SignalDBM:     network.SignalDBM,
		SignalPct:     network.SignalPct,
		CellularRSRP:  network.CellularRSRP,
		CellularRSRQ:  network.CellularRSRQ,
		CellularSINR:  network.CellularSINR,
		WifiSSID:      network.WifiSSID,
		WifiRSSI:      network.WifiRSSI,
		PingTarget:    network.PingTarget,
		PingLatencyMs: network.PingLatencyMs,
		PingLossPct:   network.PingLossPct,
		RXRateBps:     network.RXRateBps,
		TXRateBps:     network.TXRateBps,
		CreateTime:    now,
	}).Error
}

func (s DeviceService) recordDailyDiskUsageEvent(deviceGuid string, req HeartbeatRequest, now int64) {
	if strings.TrimSpace(deviceGuid) == "" || req.DiskUsedPct < diskUsageHighThreshold {
		return
	}
	start, end := localDayRangeMillis(now)
	var count int64
	if err := s.DB().Model(&domains.Event{}).
		Where("device_guid = ? AND event_type = ? AND create_time >= ? AND create_time < ?", deviceGuid, diskUsageHighEventType, start, end).
		Count(&count).Error; err != nil || count > 0 {
		return
	}
	message := fmt.Sprintf("磁盘使用率 %.1f%%，已达到 %.0f%% 告警阈值", req.DiskUsedPct, diskUsageHighThreshold)
	if req.DiskTotal > 0 && req.DiskFree >= 0 {
		message = fmt.Sprintf("%s，剩余 %s / 总量 %s", message, formatStorageSize(req.DiskFree), formatStorageSize(req.DiskTotal))
	}
	ServiceGroupApp.EventService.WithDB(s.DB()).RecordAt(EventInput{
		DeviceGuid: deviceGuid,
		EventType:  diskUsageHighEventType,
		Level:      "warn",
		Title:      "磁盘空间不足",
		Message:    message,
	}, now)
	var device domains.Device
	if err := s.DB().Where("guid = ?", deviceGuid).First(&device).Error; err != nil {
		return
	}
	go ServiceGroupApp.EmailService.NotifyDiskUsageHigh(&device, req, message, now)
}

func (s DeviceService) hasDeviceOfflineEventSinceLastSeen(deviceGuid string, lastSeenTime int64) bool {
	var count int64
	if err := s.DB().Model(&domains.Event{}).
		Where("device_guid = ? AND event_type = ? AND create_time >= ?", deviceGuid, deviceOfflineEventType, lastSeenTime).
		Count(&count).Error; err != nil {
		return true
	}
	return count > 0
}

func localDayRangeMillis(now int64) (int64, int64) {
	t := time.UnixMilli(now).In(time.Local)
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return start.UnixMilli(), start.Add(24 * time.Hour).UnixMilli()
}

func formatStorageSize(value int64) string {
	if value < 0 {
		value = 0
	}
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	size := float64(value)
	for _, suffix := range []string{"KB", "MB", "GB", "TB", "PB"} {
		size = size / unit
		if size < unit {
			return fmt.Sprintf("%.1f %s", size, suffix)
		}
	}
	return fmt.Sprintf("%.1f EB", size/unit)
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

func deviceOfflineEventDelay() time.Duration {
	return deviceSettingDuration("device_offline_event_delay", 300*time.Second)
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
