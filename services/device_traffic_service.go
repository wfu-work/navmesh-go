package services

import (
	"errors"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DeviceTrafficService struct {
	db *gorm.DB
}

type DeviceTrafficSummary struct {
	RXBytes    int64 `json:"rxBytes"`
	TXBytes    int64 `json:"txBytes"`
	TotalBytes int64 `json:"totalBytes"`
}

func (s DeviceTrafficService) WithDB(db *gorm.DB) DeviceTrafficService {
	s.db = db
	return s
}

func (s DeviceTrafficService) DB() *gorm.DB {
	if s.db != nil {
		return s.db
	}
	return global.NAV_DB
}

func (s DeviceTrafficService) RecordHeartbeatSample(deviceGuid string, req HeartbeatRequest, now int64) error {
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return nil
	}
	iface := normalizeTrafficInterface(req.TrafficIface)
	if iface == "" || req.TrafficRXBytes < 0 || req.TrafficTXBytes < 0 {
		return nil
	}
	sampleTime := req.TrafficSampleTime
	if sampleTime <= 0 {
		sampleTime = now
	}
	if sampleTime < now-10*60*1000 || sampleTime > now+10*60*1000 {
		sampleTime = now
	}
	return s.DB().Transaction(func(tx *gorm.DB) error {
		service := s.WithDB(tx)
		var state domains.DeviceTrafficState
		err := tx.Where("device_guid = ? AND iface = ?", deviceGuid, iface).First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&domains.DeviceTrafficState{
				DeviceGuid:    deviceGuid,
				Interface:     iface,
				TrafficBootID: strings.TrimSpace(req.TrafficBootID),
				RXBytes:       req.TrafficRXBytes,
				TXBytes:       req.TrafficTXBytes,
				SampleTime:    sampleTime,
				CreateTime:    now,
				UpdateTime:    now,
			}).Error
		}
		if err != nil {
			return err
		}
		reset := trafficCounterReset(state, req)
		rxDelta := req.TrafficRXBytes - state.RXBytes
		txDelta := req.TrafficTXBytes - state.TXBytes
		if reset {
			rxDelta = 0
			txDelta = 0
		}
		if rxDelta > 0 || txDelta > 0 || reset {
			resetCount := int64(0)
			if reset {
				resetCount = 1
			}
			if err := service.upsertDailyDelta(deviceGuid, iface, sampleTime, rxDelta, txDelta, resetCount, now); err != nil {
				return err
			}
		}
		return tx.Model(&domains.DeviceTrafficState{}).
			Where("device_guid = ? AND iface = ?", deviceGuid, iface).
			Updates(map[string]any{
				"traffic_boot_id": strings.TrimSpace(req.TrafficBootID),
				"rx_bytes":        req.TrafficRXBytes,
				"tx_bytes":        req.TrafficTXBytes,
				"sample_time":     sampleTime,
				"update_time":     now,
			}).Error
	})
}

func (s DeviceTrafficService) Daily(params map[string]string) ([]domains.DeviceTrafficDaily, DeviceTrafficSummary, error) {
	startDay, endDay := trafficDayRange(params)
	db := s.DB().Model(&domains.DeviceTrafficDaily{}).Where("day >= ? AND day <= ?", startDay, endDay)
	if deviceGuid := strings.TrimSpace(params["deviceGuid"]); deviceGuid != "" {
		db = db.Where("device_guid = ?", deviceGuid)
	}
	if iface := normalizeTrafficInterface(params["iface"]); iface != "" {
		db = db.Where("iface = ?", iface)
	}
	summary := DeviceTrafficSummary{}
	if err := db.Select("COALESCE(SUM(rx_bytes), 0) AS rx_bytes, COALESCE(SUM(tx_bytes), 0) AS tx_bytes, COALESCE(SUM(total_bytes), 0) AS total_bytes").Scan(&summary).Error; err != nil {
		return nil, DeviceTrafficSummary{}, err
	}
	var items []domains.DeviceTrafficDaily
	if err := db.Order("day ASC, device_guid ASC, iface ASC").Find(&items).Error; err != nil {
		return nil, DeviceTrafficSummary{}, err
	}
	return items, summary, nil
}

func (s DeviceTrafficService) upsertDailyDelta(deviceGuid, iface string, sampleTime, rxDelta, txDelta, resetCount, now int64) error {
	totalDelta := rxDelta + txDelta
	day := trafficDay(sampleTime)
	row := domains.DeviceTrafficDaily{
		DeviceGuid:    deviceGuid,
		Interface:     iface,
		Day:           day,
		RXBytes:       rxDelta,
		TXBytes:       txDelta,
		TotalBytes:    totalDelta,
		SampleCount:   1,
		ResetCount:    resetCount,
		FirstSeenTime: sampleTime,
		LastSeenTime:  sampleTime,
		CreateTime:    now,
		UpdateTime:    now,
	}
	return s.DB().Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "device_guid"},
			{Name: "iface"},
			{Name: "day"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"rx_bytes":       gorm.Expr("rx_bytes + ?", rxDelta),
			"tx_bytes":       gorm.Expr("tx_bytes + ?", txDelta),
			"total_bytes":    gorm.Expr("total_bytes + ?", totalDelta),
			"sample_count":   gorm.Expr("sample_count + ?", 1),
			"reset_count":    gorm.Expr("reset_count + ?", resetCount),
			"last_seen_time": sampleTime,
			"update_time":    now,
		}),
	}).Create(&row).Error
}

func trafficCounterReset(state domains.DeviceTrafficState, req HeartbeatRequest) bool {
	if req.TrafficRXBytes < state.RXBytes || req.TrafficTXBytes < state.TXBytes {
		return true
	}
	bootID := strings.TrimSpace(req.TrafficBootID)
	return bootID != "" && strings.TrimSpace(state.TrafficBootID) != "" && bootID != strings.TrimSpace(state.TrafficBootID)
}

func normalizeTrafficInterface(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func trafficDayRange(params map[string]string) (string, string) {
	end := strings.TrimSpace(params["to"])
	if end == "" {
		end = trafficDay(domains.NowMilli())
	}
	start := strings.TrimSpace(params["from"])
	if start != "" {
		return normalizeTrafficDay(start, end), normalizeTrafficDay(end, trafficDay(domains.NowMilli()))
	}
	days := utils.Str2Int(params["days"])
	if days <= 0 {
		days = 30
	}
	if days > 370 {
		days = 370
	}
	endTime := parseTrafficDay(end)
	startTime := endTime.AddDate(0, 0, -days+1)
	return startTime.Format("2006-01-02"), endTime.Format("2006-01-02")
}

func normalizeTrafficDay(value, fallback string) string {
	if _, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		return value
	}
	return fallback
}

func parseTrafficDay(day string) time.Time {
	if t, err := time.ParseInLocation("2006-01-02", day, time.Local); err == nil {
		return t
	}
	now := time.Now().In(time.Local)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func trafficDay(sampleTime int64) string {
	if sampleTime <= 0 {
		sampleTime = domains.NowMilli()
	}
	return time.UnixMilli(sampleTime).In(time.Local).Format("2006-01-02")
}
