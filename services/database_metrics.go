package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"navmesh-go/domains"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultDatabaseSlowThreshold = 200 * time.Millisecond
	maxRecentSlowQueries         = 32
	maxDatabaseQueryPatternBytes = 512
)

type databaseOperation uint8

const (
	databaseOperationCreate databaseOperation = iota
	databaseOperationQuery
	databaseOperationUpdate
	databaseOperationDelete
	databaseOperationRow
	databaseOperationRaw
	databaseOperationCount
)

var databaseOperationNames = [...]string{
	"create",
	"query",
	"update",
	"delete",
	"row",
	"raw",
}

type databaseOperationMetrics struct {
	count        atomic.Int64
	errors       atomic.Int64
	rowsAffected atomic.Int64
}

type DatabaseOperationStats struct {
	Count        int64 `json:"count"`
	Errors       int64 `json:"errors"`
	RowsAffected int64 `json:"rowsAffected"`
}

type SlowDatabaseQuery struct {
	Operation    string  `json:"operation"`
	Pattern      string  `json:"pattern,omitempty"`
	DurationMs   float64 `json:"durationMs"`
	RowsAffected int64   `json:"rowsAffected"`
	OccurredAt   int64   `json:"occurredAt"`
	Failed       bool    `json:"failed"`
}

type DatabaseQueryStats struct {
	SlowThresholdMs   int64                             `json:"slowThresholdMs"`
	Total             int64                             `json:"total"`
	Errors            int64                             `json:"errors"`
	Slow              int64                             `json:"slow"`
	TotalDurationMs   float64                           `json:"totalDurationMs"`
	AverageDurationMs float64                           `json:"averageDurationMs"`
	MaxDurationMs     float64                           `json:"maxDurationMs"`
	RowsAffected      int64                             `json:"rowsAffected"`
	Operations        map[string]DatabaseOperationStats `json:"operations"`
	RecentSlow        []SlowDatabaseQuery               `json:"recentSlow"`
}

// DatabaseQueryMetrics aggregates bounded, low-cardinality SQL metrics. It
// reuses GORM's existing trace start time, so normal queries don't allocate
// metric state. Query patterns are generated only for slow operations.
type DatabaseQueryMetrics struct {
	slowThresholdNanos atomic.Int64
	total              atomic.Int64
	errors             atomic.Int64
	slow               atomic.Int64
	totalNanos         atomic.Int64
	maxNanos           atomic.Int64
	rowsAffected       atomic.Int64
	operations         [databaseOperationCount]databaseOperationMetrics
	recentMu           sync.Mutex
	recentSlow         []SlowDatabaseQuery
}

func NewDatabaseQueryMetrics(slowThreshold time.Duration) *DatabaseQueryMetrics {
	metrics := &DatabaseQueryMetrics{}
	metrics.SetSlowThreshold(slowThreshold)
	return metrics
}

func (m *DatabaseQueryMetrics) SetSlowThreshold(threshold time.Duration) {
	if m == nil {
		return
	}
	if threshold <= 0 {
		threshold = defaultDatabaseSlowThreshold
	}
	m.slowThresholdNanos.Store(int64(threshold))
}

func (m *DatabaseQueryMetrics) Reset() {
	if m == nil {
		return
	}
	m.total.Store(0)
	m.errors.Store(0)
	m.slow.Store(0)
	m.totalNanos.Store(0)
	m.maxNanos.Store(0)
	m.rowsAffected.Store(0)
	for index := range m.operations {
		m.operations[index].count.Store(0)
		m.operations[index].errors.Store(0)
		m.operations[index].rowsAffected.Store(0)
	}
	m.recentMu.Lock()
	m.recentSlow = nil
	m.recentMu.Unlock()
}

