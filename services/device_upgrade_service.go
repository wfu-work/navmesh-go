package services

import (
	"errors"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

type DeviceUpgradeService struct {
	commonServices.CrudService[domains.DeviceUpgradeTask]
}

const deviceUpgradeRunningLease = 15 * time.Minute

func (s DeviceUpgradeService) WithDB(db *gorm.DB) DeviceUpgradeService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type CreateDeviceUpgradeRequest struct {
	ReleaseGuid string `json:"releaseGuid"`
	Message     string `json:"message"`
}

type DeviceUpgradeReportRequest struct {
	Token         string `json:"token"`
	TaskGuid      string `json:"taskGuid"`
	DeviceGuid    string `json:"deviceGuid"`
	Sncode        string `json:"sncode"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	ErrorMessage  string `json:"errorMessage"`
	ClientVersion string `json:"clientVersion"`
}

type DeviceUpgradeCommand struct {
	TaskGuid    string `json:"taskGuid"`
	Version     string `json:"version"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	FileName    string `json:"fileName"`
	DownloadURL string `json:"downloadUrl"`
	Sha256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

func (s DeviceUpgradeService) CreateTask(deviceGuid string, req CreateDeviceUpgradeRequest, downloadURL string) (*domains.DeviceUpgradeTask, error) {
	deviceGuid = strings.TrimSpace(deviceGuid)
	req.ReleaseGuid = strings.TrimSpace(req.ReleaseGuid)
	if deviceGuid == "" {
		return nil, errors.New("device guid required")
	}
	if req.ReleaseGuid == "" {
		return nil, errors.New("release guid required")
	}
	var device domains.Device
	if err := s.DB().Where("guid = ?", deviceGuid).First(&device).Error; err != nil {
		return nil, errors.New("device not found")
	}
	release, err := ServiceGroupApp.ReleaseService.WithDB(s.DB()).GetEnabled(req.ReleaseGuid)
	if err != nil {
		return nil, err
	}
	if device.Status == domains.DeviceStatusDisabled {
		return nil, errors.New("device is disabled")
	}
	if release.ReleaseType != domains.ReleaseTypeNavmesh {
		return nil, errors.New("release is not a navmesh-client package")
	}
	if !sameDeviceType(device.DeviceType, release.DeviceType) && !sameDeviceType(device.GroupGuid, release.DeviceType) {
		return nil, errors.New("release device type does not match device")
	}
	if !samePlatform(device.OS, release.OS) || !samePlatform(device.Arch, release.Arch) {
		return nil, errors.New("release platform does not match device")
	}
	var activeCount int64
	if err := s.DB().Model(&domains.DeviceUpgradeTask{}).
		Where("device_guid = ? AND status IN ?", device.Guid, []int{domains.DeviceUpgradeStatusPending, domains.DeviceUpgradeStatusRunning}).
		Count(&activeCount).Error; err != nil {
		return nil, err
	}
	if activeCount > 0 {
		return nil, errors.New("device already has an active upgrade task")
	}
	now := domains.NowMilli()
	task := domains.DeviceUpgradeTask{
		BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now, UpdateTime: now},
		DeviceGuid:     device.Guid,
		ReleaseGuid:    release.Guid,
		Version:        release.Version,
		OS:             release.OS,
		Arch:           release.Arch,
		FileName:       release.FileName,
		DownloadURL:    firstNonEmptyString(release.DownloadURL, downloadURL),
		Sha256:         release.Sha256,
		Size:           release.Size,
		FromVersion:    device.ClientVersion,
		Status:         domains.DeviceUpgradeStatusPending,
		Message:        strings.TrimSpace(req.Message),
	}
	if task.DownloadURL == "" {
		return nil, errors.New("download url required")
	}
	if err := s.DB().Create(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s DeviceUpgradeService) List(deviceGuid string, params map[string]string) ([]domains.DeviceUpgradeTask, int64, error) {
	db := s.DB().Model(&domains.DeviceUpgradeTask{})
	if deviceGuid = strings.TrimSpace(deviceGuid); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
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
	var items []domains.DeviceUpgradeTask
	err := db.Order("create_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (s DeviceUpgradeService) PendingCommand(deviceGuid string) (*DeviceUpgradeCommand, error) {
	var task domains.DeviceUpgradeTask
	staleRunningBefore := domains.NowMilli() - int64(deviceUpgradeRunningLease/time.Millisecond)
	err := s.DB().
		Where(
			"device_guid = ? AND (status = ? OR (status = ? AND start_time > 0 AND start_time < ?))",
			strings.TrimSpace(deviceGuid),
			domains.DeviceUpgradeStatusPending,
			domains.DeviceUpgradeStatusRunning,
			staleRunningBefore,
		).
		Order("create_time ASC").
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &DeviceUpgradeCommand{
		TaskGuid:    task.Guid,
		Version:     task.Version,
		OS:          task.OS,
		Arch:        task.Arch,
		FileName:    task.FileName,
		DownloadURL: task.DownloadURL,
		Sha256:      task.Sha256,
		Size:        task.Size,
	}, nil
}

func (s DeviceUpgradeService) Report(req DeviceUpgradeReportRequest) error {
	req.TaskGuid = strings.TrimSpace(req.TaskGuid)
	req.Token = strings.TrimSpace(req.Token)
	if req.TaskGuid == "" {
		return errors.New("task guid required")
	}
	if req.Token == "" {
		return errors.New("token required")
	}
	var task domains.DeviceUpgradeTask
	if err := s.DB().Where("guid = ?", req.TaskGuid).First(&task).Error; err != nil {
		return errors.New("upgrade task not found")
	}
	if err := ServiceGroupApp.DeviceTokenService.WithDB(s.DB()).Validate(task.DeviceGuid, req.Token); err != nil {
		return err
	}
	status := reportStatus(req.Status)
	if status == 0 {
		return errors.New("invalid upgrade status")
	}
	if isUpgradeTerminal(task.Status) {
		return nil
	}
	now := domains.NowMilli()
	updates := map[string]any{
		"status":          status,
		"message":         strings.TrimSpace(req.Message),
		"error_message":   strings.TrimSpace(req.ErrorMessage),
		"current_version": strings.TrimSpace(req.ClientVersion),
		"update_time":     now,
	}
	if status == domains.DeviceUpgradeStatusRunning && task.StartTime == 0 {
		updates["start_time"] = now
	}
	if status == domains.DeviceUpgradeStatusSuccess || status == domains.DeviceUpgradeStatusFailed || status == domains.DeviceUpgradeStatusCanceled {
		updates["finish_time"] = now
	}
	if err := s.DB().Model(&domains.DeviceUpgradeTask{}).Where("guid = ?", task.Guid).Updates(updates).Error; err != nil {
		return err
	}
	if status == domains.DeviceUpgradeStatusSuccess {
		_ = s.DB().Model(&domains.Device{}).Where("guid = ?", task.DeviceGuid).Updates(map[string]any{
			"client_version": strings.TrimSpace(req.ClientVersion),
			"update_time":    now,
		}).Error
		ServiceGroupApp.EventService.Record(EventInput{DeviceGuid: task.DeviceGuid, EventType: "client_upgrade", Level: "info", Title: "客户端升级成功", Message: task.Version})
	}
	if status == domains.DeviceUpgradeStatusFailed {
		ServiceGroupApp.EventService.Record(EventInput{DeviceGuid: task.DeviceGuid, EventType: "client_upgrade", Level: "error", Title: "客户端升级失败", Message: strings.TrimSpace(req.ErrorMessage)})
	}
	return nil
}

func samePlatform(current string, target string) bool {
	current = normalizePlatformName(current)
	target = normalizePlatformName(target)
	return current == "" || target == "" || target == "all" || current == target
}

func sameDeviceType(current string, target string) bool {
	current = strings.TrimSpace(current)
	target = strings.TrimSpace(target)
	return target == "" || target == "all" || current == "" || current == target
}

func normalizePlatformName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	aliases := map[string]string{
		"aarch64":   "arm64",
		"armv8":     "arm64",
		"x86_64":    "amd64",
		"x64":       "amd64",
		"macos":     "darwin",
		"osx":       "darwin",
		"win32":     "windows",
		"win64":     "windows",
		"ubuntu":    "linux",
		"debian":    "linux",
		"centos":    "linux",
		"rhel":      "linux",
		"redhat":    "linux",
		"fedora":    "linux",
		"rocky":     "linux",
		"almalinux": "linux",
		"opensuse":  "linux",
		"suse":      "linux",
		"alpine":    "linux",
	}
	if normalized, ok := aliases[value]; ok {
		return normalized
	}
	return value
}

func isUpgradeTerminal(status int) bool {
	return status == domains.DeviceUpgradeStatusSuccess ||
		status == domains.DeviceUpgradeStatusFailed ||
		status == domains.DeviceUpgradeStatusCanceled
}

func reportStatus(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "running":
		return domains.DeviceUpgradeStatusRunning
	case "success", "succeeded":
		return domains.DeviceUpgradeStatusSuccess
	case "failed", "error":
		return domains.DeviceUpgradeStatusFailed
	case "canceled", "cancelled":
		return domains.DeviceUpgradeStatusCanceled
	default:
		return 0
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
