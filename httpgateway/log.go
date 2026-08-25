package httpgateway

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type mappingLogInput struct {
	MappingGuid  string
	DeviceGuid   string
	Host         string
	Method       string
	Path         string
	SourceIP     string
	StatusCode   int
	DurationMs   int64
	TunnelOpenMs int64
	UpstreamMs   int64
	FirstByteMs  int64
	ReusedConn   bool
	BytesIn      int64
	BytesOut     int64
	ErrorMessage string
	CreateTime   int64
}

const (
	defaultAccessLogQueueSize     = 8192
	defaultAccessLogBatchSize     = 200
	maximumAccessLogBatchSize     = 1000
	defaultAccessLogFlushInterval = 100 * time.Millisecond
	accessLogWriteMaxAttempts     = 3
)

var accessLogWriteRetryDelays = [...]time.Duration{5 * time.Millisecond, 20 * time.Millisecond}

type AccessLogWriter struct {
	db            *gorm.DB
	queue         chan mappingLogInput
	batchSize     int
	flushInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	enqueueMu     sync.RWMutex
	stopping      bool
	enqueued      atomic.Int64
	dropped       atomic.Int64
	queuePeak     atomic.Int64
	written       atomic.Int64
	batches       atomic.Int64
	retries       atomic.Int64
	writeErrors   atomic.Int64
	lastBatchUs   atomic.Int64
	maxBatchUs    atomic.Int64
}