func (m *DatabaseQueryMetrics) Snapshot() DatabaseQueryStats {
	if m == nil {
		return DatabaseQueryStats{Operations: map[string]DatabaseOperationStats{}, RecentSlow: []SlowDatabaseQuery{}}
	}
	total := m.total.Load()
	totalNanos := m.totalNanos.Load()
	result := DatabaseQueryStats{
		SlowThresholdMs: time.Duration(m.slowThresholdNanos.Load()).Milliseconds(),
		Total:           total,
		Errors:          m.errors.Load(),
		Slow:            m.slow.Load(),
		TotalDurationMs: float64(totalNanos) / float64(time.Millisecond),
		MaxDurationMs:   float64(m.maxNanos.Load()) / float64(time.Millisecond),
		RowsAffected:    m.rowsAffected.Load(),
		Operations:      make(map[string]DatabaseOperationStats, len(databaseOperationNames)),
	}
	if total > 0 {
		result.AverageDurationMs = float64(totalNanos) / float64(total) / float64(time.Millisecond)
	}
	for operation, name := range databaseOperationNames {
		metrics := &m.operations[operation]
		result.Operations[name] = DatabaseOperationStats{
			Count:        metrics.count.Load(),
			Errors:       metrics.errors.Load(),
			RowsAffected: metrics.rowsAffected.Load(),
		}
	}
	m.recentMu.Lock()
	result.RecentSlow = make([]SlowDatabaseQuery, len(m.recentSlow))
	for index := range m.recentSlow {
		result.RecentSlow[index] = m.recentSlow[len(m.recentSlow)-1-index]
	}
	m.recentMu.Unlock()
	return result
}

func (m *DatabaseQueryMetrics) recordOperation(operation databaseOperation, db *gorm.DB) {
	if m == nil || db == nil || operation >= databaseOperationCount {
		return
	}
	metrics := &m.operations[operation]
	metrics.count.Add(1)
	rowsAffected := db.RowsAffected
	if rowsAffected > 0 {
		metrics.rowsAffected.Add(rowsAffected)
		m.rowsAffected.Add(rowsAffected)
	}
	if db.Error != nil && !errors.Is(db.Error, gorm.ErrRecordNotFound) {
		metrics.errors.Add(1)
	}
}

func (m *DatabaseQueryMetrics) recordTrace(duration time.Duration, query func() (string, int64), queryErr error) {
	if m == nil {
		return
	}
	m.total.Add(1)
	m.totalNanos.Add(int64(duration))
	updateAtomicMax(&m.maxNanos, int64(duration))
	isError := queryErr != nil && !errors.Is(queryErr, gorm.ErrRecordNotFound)
	if isError {
		m.errors.Add(1)
	}
	threshold := time.Duration(m.slowThresholdNanos.Load())
	if threshold <= 0 || duration < threshold {
		return
	}
	m.slow.Add(1)
	sqlText, rowsAffected := query()
	pattern := sanitizeDatabaseQueryPattern(sqlText)
	entry := SlowDatabaseQuery{
		Operation:    classifyDatabaseOperation(pattern),
		Pattern:      pattern,
		DurationMs:   float64(duration) / float64(time.Millisecond),
		RowsAffected: max(rowsAffected, 0),
		OccurredAt:   domains.NowMilli(),
		Failed:       isError,
	}
	m.recentMu.Lock()
	if len(m.recentSlow) == maxRecentSlowQueries {
		copy(m.recentSlow, m.recentSlow[1:])
		m.recentSlow[len(m.recentSlow)-1] = entry
	} else {
		m.recentSlow = append(m.recentSlow, entry)
	}
	m.recentMu.Unlock()
}

func updateAtomicMax(target *atomic.Int64, value int64) {
	for current := target.Load(); value > current; current = target.Load() {
		if target.CompareAndSwap(current, value) {
			return
		}
	}
}

type databaseMetricsLogger struct {
	logger.Interface
	metrics *DatabaseQueryMetrics
}

func (l databaseMetricsLogger) LogMode(level logger.LogLevel) logger.Interface {
	return databaseMetricsLogger{Interface: l.Interface.LogMode(level), metrics: l.metrics}
}

func (l databaseMetricsLogger) Trace(ctx context.Context, begin time.Time, query func() (string, int64), err error) {
	duration := time.Since(begin)
	l.Interface.Trace(ctx, begin, query, err)
	l.metrics.recordTrace(duration, query, err)
}

