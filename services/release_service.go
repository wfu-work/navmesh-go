package services

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"github.com/wfu-work/nav-common-go-lib/global"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

type ReleaseService struct {
	commonServices.CrudService[domains.Release]
}

func (s ReleaseService) WithDB(db *gorm.DB) ReleaseService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type UploadReleaseRequest struct {
	ReleaseType string
	DeviceType  string
	Version     string
	OS          string
	Arch        string
	DownloadURL string
	ChangeLog   string
}

func (s ReleaseService) Upload(file *multipart.FileHeader, req UploadReleaseRequest) (*domains.Release, error) {
	if file == nil {
		return nil, errors.New("file required")
	}
	fileName := safeReleaseFileName(file.Filename)
	if fileName == "" {
		return nil, errors.New("invalid release filename")
	}
	req, err := normalizeUploadReleaseRequest(fileName, file, req)
	if err != nil {
		return nil, err
	}
	fileName, dstPath, sha, size, err := saveReleaseFile(file, req)
	if err != nil {
		return nil, err
	}
	now := domains.NowMilli()
	row := domains.Release{
		BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now, UpdateTime: now},
		ReleaseType:    req.ReleaseType,
		DeviceType:     req.DeviceType,
		Version:        req.Version,
		OS:             req.OS,
		Arch:           req.Arch,
		FileName:       fileName,
		FilePath:       dstPath,
		Sha256:         sha,
		Size:           size,
		DownloadURL:    req.DownloadURL,
		ChangeLog:      req.ChangeLog,
		Status:         domains.ReleaseStatusEnabled,
	}
	var existing domains.Release
	if err := s.DB().
		Where("release_type = ? AND device_type = ? AND version = ? AND os = ? AND arch = ? AND file_name = ?", req.ReleaseType, req.DeviceType, req.Version, req.OS, req.Arch, fileName).
		First(&existing).Error; err == nil {
		row.BaseDataEntity = existing.BaseDataEntity
		row.UpdateTime = now
		row.Guid = existing.Guid
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := s.DB().Save(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s ReleaseService) Get(guid string) (*domains.Release, error) {
	var row domains.Release
	if err := s.DB().Where("guid = ?", strings.TrimSpace(guid)).First(&row).Error; err != nil {
		return nil, errors.New("release not found")
	}
	row.NormalizeDefaults()
	return &row, nil
}

func (s ReleaseService) Update(guid string, file *multipart.FileHeader, req UploadReleaseRequest) (*domains.Release, error) {
	row, err := s.Get(guid)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ReleaseType) == "" {
		req.ReleaseType = row.ReleaseType
	}
	if strings.TrimSpace(req.DeviceType) == "" {
		req.DeviceType = row.DeviceType
	}
	if strings.TrimSpace(req.Version) == "" {
		req.Version = row.Version
	}
	if strings.TrimSpace(req.OS) == "" {
		req.OS = row.OS
	}
	if strings.TrimSpace(req.Arch) == "" {
		req.Arch = row.Arch
	}
	fileName := row.FileName
	if file != nil {
		fileName = safeReleaseFileName(file.Filename)
		if fileName == "" {
			return nil, errors.New("invalid release filename")
		}
	}
	req, err = normalizeUploadReleaseRequest(fileName, file, req)
	if err != nil {
		return nil, err
	}
	row.ReleaseType = req.ReleaseType
	row.DeviceType = req.DeviceType
	row.Version = req.Version
	row.OS = req.OS
	row.Arch = req.Arch
	row.DownloadURL = req.DownloadURL
	row.ChangeLog = req.ChangeLog
	row.UpdateTime = domains.NowMilli()
	if file != nil {
		fileName, filePath, sha, size, err := saveReleaseFile(file, req)
		if err != nil {
			return nil, err
		}
		row.FileName = fileName
		row.FilePath = filePath
		row.Sha256 = sha
		row.Size = size
	}
	if err := s.DB().Save(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s ReleaseService) SetStatus(guid string, status int) error {
	if strings.TrimSpace(guid) == "" {
		return errors.New("release guid required")
	}
	return s.DB().Model(&domains.Release{}).
		Where("guid = ?", strings.TrimSpace(guid)).
		Updates(map[string]any{"status": status, "update_time": domains.NowMilli()}).Error
}

func (s ReleaseService) Delete(guid string) error {
	row, err := s.Get(guid)
	if err != nil {
		return err
	}
	filePath := row.FilePath
	if err := s.DB().Unscoped().Where("guid = ?", row.Guid).Delete(&domains.Release{}).Error; err != nil {
		return err
	}
	if strings.TrimSpace(filePath) != "" {
		var count int64
		if err := s.DB().Model(&domains.Release{}).Where("file_path = ?", filePath).Count(&count).Error; err == nil && count == 0 {
			_ = os.Remove(filePath)
		}
	}
	return nil
}

func (s ReleaseService) List(params map[string]string) ([]domains.Release, int64, error) {
	db := s.DB().Model(&domains.Release{})
	if releaseType := normalizeReleaseType(params["releaseType"]); releaseType != "" {
		db = db.Where("release_type = ? OR (release_type = '' AND ? = ?)", releaseType, releaseType, domains.ReleaseTypeNavmesh)
	}
	if deviceType := strings.TrimSpace(params["deviceType"]); deviceType != "" {
		db = db.Where("device_type = ?", normalizeDeviceType(deviceType))
	}
	if version := strings.TrimSpace(params["version"]); version != "" {
		db = db.Where("version LIKE ?", "%"+version+"%")
	}
	if osName := strings.TrimSpace(params["os"]); osName != "" {
		db = db.Where("os = ?", osName)
	}
	if arch := strings.TrimSpace(params["arch"]); arch != "" {
		db = db.Where("arch = ?", arch)
	}
	if statusParam := strings.TrimSpace(params["status"]); statusParam != "" {
		db = db.Where("status = ?", utils.Str2Int(statusParam))
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
	var items []domains.Release
	err := db.Order("update_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	for index := range items {
		items[index].NormalizeDefaults()
	}
	return items, total, err
}

func (s ReleaseService) GetEnabled(guid string) (*domains.Release, error) {
	var row domains.Release
	if err := s.DB().Where("guid = ? AND status = ?", strings.TrimSpace(guid), domains.ReleaseStatusEnabled).First(&row).Error; err != nil {
		return nil, errors.New("release not found")
	}
	row.NormalizeDefaults()
	return &row, nil
}

func (s ReleaseService) FindDownload(fileName string) (*domains.Release, error) {
	var row domains.Release
	if err := s.DB().
		Where("file_name = ? AND status = ?", safeReleaseFileName(fileName), domains.ReleaseStatusEnabled).
		Order("update_time DESC").
		First(&row).Error; err != nil {
		return nil, errors.New("release not found")
	}
	return ensureDownloadableRelease(&row)
}

func (s ReleaseService) FindDownloadByGuid(guid string) (*domains.Release, error) {
	var row domains.Release
	if err := s.DB().Where("guid = ? AND status = ?", strings.TrimSpace(guid), domains.ReleaseStatusEnabled).First(&row).Error; err != nil {
		return nil, errors.New("release not found")
	}
	return ensureDownloadableRelease(&row)
}

func (s ReleaseService) FindLatestDownload(params map[string]string) (*domains.Release, error) {
	releaseType := normalizeReleaseType(params["releaseType"])
	if releaseType == "" {
		releaseType = domains.ReleaseTypeNavmesh
	}
	deviceType := normalizeDeviceType(params["deviceType"])
	osName := normalizePlatformName(params["os"])
	arch := normalizePlatformName(params["arch"])
	var rows []domains.Release
	if err := s.DB().
		Where("release_type = ? AND status = ?", releaseType, domains.ReleaseStatusEnabled).
		Order("update_time DESC").
		Limit(200).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for index := range rows {
		row := &rows[index]
		row.NormalizeDefaults()
		if deviceType != "all" && !sameDeviceType(deviceType, row.DeviceType) {
			continue
		}
		if !samePlatform(osName, row.OS) || !samePlatform(arch, row.Arch) {
			continue
		}
		return ensureDownloadableRelease(row)
	}
	return nil, errors.New("release not found")
}

func ensureDownloadableRelease(row *domains.Release) (*domains.Release, error) {
	if _, err := os.Stat(row.FilePath); err != nil {
		return nil, errors.New("release file missing")
	}
	row.NormalizeDefaults()
	return row, nil
}

func normalizeUploadReleaseRequest(fileName string, file *multipart.FileHeader, req UploadReleaseRequest) (UploadReleaseRequest, error) {
	req.ReleaseType = normalizeReleaseType(req.ReleaseType)
	req.DeviceType = normalizeDeviceType(req.DeviceType)
	req.Version = strings.TrimSpace(req.Version)
	req.OS = strings.TrimSpace(req.OS)
	req.Arch = strings.TrimSpace(req.Arch)
	req.DownloadURL = strings.TrimSpace(req.DownloadURL)
	req.ChangeLog = strings.TrimSpace(req.ChangeLog)
	if req.Version == "" {
		return req, errors.New("version required")
	}
	if req.ReleaseType == domains.ReleaseTypeNavmesh && !strings.HasPrefix(fileName, "navmesh-client-") {
		return req, errors.New("invalid navmesh-client binary filename")
	}
	if req.OS == "" || req.Arch == "" {
		parsedOS, parsedArch := parseReleasePlatform(fileName)
		if parsedOS == "" || parsedArch == "" {
			contentOS, contentArch := parseReleaseContentPlatform(file)
			if parsedOS == "" {
				parsedOS = contentOS
			}
			if parsedArch == "" {
				parsedArch = contentArch
			}
		}
		if req.OS == "" {
			req.OS = parsedOS
		}
		if req.Arch == "" {
			req.Arch = parsedArch
		}
	}
	if req.ReleaseType == domains.ReleaseTypeNavmesh && (req.OS == "" || req.Arch == "") {
		return req, errors.New("os and arch required")
	}
	if req.OS == "" {
		req.OS = "all"
	}
	if req.Arch == "" {
		req.Arch = "all"
	}
	return req, nil
}

func saveReleaseFile(file *multipart.FileHeader, req UploadReleaseRequest) (string, string, string, int64, error) {
	fileName := safeReleaseFileName(file.Filename)
	if fileName == "" {
		return "", "", "", 0, errors.New("invalid release filename")
	}
	dir := releaseVersionDir(req.ReleaseType, req.DeviceType, req.Version, req.OS, req.Arch)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", 0, err
	}
	dstPath := filepath.Join(dir, fileName)
	src, err := file.Open()
	if err != nil {
		return "", "", "", 0, err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", "", "", 0, err
	}
	hash := sha256.New()
	w := io.MultiWriter(dst, hash)
	size, copyErr := io.Copy(w, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return "", "", "", 0, copyErr
	}
	if closeErr != nil {
		return "", "", "", 0, closeErr
	}
	return fileName, dstPath, hex.EncodeToString(hash.Sum(nil)), size, nil
}

func releaseDir() string {
	base := "./data/oss"
	if global.NAV_VIPER != nil {
		if value := strings.TrimSpace(global.NAV_VIPER.GetString("local.oss-path")); value != "" {
			base = value
		}
	}
	return filepath.Join(base, "releases")
}

func releaseVersionDir(releaseType, deviceType, version, osName, arch string) string {
	parts := []string{
		releaseDir(),
		safeReleaseFileName(releaseType),
		safeReleaseFileName(deviceType),
		safeReleaseFileName(version),
	}
	platform := strings.Trim(safeReleaseFileName(osName+"-"+arch), "-")
	if platform != "" {
		parts = append(parts, platform)
	}
	return filepath.Join(parts...)
}

var safeReleaseNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeReleaseFileName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = safeReleaseNameRe.ReplaceAllString(value, "-")
	return strings.Trim(value, ".-")
}

