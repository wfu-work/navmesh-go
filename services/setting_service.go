package services

import (
	"navmesh-go/domains"
	"strings"

	"github.com/wfu-work/nav-common-go-lib/global"
)

type SettingService struct{}

func (s SettingService) List() ([]domains.Setting, error) {
	var items []domains.Setting
	err := global.NAV_DB.Order("key ASC").Find(&items).Error
	return items, err
}

func (s SettingService) Save(key, value string) (*domains.Setting, error) {
	now := domains.NowMilli()
	row := domains.Setting{Key: key, Value: value, CreateTime: now, UpdateTime: now}
	err := global.NAV_DB.Save(&row).Error
	return &row, err
}

func (s SettingService) Value(key, def string) string {
	var row domains.Setting
	if err := global.NAV_DB.Where("key = ?", strings.TrimSpace(key)).First(&row).Error; err != nil {
		return def
	}
	if strings.TrimSpace(row.Value) == "" {
		return def
	}
	return strings.TrimSpace(row.Value)
}