type databaseMetricsPlugin struct {
	metrics *DatabaseQueryMetrics
}

func (p databaseMetricsPlugin) Name() string { return "navmesh:database_metrics" }

func (p databaseMetricsPlugin) Initialize(db *gorm.DB) error {
	if p.metrics == nil {
		return errors.New("database query metrics are nil")
	}
	underlying := db.Config.Logger
	if underlying == nil {
		underlying = logger.Discard
	}
	db.Config.Logger = databaseMetricsLogger{Interface: underlying, metrics: p.metrics}
	registrations := []error{
		db.Callback().Create().After("*").Register("navmesh:metrics_create", func(db *gorm.DB) { p.metrics.recordOperation(databaseOperationCreate, db) }),
		db.Callback().Query().After("*").Register("navmesh:metrics_query", func(db *gorm.DB) { p.metrics.recordOperation(databaseOperationQuery, db) }),
		db.Callback().Update().After("*").Register("navmesh:metrics_update", func(db *gorm.DB) { p.metrics.recordOperation(databaseOperationUpdate, db) }),
		db.Callback().Delete().After("*").Register("navmesh:metrics_delete", func(db *gorm.DB) { p.metrics.recordOperation(databaseOperationDelete, db) }),
		db.Callback().Row().After("*").Register("navmesh:metrics_row", func(db *gorm.DB) { p.metrics.recordOperation(databaseOperationRow, db) }),
		db.Callback().Raw().After("*").Register("navmesh:metrics_raw", func(db *gorm.DB) { p.metrics.recordOperation(databaseOperationRaw, db) }),
	}
	return errors.Join(registrations...)
}

func RegisterDatabaseQueryMetrics(db *gorm.DB, metrics *DatabaseQueryMetrics) error {
	if db == nil {
		return errors.New("database not initialized")
	}
	err := db.Use(databaseMetricsPlugin{metrics: metrics})
	if errors.Is(err, gorm.ErrRegistered) {
		return nil
	}
	return err
}

func classifyDatabaseOperation(pattern string) string {
	fields := strings.Fields(pattern)
	if len(fields) == 0 {
		return "unknown"
	}
	switch strings.ToUpper(fields[0]) {
	case "INSERT", "REPLACE":
		return "create"
	case "SELECT", "WITH":
		return "query"
	case "UPDATE":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "raw"
	}
}

func sanitizeDatabaseQueryPattern(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	var result strings.Builder
	result.Grow(min(len(query), maxDatabaseQueryPatternBytes))
	spacePending := false
	for index := 0; index < len(query); index++ {
		current := query[index]
		if current == '\'' || current == '"' {
			if spacePending && result.Len() > 0 {
				result.WriteByte(' ')
			}
			spacePending = false
			quote := current
			result.WriteByte('?')
			for index++; index < len(query); index++ {
				if query[index] != quote {
					continue
				}
				if index+1 < len(query) && query[index+1] == quote {
					index++
					continue
				}
				break
			}
			continue
		}
		if current >= '0' && current <= '9' {
			if spacePending && result.Len() > 0 {
				result.WriteByte(' ')
			}
			spacePending = false
			result.WriteByte('?')
			for index+1 < len(query) && isDatabaseNumericByte(query[index+1]) {
				index++
			}
			continue
		}
		if current == ' ' || current == '\t' || current == '\r' || current == '\n' {
			spacePending = result.Len() > 0
			continue
		}
		if spacePending {
			result.WriteByte(' ')
			spacePending = false
		}
		result.WriteByte(current)
		if result.Len() >= maxDatabaseQueryPatternBytes {
			break
		}
	}
	return truncateDatabaseMetricText(strings.TrimSpace(result.String()), maxDatabaseQueryPatternBytes)
}

func isDatabaseNumericByte(value byte) bool {
	return (value >= '0' && value <= '9') || value == '.' || value == 'e' || value == 'E' || value == '+' || value == '-'
}

func truncateDatabaseMetricText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

var DefaultDatabaseQueryMetrics = NewDatabaseQueryMetrics(defaultDatabaseSlowThreshold)
