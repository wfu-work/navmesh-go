package services

import (
	"strings"
	"sync"
	"time"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/gorm"
)

type SettingService struct {
	db *gorm.DB
}

func (s SettingService) WithDB(db *gorm.DB) SettingService {
	s.db = db
	return s
}

func (s SettingService) DB() *gorm.DB {
	if s.db != nil {
		return s.db
	}
	return global.NAV_DB
}

const (
	settingValueCacheTTL = time.Minute
	settingValueCacheMax = 128
)

type settingValueCacheKey struct {
	db  *gorm.DB
	key string
}

type settingValueCacheEntry struct {
	value     string
	expiresAt time.Time
}

var settingValueCache struct {
	sync.RWMutex
	items map[settingValueCacheKey]settingValueCacheEntry
}

func (s SettingService) List() ([]domains.Setting, error) {
	var items []domains.Setting
	db := s.DB()
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	err := db.Order("key ASC").Find(&items).Error
	return items, err
}

func (s SettingService) Save(key, value string) (*domains.Setting, error) {
	key = strings.TrimSpace(key)
	db := s.DB()
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	now := domains.NowMilli()
	row := domains.Setting{Key: key, Value: value, CreateTime: now, UpdateTime: now}
	err := db.Save(&row).Error
	if err == nil {
		invalidateSettingValueCache(db, key)
	}
	if err == nil && isTCPMappingSetting(key) {
		triggerTCPMappingReload()
	}
	if err == nil && key == "telemetry_sample_interval" {
		invalidateTelemetrySampleInterval()
	}
	return &row, err
}

func (s SettingService) Value(key, def string) string {
	key = strings.TrimSpace(key)
	db := s.DB()
	if db == nil || key == "" {
		return def
	}
	cacheKey := settingValueCacheKey{db: db, key: key}
	now := time.Now()
	settingValueCache.RLock()
	entry, ok := settingValueCache.items[cacheKey]
	settingValueCache.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		if entry.value == "" {
			return def
		}
		return entry.value
	}

	var row domains.Setting
	if err := db.Where("key = ?", key).First(&row).Error; err != nil {
		return def
	}
	value := strings.TrimSpace(row.Value)
	settingValueCache.Lock()
	if settingValueCache.items == nil {
		settingValueCache.items = make(map[settingValueCacheKey]settingValueCacheEntry)
	}
	for cachedKey, cachedEntry := range settingValueCache.items {
		if !now.Before(cachedEntry.expiresAt) {
			delete(settingValueCache.items, cachedKey)
		}
	}
	for len(settingValueCache.items) >= settingValueCacheMax {
		for cachedKey := range settingValueCache.items {
			delete(settingValueCache.items, cachedKey)
			break
		}
	}
	settingValueCache.items[cacheKey] = settingValueCacheEntry{value: value, expiresAt: now.Add(settingValueCacheTTL)}
	settingValueCache.Unlock()
	if value == "" {
		return def
	}
	return value
}

func invalidateSettingValueCache(db *gorm.DB, key string) {
	key = strings.TrimSpace(key)
	settingValueCache.Lock()
	delete(settingValueCache.items, settingValueCacheKey{db: db, key: key})
	settingValueCache.Unlock()
}

func isTCPMappingSetting(key string) bool {
	switch key {
	case "tcp_mapping_enabled", "tcp_public_port_min", "tcp_public_port_max", "tcp_gateway_domain":
		return true
	default:
		return false
	}
}
