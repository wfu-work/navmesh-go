package inits

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"navmesh-go/domains"
	"navmesh-go/httpgateway"
	"navmesh-go/routers"
	"navmesh-go/services"
	"navmesh-go/sshgateway"
	"navmesh-go/tcpgateway"
	"navmesh-go/tunnel"
	"navmesh-go/utils"
	"navmesh-go/webs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"github.com/wfu-work/nav-common-go-lib/global"
	commonInits "github.com/wfu-work/nav-common-go-lib/inits"
	"github.com/wfu-work/nav-common-go-lib/scheduleds"
	"go.uber.org/zap"
	"gorm.io/gorm/logger"
)

var tunnelServer *tunnel.Server
var sshServer *sshgateway.Server
var httpMappingServer *httpgateway.Server
var tcpMappingServer *tcpgateway.Server
var maintenanceTimer scheduleds.Timer
var deviceOfflineCancel context.CancelFunc
var deviceLivenessCancel context.CancelFunc
var deviceLivenessDone chan struct{}
var httpRouteCancel context.CancelFunc
var httpRouteDone chan struct{}

const (
	sqliteBusyTimeoutMs     = 5000
	sqlitePragmaWarmupLimit = 8
	sqlitePragmaInitTimeout = 10 * time.Second
)

var sqlitePerformanceIndexStatements = []string{
	"CREATE INDEX IF NOT EXISTS idx_event_suppress ON navmesh_events(device_guid, event_type, create_time DESC)",
	"CREATE INDEX IF NOT EXISTS idx_event_active_time_id ON navmesh_events(create_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_event_device_active_time_id ON navmesh_events(device_guid, create_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_connection_status_active ON navmesh_device_connections(status, last_active_time)",
	"CREATE INDEX IF NOT EXISTS idx_connection_status_update ON navmesh_device_connections(status, update_time)",
	"CREATE INDEX IF NOT EXISTS idx_device_token_validate ON navmesh_device_tokens(device_guid, token_hash, status, expire_time)",
	"CREATE INDEX IF NOT EXISTS idx_upgrade_pending ON navmesh_device_upgrade_tasks(device_guid, status, create_time, update_time)",
	"CREATE INDEX IF NOT EXISTS idx_device_status_seen ON navmesh_devices(status, last_seen_time)",
	"CREATE INDEX IF NOT EXISTS idx_device_active_update_time_id ON navmesh_devices(update_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_device_status_active_update_time_id ON navmesh_devices(status, update_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_mapping_active_update_time_id ON navmesh_port_mappings(update_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_mapping_device_active_update_time_id ON navmesh_port_mappings(device_guid, update_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_mapping_status_active_update_time_id ON navmesh_port_mappings(status, update_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_tcp_mapping_active_update_time_id ON navmesh_tcp_mappings(update_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_tcp_mapping_device_active_update_time_id ON navmesh_tcp_mappings(device_guid, update_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_tcp_mapping_status_active_update_time_id ON navmesh_tcp_mappings(status, update_time DESC, id DESC) WHERE deleted_time IS NULL",
	"CREATE INDEX IF NOT EXISTS idx_heartbeat_device_time ON navmesh_device_heartbeats(device_guid, create_time DESC)",
	"CREATE INDEX IF NOT EXISTS idx_http_log_mapping_time_id ON navmesh_http_access_logs(mapping_guid, create_time DESC, id DESC)",
	"CREATE INDEX IF NOT EXISTS idx_http_log_device_time_id ON navmesh_http_access_logs(device_guid, create_time DESC, id DESC)",
	"CREATE INDEX IF NOT EXISTS idx_http_log_host_time_id ON navmesh_http_access_logs(host, create_time DESC, id DESC)",
	"CREATE INDEX IF NOT EXISTS idx_http_log_failure_time ON navmesh_http_access_logs(create_time DESC) WHERE status_code >= 500 OR error_message <> ''",
	"CREATE INDEX IF NOT EXISTS idx_session_device_time_id ON navmesh_tunnel_sessions(device_guid, start_time DESC, id DESC)",
	"CREATE INDEX IF NOT EXISTS idx_session_device_status_time_id ON navmesh_tunnel_sessions(device_guid, status, start_time DESC, id DESC)",
	"CREATE INDEX IF NOT EXISTS idx_session_type_time_id ON navmesh_tunnel_sessions(session_type, start_time DESC, id DESC)",
	"CREATE INDEX IF NOT EXISTS idx_session_public_time_id ON navmesh_tunnel_sessions(public_host, start_time DESC, id DESC)",
	"CREATE INDEX IF NOT EXISTS idx_session_status_time_id ON navmesh_tunnel_sessions(status, start_time DESC, id DESC)",
	"CREATE INDEX IF NOT EXISTS idx_session_status_end_time ON navmesh_tunnel_sessions(status, end_time)",
}

