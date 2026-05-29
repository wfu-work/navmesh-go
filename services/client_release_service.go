package services

import (
	"crypto/sha256"
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

type ClientReleaseService struct {
	commonServices.CrudService[domains.ClientRelease]
}

func (s ClientReleaseService) WithDB(db *gorm.DB) ClientReleaseService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type UploadClientReleaseRequest struct {
	Version     string
	OS          string
	Arch        string
	DownloadURL string
}

func (s ClientReleaseService) Upload(file *multipart.FileHeader, req UploadClientReleaseRequest) (*domains.ClientRelease, error) {
	if file == nil {
		return nil, errors.New("file required")
	}
	fileName := safeClientReleaseFileName(file.Filename)
	if fileName == "" || !strings.HasPrefix(fileName, "navmesh-client-") {
		return nil, errors.New("invalid client binary filename")
	}
	req.Version = strings.TrimSpace(req.Version)
	req.OS = strings.TrimSpace(req.OS)
	req.Arch = strings.TrimSpace(req.Arch)
	req.DownloadURL = strings.TrimSpace(req.DownloadURL)
	if req.OS == "" || req.Arch == "" {
		req.OS, req.Arch = parseClientReleasePlatform(fileName)
	}
	if req.OS == "" || req.Arch == "" {
		return nil, errors.New("os and arch required")
	}
	dir := clientReleaseDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dstPath := filepath.Join(dir, fileName)
	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	w := io.MultiWriter(dst, hash)
	size, copyErr := io.Copy(w, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	now := domains.NowMilli()
	row := domains.ClientRelease{
		BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now, UpdateTime: now},
		Version:        req.Version,
		OS:             req.OS,
		Arch:           req.Arch,
		FileName:       fileName,
		FilePath:       dstPath,
		Sha256:         hex.EncodeToString(hash.Sum(nil)),
		Size:           size,
		DownloadURL:    req.DownloadURL,
		Status:         domains.ClientReleaseStatusEnabled,
	}
	var existing domains.ClientRelease
	if err := s.DB().Where("file_name = ?", fileName).First(&existing).Error; err == nil {
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

func (s ClientReleaseService) List(params map[string]string) ([]domains.ClientRelease, int64, error) {
	db := s.DB().Model(&domains.ClientRelease{})
	if osName := strings.TrimSpace(params["os"]); osName != "" {
		db = db.Where("os = ?", osName)
	}
	if arch := strings.TrimSpace(params["arch"]); arch != "" {
		db = db.Where("arch = ?", arch)
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
	var items []domains.ClientRelease
	err := db.Order("update_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (s ClientReleaseService) GetEnabled(guid string) (*domains.ClientRelease, error) {
	var row domains.ClientRelease
	if err := s.DB().Where("guid = ? AND status = ?", strings.TrimSpace(guid), domains.ClientReleaseStatusEnabled).First(&row).Error; err != nil {
		return nil, errors.New("client release not found")
	}
	return &row, nil
}

func (s ClientReleaseService) FindDownload(fileName string) (*domains.ClientRelease, error) {
	var row domains.ClientRelease
	if err := s.DB().Where("file_name = ? AND status = ?", safeClientReleaseFileName(fileName), domains.ClientReleaseStatusEnabled).First(&row).Error; err != nil {
		return nil, errors.New("client release not found")
	}
	if _, err := os.Stat(row.FilePath); err != nil {
		return nil, errors.New("client release file missing")
	}
	return &row, nil
}

func clientReleaseDir() string {
	base := "./data/oss"
	if global.NAV_VIPER != nil {
		if value := strings.TrimSpace(global.NAV_VIPER.GetString("local.oss-path")); value != "" {
			base = value
		}
	}
	return filepath.Join(base, "navmesh-client")
}

var safeClientReleaseNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeClientReleaseFileName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = safeClientReleaseNameRe.ReplaceAllString(value, "-")
	return strings.Trim(value, ".-")
}

func parseClientReleasePlatform(fileName string) (string, string) {
	fileName = strings.TrimSuffix(fileName, ".exe")
	parts := strings.Split(fileName, "-")
	if len(parts) < 4 {
		return "", ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}
