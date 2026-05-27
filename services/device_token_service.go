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

type DeviceTokenService struct {
	commonServices.CrudService[domains.DeviceToken]
}

func (s DeviceTokenService) WithDB(db *gorm.DB) DeviceTokenService {
	s.CrudService = *s.CrudService.WithDB(db)
	return s
}

type CreateDeviceTokenRequest struct {
	Name       string `json:"name"`
	ExpireTime int64  `json:"expireTime"`
}

type DeviceTokenResult struct {
	Token string              `json:"token"`
	Item  domains.DeviceToken `json:"item"`
}

func (s DeviceTokenService) ListByDevice(deviceGuid string) ([]domains.DeviceToken, error) {
	var tokens []domains.DeviceToken
	err := s.DB().
		Where("device_guid = ?", strings.TrimSpace(deviceGuid)).
		Order("status DESC, update_time DESC, create_time DESC").
		Limit(1).
		Find(&tokens).Error
	return tokens, err
}

func (s DeviceTokenService) Disable(deviceGuid, tokenGuid string) error {
	return s.SetStatus(deviceGuid, tokenGuid, domains.DeviceTokenStatusDisabled)
}

func (s DeviceTokenService) Enable(deviceGuid, tokenGuid string) error {
	return s.SetStatus(deviceGuid, tokenGuid, domains.DeviceTokenStatusEnabled)
}

func (s DeviceTokenService) SetStatus(deviceGuid, tokenGuid string, status int) error {
	deviceGuid = strings.TrimSpace(deviceGuid)
	tokenGuid = strings.TrimSpace(tokenGuid)
	if deviceGuid == "" || tokenGuid == "" {
		return errors.New("deviceGuid and tokenGuid required")
	}
	if status != domains.DeviceTokenStatusDisabled && status != domains.DeviceTokenStatusEnabled {
		return errors.New("unsupported token status")
	}
	return s.DB().Model(&domains.DeviceToken{}).Where("device_guid = ? AND guid = ?", deviceGuid, tokenGuid).Updates(map[string]any{
		"status":      status,
		"update_time": domains.NowMilli(),
	}).Error
}

func (s DeviceTokenService) CreateToken(deviceGuid string, req CreateDeviceTokenRequest) (*DeviceTokenResult, error) {
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
	var row domains.DeviceToken
	err = s.DB().Where("device_guid = ?", deviceGuid).Order("create_time ASC").First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.DeviceToken{
			BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now},
			DeviceGuid:     deviceGuid,
		}
	}
	row.Token = token
	row.TokenHash = utils.HashToken(token)
	row.Name = name
	row.Status = domains.DeviceTokenStatusEnabled
	row.ExpireTime = req.ExpireTime
	row.UpdateTime = now
	if err := s.DB().Save(&row).Error; err != nil {
		return nil, err
	}
	return &DeviceTokenResult{Token: token, Item: row}, nil
}

func (s DeviceTokenService) EnabledTokenResult(deviceGuid string) (*DeviceTokenResult, error) {
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return nil, errors.New("deviceGuid required")
	}
	var row domains.DeviceToken
	err := s.DB().
		Where("device_guid = ? AND status = ?", deviceGuid, domains.DeviceTokenStatusEnabled).
		Where("expire_time = 0 OR expire_time > ?", domains.NowMilli()).
		Order("update_time DESC, create_time DESC").
		First(&row).Error
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(row.Token) == "" {
		return nil, errors.New("device token unavailable")
	}
	return &DeviceTokenResult{Token: row.Token, Item: row}, nil
}

func (s DeviceTokenService) Rotate(deviceGuid, tokenGuid string) (*DeviceTokenResult, error) {
	var old domains.DeviceToken
	_ = s.DB().Where("device_guid = ? AND guid = ?", strings.TrimSpace(deviceGuid), strings.TrimSpace(tokenGuid)).First(&old).Error
	return s.CreateToken(deviceGuid, CreateDeviceTokenRequest{Name: old.Name, ExpireTime: old.ExpireTime})
}

func (s DeviceTokenService) Validate(deviceGuid, token string) error {
	var count int64
	err := s.DB().Model(&domains.DeviceToken{}).
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

func (s DeviceTokenService) HasEnabled(deviceGuid string) bool {
	var count int64
	_ = s.DB().Model(&domains.DeviceToken{}).
		Where("device_guid = ? AND status = ?", strings.TrimSpace(deviceGuid), domains.DeviceTokenStatusEnabled).
		Where("expire_time = 0 OR expire_time > ?", domains.NowMilli()).
		Count(&count).Error
	return count > 0
}

func (s DeviceTokenService) HasTokenHash(deviceGuid, token string) bool {
	var count int64
	_ = s.DB().Model(&domains.DeviceToken{}).
		Where("device_guid = ? AND token_hash = ?", strings.TrimSpace(deviceGuid), utils.HashToken(token)).
		Count(&count).Error
	return count > 0
}

func (s DeviceTokenService) DisableTokenHash(deviceGuid, token string) error {
	return s.DB().Model(&domains.DeviceToken{}).
		Where("device_guid = ? AND token_hash = ?", strings.TrimSpace(deviceGuid), utils.HashToken(token)).
		Updates(map[string]any{"status": domains.DeviceTokenStatusDisabled, "update_time": domains.NowMilli()}).Error
}
