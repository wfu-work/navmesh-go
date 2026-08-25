package services

import (
	"errors"
	"strconv"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

const (
	defaultTCPPublicPortMin = 20000
	defaultTCPPublicPortMax = 29999
)

type TCPMappingService struct {
	commonServices.CrudService[domains.TCPMapping]
}

type SaveTCPMappingRequest struct {
	Guid       string `json:"guid"`
	DeviceGuid string `json:"deviceGuid"`
	Name       string `json:"name"`
	PublicHost string `json:"publicHost"`
	PublicPort int    `json:"publicPort"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
	Remark     string `json:"remark"`
	Status     int    `json:"status"`
}

type TCPPortRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func (s TCPMappingService) WithDB(db *gorm.DB) TCPMappingService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

func (s TCPMappingService) List(params map[string]string) ([]domains.TCPMapping, int64, error) {
	db := s.DB().Model(&domains.TCPMapping{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR public_host LIKE ? OR device_guid LIKE ?", like, like, like)
	}
	if status := utils.Str2Int(params["status"]); status > 0 {
		db = db.Where("status = ?", status)
	}
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	if publicPort := utils.Str2Int(params["publicPort"]); publicPort > 0 {
		db = db.Where("public_port = ?", publicPort)
	}
	return queryPage[domains.TCPMapping](db, params, DefaultMaxPageSize, "update_time DESC, id DESC")
}

func (s TCPMappingService) Enabled() ([]domains.TCPMapping, error) {
	if strings.EqualFold(getSettingValue("tcp_mapping_enabled", "true"), "false") {
		return []domains.TCPMapping{}, nil
	}
	var items []domains.TCPMapping
	err := s.DB().
		Where("status = ? AND public_port > 0", int(domains.StatusEnabled)).
		Order("public_port ASC").
		Find(&items).Error
	return items, err
}

func (s TCPMappingService) Save(req SaveTCPMappingRequest) (*domains.TCPMapping, error) {
	req = normalizeTCPMappingRequest(req)
	if req.DeviceGuid == "" {
		return nil, errors.New("deviceGuid required")
	}
	if req.Name == "" {
		return nil, errors.New("name required")
	}
	if req.TargetHost == "" {
		return nil, errors.New("targetHost required")
	}
	if req.TargetPort <= 0 || req.TargetPort > 65535 {
		return nil, errors.New("targetPort must be between 1 and 65535")
	}
	if err := ensureDeviceExists(req.DeviceGuid); err != nil {
		return nil, err
	}
	portRange := TCPMappingPortRange()
	if req.PublicPort <= 0 {
		port, err := s.nextAvailablePublicPort(req.Guid, portRange)
		if err != nil {
			return nil, err
		}
		req.PublicPort = port
	}
	if err := validateTCPPublicPort(req.PublicPort, portRange); err != nil {
		return nil, err
	}
	if req.Status != int(domains.StatusDisabled) {
		if err := s.ensurePublicPortAvailable(req.PublicPort, req.Guid); err != nil {
			return nil, err
		}
	}
	if req.PublicHost == "" {
		req.PublicHost = defaultTCPPublicHost(req.Name, req.PublicPort)
	}
	now := domains.NowMilli()
	var row domains.TCPMapping
	err := s.DB().Where("guid = ?", req.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.TCPMapping{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}}
		if req.Guid != "" {
			row.Guid = req.Guid
		}
	}
	row.DeviceGuid = req.DeviceGuid
	row.Name = req.Name
	row.PublicHost = req.PublicHost
	row.PublicPort = req.PublicPort
	row.TargetHost = req.TargetHost
	row.TargetPort = req.TargetPort
	row.Remark = req.Remark
	row.Status = req.Status
	row.UpdateTime = now
	if err := s.DB().Save(&row).Error; err != nil {
		return nil, err
	}
	triggerTCPMappingReload()
	return &row, nil
}

func (s TCPMappingService) Disable(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	err := s.DB().Model(&domains.TCPMapping{}).Where("guid = ?", guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
	if err == nil {
		triggerTCPMappingReload()
	}
	return err
}

func (s TCPMappingService) nextAvailablePublicPort(currentGuid string, portRange TCPPortRange) (int, error) {
	var used []int
	query := s.DB().Model(&domains.TCPMapping{}).
		Where("status = ? AND public_port BETWEEN ? AND ?", int(domains.StatusEnabled), portRange.Min, portRange.Max)
	if strings.TrimSpace(currentGuid) != "" {
		query = query.Where("guid != ?", strings.TrimSpace(currentGuid))
	}
	if err := query.Pluck("public_port", &used).Error; err != nil {
		return 0, err
	}
	usedPorts := make(map[int]struct{}, len(used))
	for _, port := range used {
		usedPorts[port] = struct{}{}
	}
	for port := portRange.Min; port <= portRange.Max; port++ {
		if _, ok := usedPorts[port]; !ok {
			return port, nil
		}
	}
	return 0, errors.New("no available tcp public port")
}

func (s TCPMappingService) ensurePublicPortAvailable(publicPort int, currentGuid string) error {
	var existing domains.TCPMapping
	query := s.DB().
		Where("public_port = ? AND status = ?", publicPort, int(domains.StatusEnabled))
	if strings.TrimSpace(currentGuid) != "" {
		query = query.Where("guid != ?", strings.TrimSpace(currentGuid))
	}
	err := query.First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("publicPort already exists")
}

func normalizeTCPMappingRequest(req SaveTCPMappingRequest) SaveTCPMappingRequest {
	req.Guid = strings.TrimSpace(req.Guid)
	req.DeviceGuid = strings.TrimSpace(req.DeviceGuid)
	req.Name = strings.TrimSpace(req.Name)
	req.PublicHost = normalizeHost(req.PublicHost)
	req.TargetHost = strings.TrimSpace(req.TargetHost)
	req.Remark = strings.TrimSpace(req.Remark)
	if req.TargetHost == "" {
		req.TargetHost = "127.0.0.1"
	}
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	return req
}

func TCPMappingPortRange() TCPPortRange {
	min := settingIntValue("tcp_public_port_min", defaultTCPPublicPortMin)
	max := settingIntValue("tcp_public_port_max", defaultTCPPublicPortMax)
	if min <= 0 || min > 65535 {
		min = defaultTCPPublicPortMin
	}
	if max <= 0 || max > 65535 {
		max = defaultTCPPublicPortMax
	}
	if min > max {
		min, max = defaultTCPPublicPortMin, defaultTCPPublicPortMax
	}
	return TCPPortRange{Min: min, Max: max}
}

func validateTCPPublicPort(port int, portRange TCPPortRange) error {
	if port <= 0 || port > 65535 {
		return errors.New("publicPort must be between 1 and 65535")
	}
	if port < portRange.Min || port > portRange.Max {
		return errors.New("publicPort outside configured tcp public port range")
	}
	return nil
}

func defaultTCPPublicHost(name string, publicPort int) string {
	label := dnsLabel(name)
	if label == "" {
		label = "tcp-" + strconv.Itoa(publicPort)
	}
	publicDomain := strings.TrimSpace(getSettingValue("tcp_gateway_domain", "tcpd.navfirst.com"))
	if publicDomain == "" {
		return label
	}
	return label + "." + strings.TrimPrefix(publicDomain, ".")
}

func dnsLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 63 {
		result = strings.Trim(result[:63], "-")
	}
	return result
}

var tcpMappingReloader func()

func RegisterTCPMappingReloader(reloader func()) {
	tcpMappingReloader = reloader
}

func triggerTCPMappingReload() {
	if tcpMappingReloader != nil {
		go tcpMappingReloader()
	}
}
