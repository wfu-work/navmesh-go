package inits

import (
	"context"
	_ "embed"
	"fmt"
	"navmesh-go/domains"
	"navmesh-go/httpgateway"
	"navmesh-go/routers"
	"navmesh-go/services"
	"navmesh-go/sshgateway"
	"navmesh-go/tunnel"
	"navmesh-go/utils"
	"navmesh-go/webs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wfu-work/nav-common-go-lib/global"
	commonInits "github.com/wfu-work/nav-common-go-lib/inits"
	"go.uber.org/zap"
)

var tunnelServer *tunnel.Server
var sshServer *sshgateway.Server
var httpMappingServer *httpgateway.Server
var maintenanceCancel context.CancelFunc
var deviceOfflineCancel context.CancelFunc

//go:embed config.yaml
var defaultConfig []byte

func Init() {
	if err := utils.NewDefaultConfigManager(defaultConfig).Ensure(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "prepare config failed: %v\n", err)
		os.Exit(1)
	}
	sysInit := commonInits.SysInit{}
	sysInit.OnTableInit(registerTables)
	sysInit.OnRouterInit(func(publicGroup *gin.RouterGroup, privateGroup *gin.RouterGroup) {
		routers.RouterGroupApp.InitRouters(publicGroup, privateGroup)
	})
	sysInit.OnOtherInit(startBackgroundServers)
	sysInit.OnShutInit(stopBackgroundServers)
	sysInit.OnWebInit(func(router *gin.Engine) {
		_ = webs.InitStatic(router)
	})
	sysInit.Init()
}

func registerTables() {
	if err := ensureDataDirs(); err != nil {
		global.NAV_LOG.Error("ensure navmesh data dir failed", zap.Error(err))
	}
	err := global.NAV_DB.AutoMigrate(
		domains.Device{},
		domains.DeviceToken{},
		domains.DeviceConnection{},
		domains.DeviceHeartbeat{},
		domains.SSHAlias{},
		domains.SSHEntrypoint{},
		domains.PortMapping{},
		domains.CustomDomain{},
		domains.TunnelSession{},
		domains.HTTPAccessLog{},
		domains.AccessPolicy{},
		domains.DeviceGroup{},
		domains.Event{},
		domains.AuditLog{},
		domains.Setting{},
	)
	if err != nil {
		global.NAV_LOG.Error("register navmesh business table failed", zap.Error(err))
		return
	}
	seedDefaultSettings()
	services.ServiceGroupApp.GroupService.SeedDefaults()
	global.NAV_LOG.Info("register navmesh business table success")
}

