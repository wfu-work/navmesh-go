package httpgateway

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CachedRoute struct {
	Mapping   domains.PortMapping
	Device    domains.Device
	AllowHTTP bool
}

type RouteSnapshot struct {
	Version int64
	ByHost  map[string]CachedRoute
}

type RouteStore struct {
	db       *gorm.DB
	current  atomic.Pointer[RouteSnapshot]
	reloadCh chan struct{}
}

type RouteStoreStats struct {
	Ready   bool  `json:"ready"`
	Version int64 `json:"version"`
	Routes  int   `json:"routes"`
}

func NewRouteStore(db *gorm.DB) *RouteStore {
	return &RouteStore{db: db, reloadCh: make(chan struct{}, 1)}
}

func (s *RouteStore) DB() *gorm.DB {
	if s != nil && s.db != nil {
		return s.db
	}
	return global.NAV_DB
}

func (s *RouteStore) Ready() bool {
	return s != nil && s.current.Load() != nil
}

func (s *RouteStore) Stats() RouteStoreStats {
	if s == nil {
		return RouteStoreStats{}
	}
	snapshot := s.current.Load()
	if snapshot == nil {
		return RouteStoreStats{}
	}
	return RouteStoreStats{Ready: true, Version: snapshot.Version, Routes: len(snapshot.ByHost)}
}

func (s *RouteStore) Lookup(host string) (CachedRoute, bool) {
	if s == nil {
		return CachedRoute{}, false
	}
	snapshot := s.current.Load()
	if snapshot == nil {
		return CachedRoute{}, false
	}
	route, ok := snapshot.ByHost[normalizeHost(host)]
	return route, ok
}

func (s *RouteStore) Reload() error {
	db := s.DB()
	if db == nil {
		return errors.New("route store database unavailable")
	}
	var mappings []domains.PortMapping
	if err := db.Where("status = ?", int(domains.StatusEnabled)).Find(&mappings).Error; err != nil {
		return err
	}
	deviceIDs := make([]string, 0, len(mappings))
	seenDevices := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		if mapping.Protocol != "" && mapping.Protocol != "http" && mapping.Protocol != "https" {
			continue
		}
		if _, ok := seenDevices[mapping.DeviceGuid]; !ok {
			seenDevices[mapping.DeviceGuid] = struct{}{}
			deviceIDs = append(deviceIDs, mapping.DeviceGuid)
		}
	}
	var devices []domains.Device
	if len(deviceIDs) > 0 {
		if err := db.Where("guid IN ? AND status <> ?", deviceIDs, domains.DeviceStatusDisabled).Find(&devices).Error; err != nil {
			return err
		}
	}
	deviceByID := make(map[string]domains.Device, len(devices))
	for _, device := range devices {
		deviceByID[device.Guid] = device
	}
	var policies []domains.AccessPolicy
	if err := db.Where("status = ?", int(domains.StatusEnabled)).Find(&policies).Error; err != nil {
		return err
	}
	routes := make(map[string]CachedRoute, len(mappings))
	for _, mapping := range mappings {
		if mapping.Protocol != "" && mapping.Protocol != "http" && mapping.Protocol != "https" {
			continue
		}
		device, ok := deviceByID[mapping.DeviceGuid]
		if !ok {
			continue
		}
		host := normalizeHost(mapping.PublicHost)
		if host == "" {
			continue
		}
		routes[host] = CachedRoute{
			Mapping:   mapping,
			Device:    device,
			AllowHTTP: routeAllowsHTTP(device, mapping, policies),
		}
	}
	s.current.Store(&RouteSnapshot{Version: time.Now().UnixMilli(), ByHost: routes})
	return nil
}

func routeAllowsHTTP(device domains.Device, mapping domains.PortMapping, policies []domains.AccessPolicy) bool {
	allowed := true
	for _, policy := range policies {
		if !routePolicyApplies(policy, device, mapping) {
			continue
		}
		allowed = allowed && policy.AllowHTTP
	}
	return allowed
}

func routePolicyApplies(policy domains.AccessPolicy, device domains.Device, mapping domains.PortMapping) bool {
	switch strings.ToLower(strings.TrimSpace(policy.Scope)) {
	case "global":
		return true
	case "device":
		return strings.TrimSpace(policy.TargetID) == device.Guid
	case "group":
		target := strings.TrimSpace(policy.TargetID)
		return target != "" && (target == strings.TrimSpace(device.GroupGuid) || target == strings.TrimSpace(device.DeviceType))
	case "mapping":
		return strings.TrimSpace(policy.TargetID) == mapping.Guid
	default:
		return false
	}
}

func (s *RouteStore) RequestReload() {
	if s == nil {
		return
	}
	select {
	case s.reloadCh <- struct{}{}:
	default:
	}
}

func (s *RouteStore) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.reloadCh:
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
			for {
				select {
				case <-s.reloadCh:
				default:
					if err := s.Reload(); err != nil && global.NAV_LOG != nil {
						global.NAV_LOG.Warn("reload http route cache failed", zap.Error(err))
					}
					goto reloaded
				}
			}
		reloaded:
		}
	}
}

var DefaultRouteStore = NewRouteStore(nil)