func parseReleasePlatform(fileName string) (string, string) {
	fileName = strings.ToLower(fileName)
	osName := matchReleasePlatformAlias(fileName, map[string]string{
		"linux":   "linux",
		"darwin":  "darwin",
		"macos":   "darwin",
		"osx":     "darwin",
		"windows": "windows",
		"win32":   "windows",
		"win64":   "windows",
	})
	arch := matchReleasePlatformAlias(fileName, map[string]string{
		"amd64":   "amd64",
		"x86_64":  "amd64",
		"x64":     "amd64",
		"arm64":   "arm64",
		"aarch64": "arm64",
		"armv8":   "arm64",
	})
	return osName, arch
}

func parseReleaseContentPlatform(file *multipart.FileHeader) (string, string) {
	if file == nil {
		return "", ""
	}
	src, err := file.Open()
	if err != nil {
		return "", ""
	}
	defer src.Close()
	buffer := make([]byte, 4096)
	n, err := src.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", ""
	}
	return parseReleaseBinaryPlatform(buffer[:n])
}

func parseReleaseBinaryPlatform(data []byte) (string, string) {
	if len(data) >= 20 && data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		var byteOrder binary.ByteOrder = binary.LittleEndian
		if data[5] == 2 {
			byteOrder = binary.BigEndian
		}
		return "linux", archFromELFMachine(byteOrder.Uint16(data[18:20]))
	}
	if len(data) >= 64 && data[0] == 'M' && data[1] == 'Z' {
		peOffset := int(binary.LittleEndian.Uint32(data[0x3c:0x40]))
		if peOffset+6 <= len(data) && string(data[peOffset:peOffset+4]) == "PE\x00\x00" {
			return "windows", archFromPEMachine(binary.LittleEndian.Uint16(data[peOffset+4 : peOffset+6]))
		}
	}
	if len(data) >= 8 {
		if binary.BigEndian.Uint32(data[0:4]) == 0xcafebabe {
			return parseFatMachOPlatform(data, binary.BigEndian)
		}
		if binary.LittleEndian.Uint32(data[0:4]) == 0xcafebabe {
			return parseFatMachOPlatform(data, binary.LittleEndian)
		}
		if isMachOMagic(binary.BigEndian.Uint32(data[0:4])) {
			return "darwin", archFromMachOCPU(binary.BigEndian.Uint32(data[4:8]))
		}
		if isMachOMagic(binary.LittleEndian.Uint32(data[0:4])) {
			return "darwin", archFromMachOCPU(binary.LittleEndian.Uint32(data[4:8]))
		}
	}
	return "", ""
}