func ensureDataDirs() error {
	dataDir := "./data/navmesh"
	if global.NAV_VIPER != nil {
		if value := global.NAV_VIPER.GetString("navmesh.data-dir"); value != "" {
			dataDir = value
		}
	}
	for _, dir := range []string{"./data", dataDir, filepath.Join(dataDir, "audit"), filepath.Join(dataDir, "cache")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	cachePath := "./data/cache.json"
	if global.NAV_VIPER != nil {
		if value := global.NAV_VIPER.GetString("local.cache-path"); value != "" {
			cachePath = value
		}
	}
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		if err := os.WriteFile(cachePath, []byte("{}"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func seedDefaultSettings() {
	defaults := map[string]string{
		"ssh_gateway_domain":               "sshd.navfirst.com",
		"ssh_listen":                       ":3010",
		"ssh_enabled":                      "true",
		"http_listen":                      ":3009",
		"http_mapping_enabled":             "true",
		"tunnel_listen":                    ":3008",
		"tunnel_enabled":                   "true",
		"device_register_token":            services.DefaultDeviceRegisterToken(),
		"device_heartbeat_timeout":         "90s",
		"device_offline_check_interval":    "30s",
		"session_idle_timeout":             "30m",
		"max_concurrent_sessions":          "0",
		"max_device_sessions":              "0",
		"rate_limit_per_minute":            "0",
		"retention_cleanup_enabled":        "true",
		"retention_cleanup_interval":       "24h",
		"audit_retention_days":             "90",
		"http_access_retention_days":       "30",
		"session_retention_days":           "90",
		"heartbeat_retention_days":         "7",
		"device_connection_retention_days": "30",
	}
	now := domains.NowMilli()
	for key, value := range defaults {
		var count int64
		if err := global.NAV_DB.Model(&domains.Setting{}).Where("key = ?", key).Count(&count).Error; err != nil {
			global.NAV_LOG.Warn("check navmesh default setting failed", zap.String("key", key), zap.Error(err))
			continue
		}
		if count > 0 {
			continue
		}
		row := domains.Setting{Key: key, Value: value, CreateTime: now, UpdateTime: now}
		if err := global.NAV_DB.Create(&row).Error; err != nil {
			global.NAV_LOG.Warn("seed navmesh default setting failed", zap.String("key", key), zap.Error(err))
		}
	}
}

func startBackgroundServers() {
	services.RegisterDeviceConnectionCloser(tunnel.DefaultManager)
	markBootStaleDevicesOffline()
	startMaintenanceJobs()
	startDeviceOfflineCleaner()
	startTunnelServer()
	startSSHServer()
	startHTTPMappingServer()
}

func stopBackgroundServers() {
	stopHTTPMappingServer()
	stopSSHServer()
	stopTunnelServer()
	stopDeviceOfflineCleaner()
	stopMaintenanceJobs()
}

func markBootStaleDevicesOffline() {
	affected, err := services.ServiceGroupApp.DeviceService.MarkOnlineDevicesOffline()
	if err != nil {
		global.NAV_LOG.Warn("mark boot online devices offline failed", zap.Error(err))
		return
	}
	if affected > 0 {
		global.NAV_LOG.Info("mark boot online devices offline", zap.Int64("affected", affected))
	}
}

func startMaintenanceJobs() {
	ctx, cancel := context.WithCancel(context.Background())
	maintenanceCancel = cancel
	services.ServiceGroupApp.MaintenanceService.StartRetentionCleaner(ctx)
}

func stopMaintenanceJobs() {
	if maintenanceCancel != nil {
		maintenanceCancel()
	}
}

func startDeviceOfflineCleaner() {
	ctx, cancel := context.WithCancel(context.Background())
	deviceOfflineCancel = cancel
	go services.ServiceGroupApp.DeviceService.StartOfflineCleaner(ctx)
}

func stopDeviceOfflineCleaner() {
	if deviceOfflineCancel != nil {
		deviceOfflineCancel()
	}
}

func startTunnelServer() {
	if strings.EqualFold(getSettingValue("tunnel_enabled", "true"), "false") {
		global.NAV_LOG.Info("navmesh quic tunnel server disabled")
		return
	}
	addr := getSettingValue("tunnel_listen", ":3008")
	tunnelServer = tunnel.NewServer(addr, tunnel.DefaultManager)
	if err := tunnelServer.Start(); err != nil {
		global.NAV_LOG.Error("start navmesh quic tunnel server failed", zap.String("addr", addr), zap.Error(err))
	}
}

func startSSHServer() {
	if strings.EqualFold(getSettingValue("ssh_enabled", "true"), "false") {
		global.NAV_LOG.Info("navmesh ssh gateway disabled")
		return
	}
	addr := getSettingValue("ssh_listen", ":22")
	sshServer = sshgateway.NewServer(addr, tunnel.DefaultManager)
	if err := sshServer.Start(); err != nil {
		global.NAV_LOG.Error("start navmesh ssh gateway failed", zap.String("addr", addr), zap.Error(err))
	}
}

func stopTunnelServer() {
	if tunnelServer != nil {
		tunnelServer.Stop()
	}
}

func stopSSHServer() {
	if sshServer != nil {
		sshServer.Stop()
	}
}

func startHTTPMappingServer() {
	if strings.EqualFold(getSettingValue("http_mapping_enabled", "true"), "false") {
		global.NAV_LOG.Info("navmesh http mapping gateway disabled")
		return
	}
	addr := getSettingValue("http_listen", ":8080")
	httpMappingServer = httpgateway.NewServer(addr, tunnel.DefaultManager)
	if err := httpMappingServer.Start(); err != nil {
		global.NAV_LOG.Error("start navmesh http mapping gateway failed", zap.String("addr", addr), zap.Error(err))
	}
}

func stopHTTPMappingServer() {
	if httpMappingServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpMappingServer.Stop(ctx)
	}
}

func getSettingValue(key, def string) string {
	var row domains.Setting
	if err := global.NAV_DB.Where("key = ?", key).First(&row).Error; err == nil && strings.TrimSpace(row.Value) != "" {
		return strings.TrimSpace(row.Value)
	}
	return def
}
