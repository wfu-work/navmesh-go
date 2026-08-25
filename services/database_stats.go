package services

import (
	"errors"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/gorm"
)

type DatabasePoolStats struct {
	MaxOpenConnections int   `json:"maxOpenConnections"`
	OpenConnections    int   `json:"openConnections"`
	InUse              int   `json:"inUse"`
	Idle               int   `json:"idle"`
	WaitCount          int64 `json:"waitCount"`
	WaitDurationMs     int64 `json:"waitDurationMs"`
	MaxIdleClosed      int64 `json:"maxIdleClosed"`
	MaxIdleTimeClosed  int64 `json:"maxIdleTimeClosed"`
	MaxLifetimeClosed  int64 `json:"maxLifetimeClosed"`
}

type SQLiteRuntimeStats struct {
	JournalMode       string `json:"journalMode"`
	Synchronous       int    `json:"synchronous"`
	ForeignKeys       bool   `json:"foreignKeys"`
	BusyTimeoutMs     int    `json:"busyTimeoutMs"`
	PageSizeBytes     int64  `json:"pageSizeBytes"`
	PageCount         int64  `json:"pageCount"`
	FreePageCount     int64  `json:"freePageCount"`
	DatabaseSizeBytes int64  `json:"databaseSizeBytes"`
	UsedSizeBytes     int64  `json:"usedSizeBytes"`
	CacheSize         int64  `json:"cacheSize"`
}

type DatabaseRuntimeStats struct {
	Dialect     string              `json:"dialect"`
	CollectedAt int64               `json:"collectedAt"`
	Pool        DatabasePoolStats   `json:"pool"`
	Queries     DatabaseQueryStats  `json:"queries"`
	SQLite      *SQLiteRuntimeStats `json:"sqlite,omitempty"`
}

func CollectDatabaseRuntimeStats(db *gorm.DB) (*DatabaseRuntimeStats, error) {
	if db == nil {
		return nil, errors.New("database not initialized")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	pool := sqlDB.Stats()
	result := &DatabaseRuntimeStats{
		Dialect:     db.Dialector.Name(),
		CollectedAt: domains.NowMilli(),
		Queries:     DefaultDatabaseQueryMetrics.Snapshot(),
		Pool: DatabasePoolStats{
			MaxOpenConnections: pool.MaxOpenConnections,
			OpenConnections:    pool.OpenConnections,
			InUse:              pool.InUse,
			Idle:               pool.Idle,
			WaitCount:          pool.WaitCount,
			WaitDurationMs:     pool.WaitDuration.Milliseconds(),
			MaxIdleClosed:      pool.MaxIdleClosed,
			MaxIdleTimeClosed:  pool.MaxIdleTimeClosed,
			MaxLifetimeClosed:  pool.MaxLifetimeClosed,
		},
	}
	if result.Dialect != "sqlite" {
		return result, nil
	}
	sqliteStats, err := collectSQLiteRuntimeStats(db)
	if err != nil {
		return nil, err
	}
	result.SQLite = sqliteStats
	return result, nil
}

func collectSQLiteRuntimeStats(db *gorm.DB) (*SQLiteRuntimeStats, error) {
	stats := &SQLiteRuntimeStats{}
	var foreignKeys int
	queries := []struct {
		statement string
		target    any
	}{
		{statement: "PRAGMA journal_mode", target: &stats.JournalMode},
		{statement: "PRAGMA synchronous", target: &stats.Synchronous},
		{statement: "PRAGMA foreign_keys", target: &foreignKeys},
		{statement: "PRAGMA busy_timeout", target: &stats.BusyTimeoutMs},
		{statement: "PRAGMA page_size", target: &stats.PageSizeBytes},
		{statement: "PRAGMA page_count", target: &stats.PageCount},
		{statement: "PRAGMA freelist_count", target: &stats.FreePageCount},
		{statement: "PRAGMA cache_size", target: &stats.CacheSize},
	}
	for _, query := range queries {
		if err := db.Raw(query.statement).Scan(query.target).Error; err != nil {
			return nil, err
		}
	}
	stats.ForeignKeys = foreignKeys != 0
	stats.DatabaseSizeBytes = stats.PageSizeBytes * stats.PageCount
	usedPages := stats.PageCount - stats.FreePageCount
	if usedPages < 0 {
		usedPages = 0
	}
	stats.UsedSizeBytes = stats.PageSizeBytes * usedPages
	return stats, nil
}

func (s MaintenanceService) DatabaseStats() (*DatabaseRuntimeStats, error) {
	return CollectDatabaseRuntimeStats(global.NAV_DB)
}
