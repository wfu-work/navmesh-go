package services

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
)

type SessionPermit struct {
	DeviceGuid string
	SourceIP   string
}

type RuntimePolicy struct {
	mu           sync.Mutex
	total        int
	byDevice     map[string]int
	sourceHits   map[string][]time.Time
	lastHitSweep time.Time
	lastSettings time.Time
	settings     RuntimeSettings
}

type RuntimeSettings struct {
	MaxConcurrentSessions int
	MaxDeviceSessions     int
	RateLimitPerMinute    int
	SessionIdleTimeout    time.Duration
}

func NewRuntimePolicy() *RuntimePolicy {
	return &RuntimePolicy{
		byDevice:   make(map[string]int),
		sourceHits: make(map[string][]time.Time),
		settings:   defaultRuntimeSettings(),
	}
}

func (p *RuntimePolicy) Acquire(deviceGuid, sourceIP string) (*SessionPermit, error) {
	deviceGuid = strings.TrimSpace(deviceGuid)
	sourceIP = strings.TrimSpace(sourceIP)
	settings := p.currentSettings()
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sweepSourceHitsLocked(now)
	if settings.RateLimitPerMinute > 0 && sourceIP != "" {
		windowStart := now.Add(-time.Minute)
		hits := p.sourceHits[sourceIP]
		kept := hits[:0]
		for _, hit := range hits {
			if hit.After(windowStart) {
				kept = append(kept, hit)
			}
		}
		if len(kept) >= settings.RateLimitPerMinute {
			p.sourceHits[sourceIP] = kept
			return nil, errors.New("rate limit exceeded")
		}
		p.sourceHits[sourceIP] = append(kept, now)
	}
	if settings.MaxConcurrentSessions > 0 && p.total >= settings.MaxConcurrentSessions {
		return nil, errors.New("max concurrent sessions exceeded")
	}
	if settings.MaxDeviceSessions > 0 && deviceGuid != "" && p.byDevice[deviceGuid] >= settings.MaxDeviceSessions {
		return nil, errors.New("max device sessions exceeded")
	}
	p.total++
	if deviceGuid != "" {
		p.byDevice[deviceGuid]++
	}
	return &SessionPermit{DeviceGuid: deviceGuid, SourceIP: sourceIP}, nil
}

func (p *RuntimePolicy) sweepSourceHitsLocked(now time.Time) {
	if !p.lastHitSweep.IsZero() && now.Sub(p.lastHitSweep) < time.Minute {
		return
	}
	windowStart := now.Add(-time.Minute)
	for sourceIP, hits := range p.sourceHits {
		kept := hits[:0]
		for _, hit := range hits {
			if hit.After(windowStart) {
				kept = append(kept, hit)
			}
		}
		if len(kept) == 0 {
			delete(p.sourceHits, sourceIP)
			continue
		}
		p.sourceHits[sourceIP] = kept
	}
	p.lastHitSweep = now
}

func (p *RuntimePolicy) Release(permit *SessionPermit) {
	if permit == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.total > 0 {
		p.total--
	}
	if permit.DeviceGuid != "" && p.byDevice[permit.DeviceGuid] > 0 {
		p.byDevice[permit.DeviceGuid]--
		if p.byDevice[permit.DeviceGuid] == 0 {
			delete(p.byDevice, permit.DeviceGuid)
		}
	}
}

func (p *RuntimePolicy) IdleTimeout() time.Duration {
	return p.currentSettings().SessionIdleTimeout
}

func (p *RuntimePolicy) Snapshot() RuntimeSettings {
	return p.currentSettings()
}

func (p *RuntimePolicy) currentSettings() RuntimeSettings {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.lastSettings) < 5*time.Second {
		return p.settings
	}
	p.settings = loadRuntimeSettings()
	p.lastSettings = time.Now()
	return p.settings
}

func loadRuntimeSettings() RuntimeSettings {
	settings := defaultRuntimeSettings()
	settings.MaxConcurrentSessions = settingIntValue("max_concurrent_sessions", settings.MaxConcurrentSessions)
	settings.MaxDeviceSessions = settingIntValue("max_device_sessions", settings.MaxDeviceSessions)
	settings.RateLimitPerMinute = settingIntValue("rate_limit_per_minute", settings.RateLimitPerMinute)
	settings.SessionIdleTimeout = settingDurationValue("session_idle_timeout", settings.SessionIdleTimeout)
	return settings
}

func defaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		MaxConcurrentSessions: 0,
		MaxDeviceSessions:     0,
		RateLimitPerMinute:    0,
		SessionIdleTimeout:    30 * time.Minute,
	}
}

func settingIntValue(key string, def int) int {
	var row domains.Setting
	if err := global.NAV_DB.Where("key = ?", key).First(&row).Error; err != nil {
		return def
	}
	value, err := strconv.Atoi(strings.TrimSpace(row.Value))
	if err != nil {
		return def
	}
	return value
}

func settingDurationValue(key string, def time.Duration) time.Duration {
	var row domains.Setting
	if err := global.NAV_DB.Where("key = ?", key).First(&row).Error; err != nil {
		return def
	}
	value, err := time.ParseDuration(strings.TrimSpace(row.Value))
	if err != nil || value <= 0 {
		return def
	}
	return value
}

var DefaultRuntimePolicy = NewRuntimePolicy()
