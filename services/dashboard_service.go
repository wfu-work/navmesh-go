package services

import (
	"time"

	"navmesh-go/domains"
)

type DashboardCountSummary struct {
	Total  int64 `json:"total"`
	Active int64 `json:"active"`
}

type DashboardTrafficDay struct {
	Day        string `json:"day"`
	RXBytes    int64  `json:"rxBytes"`
	TXBytes    int64  `json:"txBytes"`
	TotalBytes int64  `json:"totalBytes"`
}

type DashboardSummary struct {
	Devices        DashboardCountSummary `json:"devices"`
	Mappings       DashboardCountSummary `json:"mappings"`
	Events         DashboardCountSummary `json:"events"`
	RecentFailures int64                 `json:"recentFailures"`
	TrafficDays    []DashboardTrafficDay `json:"trafficDays"`
}

func (s DeviceService) Dashboard() (*DashboardSummary, error) {
	db := s.DB()
	result := &DashboardSummary{TrafficDays: make([]DashboardTrafficDay, 0)}

	if err := db.Model(&domains.Device{}).
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS active", domains.DeviceStatusOnline).
		Scan(&result.Devices).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domains.PortMapping{}).
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS active", int(domains.StatusEnabled)).
		Scan(&result.Mappings).Error; err != nil {
		return nil, err
	}
	eventQuery := db.Model(&domains.Event{}).Where("event_type <> ?", ignoredServiceLogEventType)
	eventQuery = withoutEventCenterNoise(eventQuery)
	if err := eventQuery.
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS active", int(domains.StatusEnabled)).
		Scan(&result.Events).Error; err != nil {
		return nil, err
	}
	recentSince := domains.NowMilli() - 24*time.Hour.Milliseconds()
	if err := db.Model(&domains.HttpAccessLog{}).
		Where("create_time >= ?", recentSince).
		Where("status_code >= 500 OR error_message <> ''").
		Count(&result.RecentFailures).Error; err != nil {
		return nil, err
	}
	startDay, endDay := trafficDayRange(map[string]string{"days": "30"})
	if err := db.Model(&domains.DeviceTrafficDaily{}).
		Select("day, COALESCE(SUM(rx_bytes), 0) AS rx_bytes, COALESCE(SUM(tx_bytes), 0) AS tx_bytes, COALESCE(SUM(total_bytes), 0) AS total_bytes").
		Where("day >= ? AND day <= ?", startDay, endDay).
		Group("day").
		Order("day ASC").
		Scan(&result.TrafficDays).Error; err != nil {
		return nil, err
	}
	return result, nil
}