var sqliteRedundantPerformanceIndexes = []string{
	"idx_session_device_status_time",
	"idx_navmesh_tunnel_sessions_device_guid",
	"idx_navmesh_tunnel_sessions_session_type",
	"idx_navmesh_tunnel_sessions_public_host",
	"idx_navmesh_tunnel_sessions_status",
	"idx_http_log_mapping_time",
	"idx_http_log_device_time",
	"idx_navmesh_http_access_logs_mapping_guid",
	"idx_navmesh_http_access_logs_device_guid",
	"idx_navmesh_http_access_logs_host",
	"idx_device_update_time",
}

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
	sysInit.OnScheInit(registerScheduledJobs)
	sysInit.OnOtherInit(startBackgroundServers)
	sysInit.OnShutInit(stopBackgroundServers)
	sysInit.OnWebInit(func(router *gin.Engine) {
		_ = webs.InitStatic(router)
	})
	sysInit.Init()
}

func registerTables() {
	configureDatabase()
	warnInsecureRuntimeConfig()
	if err := ensureDataDirs(); err != nil {
		panic(fmt.Errorf("ensure navmesh data dir: %w", err))
	}
	err := global.NAV_DB.AutoMigrate(
		domains.Device{},
		domains.DeviceToken{},
		domains.Release{},
		domains.DeviceUpgradeBatch{},
		domains.DeviceUpgradeTask{},
		domains.DeviceConnection{},
		domains.DeviceHeartbeat{},
		domains.DeviceTrafficState{},
		domains.DeviceTrafficDaily{},
		domains.SSHAlias{},
		domains.SSHEntrypoint{},
		domains.PortMapping{},
		domains.TCPMapping{},
		domains.CustomDomain{},
		domains.TunnelSession{},
		domains.HttpAccessLog{},
		domains.AccessPolicy{},
		domains.DeviceGroup{},
		domains.Event{},
		domains.MessageEmailConfig{},
		domains.MessageTemplate{},
		domains.MessageRecipient{},
		domains.MessageSendRecord{},
		domains.AuditLog{},
		domains.Setting{},
	)
	if err != nil {
		panic(fmt.Errorf("register navmesh business table: %w", err))
	}
	ensurePerformanceIndexes()
	seedDefaultSettings()
	services.ServiceGroupApp.GroupService.SeedDefaults()
	services.ServiceGroupApp.MessageService.SeedDefaults()
	global.NAV_LOG.Info("register navmesh business table success")
}

func warnInsecureRuntimeConfig() {
	if global.NAV_VIPER == nil || global.NAV_LOG == nil {
		return
	}
	if value := strings.TrimSpace(global.NAV_VIPER.GetString("jwt.signing-key")); value == "" || value == "navmesh" {
		global.NAV_LOG.Warn("insecure default jwt signing key is configured; rotate it before deployment")
	}
	if value := strings.TrimSpace(global.NAV_VIPER.GetString("navmesh.device-register-token")); value == "" || value == "navfirst@2020" {
		global.NAV_LOG.Warn("insecure default device registration token is configured; rotate it before deployment")
	}
}