func parseFatMachOPlatform(data []byte, byteOrder binary.ByteOrder) (string, string) {
	count := int(byteOrder.Uint32(data[4:8]))
	if count > 8 {
		count = 8
	}
	archs := map[string]struct{}{}
	for i := 0; i < count; i++ {
		offset := 8 + i*20
		if offset+4 > len(data) {
			break
		}
		if arch := archFromMachOCPU(byteOrder.Uint32(data[offset : offset+4])); arch != "" {
			archs[arch] = struct{}{}
		}
	}
	if len(archs) > 1 {
		return "darwin", "all"
	}
	for arch := range archs {
		return "darwin", arch
	}
	return "darwin", ""
}

func isMachOMagic(value uint32) bool {
	switch value {
	case 0xfeedface, 0xfeedfacf:
		return true
	default:
		return false
	}
}

func archFromELFMachine(machine uint16) string {
	switch machine {
	case 62:
		return "amd64"
	case 183:
		return "arm64"
	default:
		return ""
	}
}

func archFromPEMachine(machine uint16) string {
	switch machine {
	case 0x8664:
		return "amd64"
	case 0xaa64:
		return "arm64"
	default:
		return ""
	}
}

func archFromMachOCPU(cpu uint32) string {
	switch cpu {
	case 0x01000007:
		return "amd64"
	case 0x0100000c:
		return "arm64"
	default:
		return ""
	}
}

func matchReleasePlatformAlias(fileName string, aliases map[string]string) string {
	for _, key := range sortedReleasePlatformAliasKeys(aliases) {
		matched, _ := regexp.MatchString(`(^|[^a-z0-9])`+regexp.QuoteMeta(key)+`([^a-z0-9]|$)`, fileName)
		if matched {
			return aliases[key]
		}
	}
	return ""
}

func sortedReleasePlatformAliasKeys(aliases map[string]string) []string {
	keys := make([]string, 0, len(aliases))
	for key := range aliases {
		keys = append(keys, key)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if len(keys[j]) > len(keys[i]) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func normalizeReleaseType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "client", "navmesh", "navmesh-client", "navmesh_client":
		return domains.ReleaseTypeNavmesh
	case "rain", "device", "software", "device-software", "device_software":
		return domains.ReleaseTypeRain
	case "hipnames", "standalone":
		return domains.ReleaseTypeHipnames
	case "dic", "visual_displacement", "visual-displacement":
		return domains.ReleaseTypeDIC
	default:
		return value
	}
}

func normalizeDeviceType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "all"
	}
	return value
}