func NewAccessLogWriter(db *gorm.DB, queueSize, batchSize int, flushInterval time.Duration) *AccessLogWriter {
	if queueSize <= 0 {
		queueSize = defaultAccessLogQueueSize
	}
	if batchSize <= 0 {
		batchSize = defaultAccessLogBatchSize
	}
	if batchSize > maximumAccessLogBatchSize {
		batchSize = maximumAccessLogBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = defaultAccessLogFlushInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &AccessLogWriter{
		db:            db,
		queue:         make(chan mappingLogInput, queueSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (w *AccessLogWriter) Start() {
	if w == nil {
		return
	}
	w.wg.Add(1)
	go w.run()
}

func (w *AccessLogWriter) Enqueue(input mappingLogInput) bool {
	if w == nil {
		return false
	}
	input = normalizeAccessLogInput(input)
	w.enqueueMu.RLock()
	defer w.enqueueMu.RUnlock()
	if w.stopping {
		return false
	}
	select {
	case w.queue <- input:
		w.enqueued.Add(1)
		updateAtomicMax(&w.queuePeak, int64(len(w.queue)))
		return true
	default:
		w.dropped.Add(1)
		return false
	}
}

func (w *AccessLogWriter) Stop(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.enqueueMu.Lock()
	w.stopping = true
	w.cancel()
	w.enqueueMu.Unlock()
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *AccessLogWriter) run() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()
	dropTicker := time.NewTicker(time.Minute)
	defer dropTicker.Stop()
	lastReportedDrops := int64(0)
	reportDrops := func() {
		total := w.dropped.Load()
		if total <= lastReportedDrops || global.NAV_LOG == nil {
			return
		}
		global.NAV_LOG.Warn(
			"http access logs dropped",
			zap.Int64("sinceLastReport", total-lastReportedDrops),
			zap.Int64("total", total),
			zap.Int("queueLength", len(w.queue)),
			zap.Int("queueCapacity", cap(w.queue)),
		)
		lastReportedDrops = total
	}
	batch := make([]domains.HttpAccessLog, 0, w.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		rows := batch
		rowCount := len(rows)
		db := w.db
		if db == nil {
			db = global.NAV_DB
		}
		started := time.Now()
		retries, err := writeAccessLogBatch(db, rows)
		elapsedUs := time.Since(started).Microseconds()
		w.lastBatchUs.Store(elapsedUs)
		updateAtomicMax(&w.maxBatchUs, elapsedUs)
		w.retries.Add(int64(retries))
		if err != nil {
			w.dropped.Add(int64(rowCount))
			w.writeErrors.Add(1)
			if global.NAV_LOG != nil {
				global.NAV_LOG.Warn("write http access log batch failed", zap.Int("rows", rowCount), zap.Int("retries", retries), zap.Error(err))
			}
		} else {
			w.written.Add(int64(rowCount))
			w.batches.Add(1)
		}
		clear(rows)
		batch = rows[:0]
	}
	for {
		select {
		case input := <-w.queue:
			batch = append(batch, accessLogRow(input))
			if len(batch) >= w.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-dropTicker.C:
			reportDrops()
		case <-w.ctx.Done():
			for {
				select {
				case input := <-w.queue:
					batch = append(batch, accessLogRow(input))
					if len(batch) >= w.batchSize {
						flush()
					}
				default:
					flush()
					reportDrops()
					return
				}
			}
		}
	}
}

func accessLogRow(input mappingLogInput) domains.HttpAccessLog {
	createTime := input.CreateTime
	if createTime <= 0 {
		createTime = domains.NowMilli()
	}
	return domains.HttpAccessLog{
		MappingGuid:  input.MappingGuid,
		DeviceGuid:   input.DeviceGuid,
		Host:         input.Host,
		Method:       input.Method,
		Path:         input.Path,
		SourceIP:     input.SourceIP,
		StatusCode:   input.StatusCode,
		DurationMs:   input.DurationMs,
		TunnelOpenMs: input.TunnelOpenMs,
		UpstreamMs:   input.UpstreamMs,
		FirstByteMs:  input.FirstByteMs,
		ReusedConn:   input.ReusedConn,
		BytesIn:      input.BytesIn,
		BytesOut:     input.BytesOut,
		ErrorMessage: input.ErrorMessage,
		CreateTime:   createTime,
	}
}

func writeAccessLogBatch(db *gorm.DB, rows []domains.HttpAccessLog) (int, error) {
	if db == nil {
		return 0, errors.New("http access log database unavailable")
	}
	if len(rows) == 0 {
		return 0, nil
	}
	var err error
	for attempt := 0; attempt < accessLogWriteMaxAttempts; attempt++ {
		// A single multi-row INSERT is atomic in SQLite without GORM's outer
		// transaction, saving an extra BEGIN/COMMIT and shortening lock time.
		err = db.Session(&gorm.Session{SkipDefaultTransaction: true}).CreateInBatches(&rows, len(rows)).Error
		if err == nil {
			return attempt, nil
		}
		if !isSQLiteBusyError(err) || attempt >= len(accessLogWriteRetryDelays) {
			return attempt, err
		}
		time.Sleep(accessLogWriteRetryDelays[attempt])
	}
	return accessLogWriteMaxAttempts - 1, err
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "database is busy")
}

func updateAtomicMax(target *atomic.Int64, value int64) {
	for current := target.Load(); value > current; current = target.Load() {
		if target.CompareAndSwap(current, value) {
			return
		}
	}
}

var accessLogWriterState struct {
	sync.RWMutex
	writer *AccessLogWriter
}

type AccessLogWriterStats struct {
	QueueLength     int   `json:"queueLength"`
	QueueCap        int   `json:"queueCap"`
	QueuePeak       int64 `json:"queuePeak"`
	BatchSize       int   `json:"batchSize"`
	FlushIntervalMs int64 `json:"flushIntervalMs"`
	Enqueued        int64 `json:"enqueued"`
	Dropped         int64 `json:"dropped"`
	Written         int64 `json:"written"`
	Batches         int64 `json:"batches"`
	Retries         int64 `json:"retries"`
	WriteErrors     int64 `json:"writeErrors"`
	LastBatchUs     int64 `json:"lastBatchUs"`
	MaxBatchUs      int64 `json:"maxBatchUs"`
}

func (w *AccessLogWriter) Stats() AccessLogWriterStats {
	if w == nil {
		return AccessLogWriterStats{}
	}
	return AccessLogWriterStats{
		QueueLength:     len(w.queue),
		QueueCap:        cap(w.queue),
		QueuePeak:       w.queuePeak.Load(),
		BatchSize:       w.batchSize,
		FlushIntervalMs: w.flushInterval.Milliseconds(),
		Enqueued:        w.enqueued.Load(),
		Dropped:         w.dropped.Load(),
		Written:         w.written.Load(),
		Batches:         w.batches.Load(),
		Retries:         w.retries.Load(),
		WriteErrors:     w.writeErrors.Load(),
		LastBatchUs:     w.lastBatchUs.Load(),
		MaxBatchUs:      w.maxBatchUs.Load(),
	}
}

func CurrentAccessLogWriterStats() AccessLogWriterStats {
	accessLogWriterState.RLock()
	writer := accessLogWriterState.writer
	accessLogWriterState.RUnlock()
	return writer.Stats()
}

func StartAccessLogWriter(db *gorm.DB) error {
	accessLogWriterState.Lock()
	defer accessLogWriterState.Unlock()
	if accessLogWriterState.writer != nil {
		return errors.New("http access log writer already started")
	}
	queueSize := defaultAccessLogQueueSize
	batchSize := defaultAccessLogBatchSize
	flushInterval := defaultAccessLogFlushInterval
	if global.NAV_VIPER != nil {
		if value := global.NAV_VIPER.GetInt("navmesh.access-log-queue-size"); value > 0 {
			queueSize = value
		}
		if value := global.NAV_VIPER.GetInt("navmesh.access-log-batch-size"); value > 0 {
			batchSize = value
		}
		if value := global.NAV_VIPER.GetDuration("navmesh.access-log-flush-interval"); value > 0 {
			flushInterval = value
		}
	}
	writer := NewAccessLogWriter(db, queueSize, batchSize, flushInterval)
	writer.Start()
	accessLogWriterState.writer = writer
	return nil
}

func StopAccessLogWriter(ctx context.Context) error {
	accessLogWriterState.Lock()
	writer := accessLogWriterState.writer
	accessLogWriterState.writer = nil
	accessLogWriterState.Unlock()
	if writer == nil {
		return nil
	}
	return writer.Stop(ctx)
}

func writeAccessLog(input mappingLogInput) {
	accessLogWriterState.RLock()
	writer := accessLogWriterState.writer
	accessLogWriterState.RUnlock()
	if writer == nil {
		return
	}
	_ = writer.Enqueue(input)
}

func normalizeAccessLogInput(input mappingLogInput) mappingLogInput {
	input.MappingGuid = truncateUTF8(input.MappingGuid, 64)
	input.DeviceGuid = truncateUTF8(input.DeviceGuid, 64)
	input.Host = truncateUTF8(input.Host, 255)
	input.Method = truncateUTF8(input.Method, 16)
	input.Path = truncateUTF8(input.Path, 1024)
	input.SourceIP = truncateUTF8(input.SourceIP, 64)
	input.ErrorMessage = truncateUTF8(input.ErrorMessage, 512)
	if input.CreateTime <= 0 {
		input.CreateTime = domains.NowMilli()
	}
	return input
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && cut < len(value) && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return strings.Clone(value[:cut])
}