func ensurePerformanceIndexes() {
	if global.NAV_DB == nil || global.NAV_VIPER == nil || !strings.EqualFold(global.NAV_VIPER.GetString("system.db-type"), "sqlite") {
		return
	}
	for _, statement := range sqlitePerformanceIndexStatements {
		if err := global.NAV_DB.Exec(statement).Error; err != nil {
			global.NAV_LOG.Warn("create navmesh performance index failed", zap.String("statement", statement), zap.Error(err))
		}
	}
	// These indexes are strict prefixes of the replacements above. Removing
	// them after the new indexes exist avoids paying twice on every session or
	// access-log write when upgrading an existing SQLite database.
	for _, index := range sqliteRedundantPerformanceIndexes {
		statement := "DROP INDEX IF EXISTS " + index
		if err := global.NAV_DB.Exec(statement).Error; err != nil {
			global.NAV_LOG.Warn("drop redundant navmesh index failed", zap.String("statement", statement), zap.Error(err))
		}
	}
}

func configureDatabase() {
	if global.NAV_DB == nil {
		return
	}
	isSQLite := global.NAV_VIPER != nil && strings.EqualFold(global.NAV_VIPER.GetString("system.db-type"), "sqlite")
	if isSQLite && strings.TrimSpace(global.NAV_VIPER.GetString("sqlite.log-mode")) == "" && global.NAV_DB.Config.Logger != nil {
		global.NAV_DB.Config.Logger = global.NAV_DB.Config.Logger.LogMode(logger.Warn)
	}
	slowThreshold := 200 * time.Millisecond
	if global.NAV_VIPER != nil {
		if value := global.NAV_VIPER.GetDuration("navmesh.database-slow-threshold"); value > 0 {
			slowThreshold = value
		}
	}
	services.DefaultDatabaseQueryMetrics.SetSlowThreshold(slowThreshold)
	if err := services.RegisterDatabaseQueryMetrics(global.NAV_DB, services.DefaultDatabaseQueryMetrics); err != nil && global.NAV_LOG != nil {
		global.NAV_LOG.Warn("register database query metrics failed", zap.Error(err))
	}
	if !isSQLite {
		return
	}
	sqlDB, err := global.NAV_DB.DB()
	if err != nil {
		if global.NAV_LOG != nil {
			global.NAV_LOG.Warn("get sqlite connection pool failed", zap.Error(err))
		}
		return
	}
	maxOpen := global.NAV_VIPER.GetInt("sqlite.max-open-conns")
	if maxOpen <= 0 {
		maxOpen = sqlitePragmaWarmupLimit
	}
	maxIdle := global.NAV_VIPER.GetInt("sqlite.max-idle-conns")
	if maxIdle <= 0 || maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	if err := configureSQLiteConnections(sqlDB, maxOpen); err != nil {
		if global.NAV_LOG != nil {
			global.NAV_LOG.Warn("configure sqlite pragmas failed", zap.Error(err))
		}
	}
	if global.NAV_LOG != nil {
		global.NAV_LOG.Info("configure sqlite connection pool", zap.Int("maxOpen", maxOpen), zap.Int("maxIdle", maxIdle))
	}
}

// configureSQLiteConnections initializes connection-local SQLite PRAGMAs on
// the initial pool connections, up to sqlitePragmaWarmupLimit.
// database/sql applies Conn.Exec only to the selected connection, so opening
// and holding the connections concurrently is intentional: it forces the
// pool to create distinct connections instead of repeatedly reusing one.
func configureSQLiteConnections(sqlDB *sql.DB, maxOpen int) error {
	if sqlDB == nil {
		return fmt.Errorf("sqlite connection pool is nil")
	}
	if maxOpen <= 0 {
		maxOpen = 1
	}
	warmupCount := maxOpen
	if warmupCount > sqlitePragmaWarmupLimit {
		warmupCount = sqlitePragmaWarmupLimit
	}

	ctx, cancel := context.WithTimeout(context.Background(), sqlitePragmaInitTimeout)
	defer cancel()

	connections := make([]*sql.Conn, 0, warmupCount)
	var initErr error
	for index := 0; index < warmupCount; index++ {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			initErr = errors.Join(initErr, fmt.Errorf("open sqlite connection %d/%d: %w", index+1, warmupCount, err))
			break
		}
		connections = append(connections, conn)
	}
	for index, conn := range connections {
		for _, statement := range sqliteConnectionPragmas() {
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				initErr = errors.Join(initErr, fmt.Errorf("connection %d: %s: %w", index+1, statement, err))
			}
		}
	}
	for index := len(connections) - 1; index >= 0; index-- {
		if err := connections[index].Close(); err != nil {
			initErr = errors.Join(initErr, fmt.Errorf("close sqlite connection %d: %w", index+1, err))
		}
	}
	return initErr
}

