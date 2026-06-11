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

type CreateDeviceUpgradeBatchRequest struct {
	DeviceGuids []string `json:"deviceGuids"`
	Message     string   `json:"message"`
}

type DeviceUpgradeReportRequest struct {
	Token          string `json:"token"`
	TaskGuid       string `json:"taskGuid"`
	DeviceGuid     string `json:"deviceGuid"`
	Sncode         string `json:"sncode"`
	Status         string `json:"status"`
	Progress       int    `json:"progress"`
	DownloadedSize int64  `json:"downloadedSize"`
	Message        string `json:"message"`
	ErrorMessage   string `json:"errorMessage"`
	ClientVersion  string `json:"clientVersion"`
}

type DeviceUpgradeCommand struct {
	TaskGuid    string `json:"taskGuid"`
	ReleaseType string `json:"releaseType"`
	DeviceType  string `json:"deviceType"`
	Version     string `json:"version"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	FileName    string `json:"fileName"`
	DownloadURL string `json:"downloadUrl"`
	Sha256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

type DeviceUpgradeCandidate struct {
	domains.Device
	HasActiveUpgrade      bool   `json:"hasActiveUpgrade"`
	ActiveTaskGuid        string `json:"activeTaskGuid,omitempty"`
	ActiveTaskStatus      int    `json:"activeTaskStatus,omitempty"`
	OnlineUpgradeable     bool   `json:"onlineUpgradeable"`
	UpgradeDisabledReason string `json:"upgradeDisabledReason,omitempty"`
}

type DeviceUpgradeFailure struct {
	DeviceGuid string `json:"deviceGuid"`
	Message    string `json:"message"`
}

type DeviceUpgradeBatchResult struct {
	Batch    domains.DeviceUpgradeBatch  `json:"batch"`
	Summary  DeviceUpgradeBatchSummary   `json:"summary"`
	Tasks    []domains.DeviceUpgradeTask `json:"tasks"`
	Failures []DeviceUpgradeFailure      `json:"failures"`
}

type DeviceUpgradeBatchSummary struct {
	domains.DeviceUpgradeBatch
	PendingCount  int    `json:"pendingCount"`
	RunningCount  int    `json:"runningCount"`
	SuccessCount  int    `json:"successCount"`
	FailedCount   int    `json:"failedCount"`
	CanceledCount int    `json:"canceledCount"`
	FinishedCount int    `json:"finishedCount"`
	Progress      int    `json:"progress"`
	Status        string `json:"status"`
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
	task, err := s.buildTask(device, *release, "", strings.TrimSpace(req.Message), downloadURL, false)
	if err != nil {
		return nil, err
	}
	if err := s.DB().Create(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s DeviceUpgradeService) Candidates(releaseGuid string) ([]DeviceUpgradeCandidate, error) {
	release, err := ServiceGroupApp.ReleaseService.WithDB(s.DB()).GetEnabled(releaseGuid)
	if err != nil {
		return nil, err
	}
	if !isOnlineUpgradeableReleaseType(release.ReleaseType) {
		return nil, errors.New("release type does not support online upgrade")
	}
	var devices []domains.Device
	if err := s.DB().
		Where("status = ?", domains.DeviceStatusOnline).
		Order("update_time DESC").
		Find(&devices).Error; err != nil {
		return nil, err
	}
	var activeTasks []domains.DeviceUpgradeTask
	if err := s.DB().
		Where("status IN ?", []int{domains.DeviceUpgradeStatusPending, domains.DeviceUpgradeStatusRunning}).
		Find(&activeTasks).Error; err != nil {
		return nil, err
	}
	activeMap := map[string]domains.DeviceUpgradeTask{}
	for _, task := range activeTasks {
		activeMap[task.DeviceGuid] = task
	}
	candidates := make([]DeviceUpgradeCandidate, 0, len(devices))
	for _, device := range devices {
		if !releaseMatchesDeviceType(*release, device) {
			continue
		}
		item := DeviceUpgradeCandidate{Device: device}
		if task, ok := activeMap[device.Guid]; ok {
			item.HasActiveUpgrade = true
			item.ActiveTaskGuid = task.Guid
			item.ActiveTaskStatus = task.Status
		}
		item.OnlineUpgradeable = true
		if reason := releaseUpgradeDisabledReason(*release, device); reason != "" {
			item.OnlineUpgradeable = false
			item.UpgradeDisabledReason = reason
		}
		candidates = append(candidates, item)
	}
	return candidates, nil
}

func (s DeviceUpgradeService) CreateBatch(releaseGuid string, req CreateDeviceUpgradeBatchRequest, downloadURL string) (*DeviceUpgradeBatchResult, error) {
	releaseGuid = strings.TrimSpace(releaseGuid)
	if releaseGuid == "" {
		return nil, errors.New("release guid required")
	}
	release, err := ServiceGroupApp.ReleaseService.WithDB(s.DB()).GetEnabled(releaseGuid)
	if err != nil {
		return nil, err
	}
	if !isOnlineUpgradeableReleaseType(release.ReleaseType) {
		return nil, errors.New("release type does not support online upgrade")
	}
	deviceGuids := uniqueTrimmedStrings(req.DeviceGuids)
	if len(deviceGuids) == 0 {
		return nil, errors.New("device guids required")
	}
	now := domains.NowMilli()
	batch := domains.DeviceUpgradeBatch{
		BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now, UpdateTime: now},
		ReleaseGuid:    release.Guid,
		ReleaseType:    release.ReleaseType,
		DeviceType:     release.DeviceType,
		Version:        release.Version,
		OS:             release.OS,
		Arch:           release.Arch,
		FileName:       release.FileName,
		Message:        strings.TrimSpace(req.Message),
	}
	result := &DeviceUpgradeBatchResult{}
	err = s.DB().Transaction(func(tx *gorm.DB) error {
		svc := s.WithDB(tx)
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		result.Batch = batch
		for _, guid := range deviceGuids {
			var device domains.Device
			if err := tx.Where("guid = ?", guid).First(&device).Error; err != nil {
				result.Failures = append(result.Failures, DeviceUpgradeFailure{DeviceGuid: guid, Message: "device not found"})
				continue
			}
			task, err := svc.buildTask(device, *release, batch.Guid, batch.Message, downloadURL, true)
			if err != nil {
				result.Failures = append(result.Failures, DeviceUpgradeFailure{DeviceGuid: guid, Message: err.Error()})
				continue
			}
			if err := tx.Create(&task).Error; err != nil {
				return err
			}
			result.Tasks = append(result.Tasks, task)
		}
		if len(result.Tasks) == 0 {
			return errors.New("no upgrade tasks created")
		}
		batch.TotalCount = len(result.Tasks)
		batch.UpdateTime = domains.NowMilli()
		if err := tx.Model(&domains.DeviceUpgradeBatch{}).Where("guid = ?", batch.Guid).Updates(map[string]any{
			"total_count": batch.TotalCount,
			"update_time": batch.UpdateTime,
		}).Error; err != nil {
			return err
		}
		result.Batch = batch
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.Summary = s.batchSummary(result.Batch)
	return result, nil
}

func (s DeviceUpgradeService) List(deviceGuid string, params map[string]string) ([]domains.DeviceUpgradeTask, int64, error) {
	db := s.DB().Model(&domains.DeviceUpgradeTask{})
	if deviceGuid = strings.TrimSpace(deviceGuid); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	if batchGuid := strings.TrimSpace(params["batchGuid"]); batchGuid != "" {
		db = db.Where("batch_guid = ?", batchGuid)
	}
	if releaseGuid := strings.TrimSpace(params["releaseGuid"]); releaseGuid != "" {
		db = db.Where("release_guid = ?", releaseGuid)
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

func (s DeviceUpgradeService) ListBatches(params map[string]string) ([]DeviceUpgradeBatchSummary, int64, error) {
	db := s.DB().Model(&domains.DeviceUpgradeBatch{})
	if releaseGuid := strings.TrimSpace(params["releaseGuid"]); releaseGuid != "" {
		db = db.Where("release_guid = ?", releaseGuid)
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
	var batches []domains.DeviceUpgradeBatch
	if err := db.Order("create_time DESC").Limit(size).Offset((page - 1) * size).Find(&batches).Error; err != nil {
		return nil, 0, err
	}
	items := make([]DeviceUpgradeBatchSummary, 0, len(batches))
	for _, batch := range batches {
		items = append(items, s.batchSummary(batch))
	}
	return items, total, nil
}

func (s DeviceUpgradeService) batchSummary(batch domains.DeviceUpgradeBatch) DeviceUpgradeBatchSummary {
	summary := DeviceUpgradeBatchSummary{DeviceUpgradeBatch: batch}
	var rows []struct {
		Status int
		Count  int
	}
	_ = s.DB().Model(&domains.DeviceUpgradeTask{}).
		Select("status, COUNT(*) AS count").
		Where("batch_guid = ?", batch.Guid).
		Group("status").
		Scan(&rows).Error
	total := 0
	for _, row := range rows {
		total += row.Count
		switch row.Status {
		case domains.DeviceUpgradeStatusPending:
			summary.PendingCount = row.Count
		case domains.DeviceUpgradeStatusRunning:
			summary.RunningCount = row.Count
		case domains.DeviceUpgradeStatusSuccess:
			summary.SuccessCount = row.Count
		case domains.DeviceUpgradeStatusFailed:
			summary.FailedCount = row.Count
		case domains.DeviceUpgradeStatusCanceled:
			summary.CanceledCount = row.Count
		}
	}
	if total > 0 {
		summary.TotalCount = total
	}
	summary.FinishedCount = summary.SuccessCount + summary.FailedCount + summary.CanceledCount
	if summary.TotalCount > 0 {
		summary.Progress = clampInt(summary.FinishedCount*100/summary.TotalCount, 0, 100)
	}
	switch {
	case summary.TotalCount == 0:
		summary.Status = "empty"
	case summary.RunningCount > 0:
		summary.Status = "running"
	case summary.PendingCount > 0:
		summary.Status = "pending"
	case summary.FailedCount > 0:
		summary.Status = "failed"
	case summary.CanceledCount > 0:
		summary.Status = "canceled"
	default:
		summary.Status = "success"
	}
	return summary
}

func (s DeviceUpgradeService) PendingCommand(deviceGuid string) (*DeviceUpgradeCommand, error) {
	var task domains.DeviceUpgradeTask
	staleRunningBefore := domains.NowMilli() - int64(deviceUpgradeRunningLease/time.Millisecond)
	result := s.DB().
		Where(
			"device_guid = ? AND (status = ? OR (status = ? AND update_time < ?))",
			strings.TrimSpace(deviceGuid),
			domains.DeviceUpgradeStatusPending,
			domains.DeviceUpgradeStatusRunning,
			staleRunningBefore,
		).
		Order("create_time ASC").
		Limit(1).
		Find(&task)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	var device domains.Device
	if err := s.DB().Where("guid = ?", task.DeviceGuid).First(&device).Error; err == nil {
		release := domains.Release{
			ReleaseType: task.ReleaseType,
			DeviceType:  task.DeviceType,
			OS:          task.OS,
			Arch:        task.Arch,
		}
		if reason := releaseUpgradeDisabledReason(release, device); reason != "" {
			now := domains.NowMilli()
			_ = s.DB().Model(&domains.DeviceUpgradeTask{}).Where("guid = ?", task.Guid).Updates(map[string]any{
				"status":        domains.DeviceUpgradeStatusFailed,
				"error_message": reason,
				"finish_time":   now,
				"update_time":   now,
			}).Error
			return nil, nil
		}
	}
	return &DeviceUpgradeCommand{
		TaskGuid:    task.Guid,
		ReleaseType: task.ReleaseType,
		DeviceType:  task.DeviceType,
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
		"progress":        reportProgress(status, req.Progress, task.Progress),
		"downloaded_size": reportDownloadedSize(req.DownloadedSize, task.DownloadedSize),
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
		if normalizeUpgradeReleaseType(task.ReleaseType) == domains.ReleaseTypeNavmesh {
			_ = s.DB().Model(&domains.Device{}).Where("guid = ?", task.DeviceGuid).Updates(map[string]any{
				"client_version": strings.TrimSpace(req.ClientVersion),
				"update_time":    now,
			}).Error
		}
		ServiceGroupApp.EventService.Record(EventInput{DeviceGuid: task.DeviceGuid, EventType: "client_upgrade", Level: "info", Title: upgradeEventTitle(task.ReleaseType, true), Message: task.Version})
	}
	if status == domains.DeviceUpgradeStatusFailed {
		ServiceGroupApp.EventService.Record(EventInput{DeviceGuid: task.DeviceGuid, EventType: "client_upgrade", Level: "error", Title: upgradeEventTitle(task.ReleaseType, false), Message: strings.TrimSpace(req.ErrorMessage)})
	}
	return nil
}

func (s DeviceUpgradeService) buildTask(device domains.Device, release domains.Release, batchGuid string, message string, downloadURL string, requireOnline bool) (domains.DeviceUpgradeTask, error) {
	release.NormalizeDefaults()
	if requireOnline && device.Status != domains.DeviceStatusOnline {
		return domains.DeviceUpgradeTask{}, errors.New("device is not online")
	}
	if device.Status == domains.DeviceStatusDisabled {
		return domains.DeviceUpgradeTask{}, errors.New("device is disabled")
	}
	if !isOnlineUpgradeableReleaseType(release.ReleaseType) {
		return domains.DeviceUpgradeTask{}, errors.New("release type does not support online upgrade")
	}
	if !releaseMatchesDevice(release, device) {
		return domains.DeviceUpgradeTask{}, errors.New("release does not match device")
	}
	if reason := releaseUpgradeDisabledReason(release, device); reason != "" {
		return domains.DeviceUpgradeTask{}, errors.New(reason)
	}
	var activeCount int64
	if err := s.DB().Model(&domains.DeviceUpgradeTask{}).
		Where("device_guid = ? AND status IN ?", device.Guid, []int{domains.DeviceUpgradeStatusPending, domains.DeviceUpgradeStatusRunning}).
		Count(&activeCount).Error; err != nil {
		return domains.DeviceUpgradeTask{}, err
	}
	if activeCount > 0 {
		return domains.DeviceUpgradeTask{}, errors.New("device already has an active upgrade task")
	}
	now := domains.NowMilli()
	task := domains.DeviceUpgradeTask{
		BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now, UpdateTime: now},
		BatchGuid:      strings.TrimSpace(batchGuid),
		DeviceGuid:     device.Guid,
		ReleaseGuid:    release.Guid,
		ReleaseType:    release.ReleaseType,
		DeviceType:     release.DeviceType,
		Version:        release.Version,
		OS:             release.OS,
		Arch:           release.Arch,
		FileName:       release.FileName,
		DownloadURL:    firstNonEmptyString(release.DownloadURL, downloadURL),
		Sha256:         release.Sha256,
		Size:           release.Size,
		FromVersion:    device.ClientVersion,
		Status:         domains.DeviceUpgradeStatusPending,
		Progress:       0,
		DownloadedSize: 0,
		Message:        strings.TrimSpace(message),
	}
	if task.DownloadURL == "" {
		return domains.DeviceUpgradeTask{}, errors.New("download url required")
	}
	return task, nil
}

func releaseMatchesDevice(release domains.Release, device domains.Device) bool {
	return releaseMatchesDeviceType(release, device) && releaseMatchesDevicePlatform(release, device)
}

func releaseMatchesDeviceType(release domains.Release, device domains.Device) bool {
	release.NormalizeDefaults()
	return sameDeviceType(device.DeviceType, release.DeviceType) || sameDeviceType(device.GroupGuid, release.DeviceType)
}

func releaseMatchesDevicePlatform(release domains.Release, device domains.Device) bool {
	release.NormalizeDefaults()
	return samePlatform(device.OS, release.OS) && samePlatform(device.Arch, release.Arch)
}

func isOnlineUpgradeableReleaseType(value string) bool {
	switch normalizeUpgradeReleaseType(value) {
	case domains.ReleaseTypeNavmesh, domains.ReleaseTypeRain:
		return true
	default:
		return false
	}
}

func releaseUpgradeDisabledReason(release domains.Release, device domains.Device) string {
	if !releaseMatchesDevicePlatform(release, device) {
		return "设备系统或架构与升级包不匹配"
	}
	if normalizeUpgradeReleaseType(release.ReleaseType) == domains.ReleaseTypeRain && !clientVersionAtLeast(device.ClientVersion, 0, 0, 3) {
		return "设备客户端版本不支持北斗降雨在线升级"
	}
	return ""
}

func clientVersionAtLeast(value string, wantMajor int, wantMinor int, wantPatch int) bool {
	major, minor, patch, ok := parseVersionNumbers(value)
	if !ok {
		return false
	}
	if major != wantMajor {
		return major > wantMajor
	}
	if minor != wantMinor {
		return minor > wantMinor
	}
	return patch >= wantPatch
}

func parseVersionNumbers(value string) (int, int, int, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(value), "v"))
	parts := strings.Split(value, ".")
	if len(parts) < 3 {
		return 0, 0, 0, false
	}
	parse := func(part string) (int, bool) {
		part = strings.TrimSpace(part)
		for index, char := range part {
			if char < '0' || char > '9' {
				part = part[:index]
				break
			}
		}
		if part == "" {
			return 0, false
		}
		return utils.Str2Int(part), true
	}
	major, ok := parse(parts[0])
	if !ok {
		return 0, 0, 0, false
	}
	minor, ok := parse(parts[1])
	if !ok {
		return 0, 0, 0, false
	}
	patch, ok := parse(parts[2])
	if !ok {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func normalizeUpgradeReleaseType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "navmesh_client":
		return domains.ReleaseTypeNavmesh
	case domains.ReleaseTypeRain, "device_software":
		return domains.ReleaseTypeRain
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func upgradeEventTitle(releaseType string, success bool) string {
	name := "客户端"
	if normalizeUpgradeReleaseType(releaseType) == domains.ReleaseTypeRain {
		name = "北斗降雨"
	}
	if success {
		return name + "升级成功"
	}
	return name + "升级失败"
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
	value = strings.NewReplacer("_", " ", "-", " ", "/", " ").Replace(value)
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
	for alias, normalized := range aliases {
		if platformNameContainsToken(value, alias) {
			return normalized
		}
	}
	return value
}

func platformNameContainsToken(value string, token string) bool {
	if value == token {
		return true
	}
	fields := strings.Fields(value)
	for _, field := range fields {
		if field == token {
			return true
		}
	}
	return false
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

func reportProgress(status int, progress int, previous int) int {
	switch status {
	case domains.DeviceUpgradeStatusRunning:
		if progress <= 0 {
			if previous > 0 {
				return minInt(previous, 99)
			}
			return 1
		}
		return clampInt(progress, 1, 99)
	case domains.DeviceUpgradeStatusSuccess:
		return 100
	case domains.DeviceUpgradeStatusFailed, domains.DeviceUpgradeStatusCanceled:
		if progress <= 0 {
			return clampInt(previous, 0, 100)
		}
		return clampInt(progress, 0, 100)
	default:
		return clampInt(progress, 0, 100)
	}
}

func reportDownloadedSize(downloaded int64, previous int64) int64 {
	if downloaded < 0 {
		return previous
	}
	if downloaded == 0 && previous > 0 {
		return previous
	}
	return downloaded
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueTrimmedStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
