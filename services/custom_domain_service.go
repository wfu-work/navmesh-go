package services

import (
	"errors"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

type CustomDomainService struct {
	commonServices.CrudService[domains.CustomDomain]
}

func (s CustomDomainService) WithDB(db *gorm.DB) CustomDomainService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type SaveCustomDomainRequest struct {
	Domain      string `json:"domain"`
	MappingGuid string `json:"mappingGuid"`
}

func (s CustomDomainService) List(params map[string]string) ([]domains.CustomDomain, int64, error) {
	db := s.DB().Model(&domains.CustomDomain{})
	if domain := strings.TrimSpace(params["domain"]); domain != "" {
		db = db.Where("domain LIKE ?", "%"+normalizeHost(domain)+"%")
	}
	if mappingGuid := strings.TrimSpace(params["mappingGuid"]); mappingGuid != "" {
		db = db.Where("mapping_guid = ?", mappingGuid)
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
	var items []domains.CustomDomain
	err := db.Order("update_time DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (s CustomDomainService) Save(req SaveCustomDomainRequest) (*domains.CustomDomain, error) {
	domain := normalizeHost(req.Domain)
	mappingGuid := strings.TrimSpace(req.MappingGuid)
	if domain == "" {
		return nil, errors.New("domain required")
	}
	if mappingGuid == "" {
		return nil, errors.New("mappingGuid required")
	}
	mappingService := ServiceGroupApp.MappingService.WithDB(s.DB())
	mapping, err := mappingService.GetByGuid(mappingGuid)
	if err != nil {
		return nil, err
	}
	if mapping == nil {
		return nil, errors.New("mapping not found")
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	return s.save(domain, mappingGuid, token, false)
}

func (s CustomDomainService) EnsureForMapping(mapping domains.PortMapping) (*domains.CustomDomain, error) {
	domain := normalizeHost(mapping.PublicHost)
	if domain == "" {
		return nil, errors.New("domain required")
	}
	if strings.TrimSpace(mapping.Guid) == "" {
		return nil, errors.New("mappingGuid required")
	}
	var row domains.CustomDomain
	err := s.DB().Where("domain = ?", domain).First(&row).Error
	if err == nil {
		return &row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	return s.save(domain, mapping.Guid, token, false)
}

func (s CustomDomainService) Verify(domain, token string) error {
	domain = normalizeHost(domain)
	token = strings.TrimSpace(token)
	if domain == "" || token == "" {
		return errors.New("domain and token required")
	}
	return s.DB().Model(&domains.CustomDomain{}).
		Where("domain = ? AND verify_token = ? AND status = ?", domain, token, int(domains.StatusEnabled)).
		Updates(map[string]any{"verified": true, "update_time": domains.NowMilli()}).Error
}

func (s CustomDomainService) Disable(domain string) error {
	domain = normalizeHost(domain)
	if domain == "" {
		return errors.New("domain required")
	}
	return s.DB().Model(&domains.CustomDomain{}).Where("domain = ?", domain).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func (s CustomDomainService) save(domain, mappingGuid, token string, verified bool) (*domains.CustomDomain, error) {
	now := domains.NowMilli()
	var row domains.CustomDomain
	err := s.DB().Where("domain = ?", domain).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.CustomDomain{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}, Domain: domain}
	}
	row.MappingGuid = mappingGuid
	row.VerifyToken = token
	row.Verified = verified
	row.Status = int(domains.StatusEnabled)
	row.UpdateTime = now
	if err := s.DB().Save(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