func sqliteConnectionPragmas() []string {
	return []string{
		fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeoutMs),
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	}
}

func ensureDataDirs() error {
	dataDir := "./data/navmesh"
	if global.NAV_VIPER != nil {
		if value := global.NAV_VIPER.GetString("navmesh.data-dir"); value != "" {
			dataDir = value
		}
	}
	ossDir := "./data/oss"
	if global.NAV_VIPER != nil {
		if value := strings.TrimSpace(global.NAV_VIPER.GetString("local.oss-path")); value != "" {
			ossDir = value
		}
	}
	for _, dir := range []string{"./data", dataDir, filepath.Join(dataDir, "audit"), filepath.Join(dataDir, "cache"), filepath.Join(ossDir, "releases")} {
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
		"http_gateway_domain":              "httpd.navfirst.com",
		"ssh_listen":                       ":3010",
		"ssh_enabled":                      "true",
		"http_listen":                      ":3009",
		"http_mapping_enabled":             "true",
		"tcp_gateway_domain":               "tcpd.navfirst.com",
		"tcp_mapping_enabled":              "true",
		"tcp_public_port_min":              "20000",
		"tcp_public_port_max":              "29999",
		"tunnel_listen":                    ":3008",
		"tunnel_enabled":                   "true",
		"device_register_token":            services.DefaultDeviceRegisterToken(),
		"device_heartbeat_timeout":         "180s",
		"device_offline_check_interval":    "30s",
		"device_offline_event_delay":       "600s",
		"telemetry_sample_interval":        "5m",
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
		"traffic_daily_retention_days":     "370",
		"device_connection_retention_days": "30",
		"client_download_base":             "",
		"client_upgrade_enabled":           "true",
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
	services.DefaultNotificationRunner.Start(notificationWorkerCount(), notificationQueueSize())
	tunnel.DefaultManager.SetActivitySink(services.DefaultLivenessRegistry)
	markBootStaleDevicesOffline()
	startDeviceLivenessFlusher()
	startDeviceOfflineCleaner()
	startHTTPRuntime()
	startTunnelServer()
	startSSHServer()
	startHTTPMappingServer()
	startTCPMappingServer()
}

func stopBackgroundServers() {
	stopTCPMappingServer()
	stopHTTPMappingServer()
	stopHTTPRuntime()
	stopSSHServer()
	stopTunnelServer()
	stopDeviceOfflineCleaner()
	stopDeviceLivenessFlusher()
	stopNotificationRunner()
	stopMaintenanceJobs()
}

func stopNotificationRunner() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := services.DefaultNotificationRunner.Stop(ctx); err != nil {
		global.NAV_LOG.Warn("stop notification runner failed", zap.Error(err))
	}
}

func startHTTPRuntime() {
	services.RegisterHTTPRouteReloader(httpgateway.DefaultRouteStore.RequestReload)
	if err := httpgateway.DefaultRouteStore.Reload(); err != nil {
		global.NAV_LOG.Warn("load initial http route cache failed", zap.Error(err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	httpRouteCancel = cancel
	httpRouteDone = make(chan struct{})
	go func() {
		defer close(httpRouteDone)
		httpgateway.DefaultRouteStore.Run(ctx)
	}()
	if err := httpgateway.StartAccessLogWriter(global.NAV_DB); err != nil {
		global.NAV_LOG.Warn("start http access log writer failed", zap.Error(err))
	}
}

func stopHTTPRuntime() {
	services.RegisterHTTPRouteReloader(nil)
	if httpRouteCancel != nil {
		httpRouteCancel()
	}
	if httpRouteDone != nil {
		select {
		case <-httpRouteDone:
		case <-time.After(2 * time.Second):
			global.NAV_LOG.Warn("wait http route cache timeout")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpgateway.StopAccessLogWriter(ctx); err != nil {
		global.NAV_LOG.Warn("stop http access log writer failed", zap.Error(err))
	}
}

func startDeviceLivenessFlusher() {
	ctx, cancel := context.WithCancel(context.Background())
	deviceLivenessCancel = cancel
	deviceLivenessDone = make(chan struct{})
	go func() {
		defer close(deviceLivenessDone)
		services.DefaultLivenessRegistry.Run(ctx, livenessFlushInterval())
	}()
}

func livenessFlushInterval() time.Duration {
	if global.NAV_VIPER != nil {
		if value := global.NAV_VIPER.GetDuration("navmesh.liveness-flush-interval"); value > 0 {
			return value
		}
	}
	return 30 * time.Second
}

func notificationWorkerCount() int {
	if global.NAV_VIPER != nil {
		if value := global.NAV_VIPER.GetInt("navmesh.notification-workers"); value > 0 {
			return value
		}
	}
	return 4
}

func notificationQueueSize() int {
	if global.NAV_VIPER != nil {
		if value := global.NAV_VIPER.GetInt("navmesh.notification-queue-size"); value > 0 {
			return value
		}
	}
	return 1024
}

func stopDeviceLivenessFlusher() {
	if deviceLivenessCancel != nil {
		deviceLivenessCancel()
	}
	if deviceLivenessDone != nil {
		select {
		case <-deviceLivenessDone:
		case <-time.After(5 * time.Second):
			global.NAV_LOG.Warn("wait device liveness flusher timeout")
		}
	}
}

func markBootStaleDevicesOffline() {
	affected, err := services.ServiceGroupApp.DeviceService.MarkStaleOnlineDevicesOffline(0)
	if err != nil {
		global.NAV_LOG.Warn("mark boot online devices offline failed", zap.Error(err))
		return
	}
	if affected > 0 {
		global.NAV_LOG.Info("mark boot online devices offline", zap.Int64("affected", affected))
	}
}

func registerScheduledJobs(timer scheduleds.Timer, options []cron.Option) {
	maintenanceTimer = timer
	services.ServiceGroupApp.MaintenanceService.RegisterRetentionCleaner(timer, options)
}

func stopMaintenanceJobs() {
	if maintenanceTimer != nil {
		services.ServiceGroupApp.MaintenanceService.StopRetentionCleaner(maintenanceTimer)
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
		panic(fmt.Errorf("start navmesh tunnel server at %s: %w", addr, err))
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
		panic(fmt.Errorf("start navmesh ssh gateway at %s: %w", addr, err))
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
		panic(fmt.Errorf("start navmesh http mapping gateway at %s: %w", addr, err))
	}
}

func stopHTTPMappingServer() {
	if httpMappingServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpMappingServer.Stop(ctx)
	}
}

func startTCPMappingServer() {
	if strings.EqualFold(getSettingValue("tcp_mapping_enabled", "true"), "false") {
		global.NAV_LOG.Info("navmesh tcp mapping gateway disabled")
		return
	}
	tcpMappingServer = tcpgateway.NewServer(tunnel.DefaultManager)
	services.RegisterTCPMappingReloader(func() {
		if tcpMappingServer == nil {
			return
		}
		if err := tcpMappingServer.Reload(); err != nil {
			global.NAV_LOG.Error("reload navmesh tcp mapping gateway failed", zap.Error(err))
		}
	})
	if err := tcpMappingServer.Start(); err != nil {
		global.NAV_LOG.Error("start navmesh tcp mapping gateway failed", zap.Error(err))
	}
}

func stopTCPMappingServer() {
	if tcpMappingServer != nil {
		tcpMappingServer.Stop()
	}
}

func getSettingValue(key, def string) string {
	var row domains.Setting
	if err := global.NAV_DB.Where("key = ?", key).First(&row).Error; err == nil && strings.TrimSpace(row.Value) != "" {
		return strings.TrimSpace(row.Value)
	}
	return def
}
