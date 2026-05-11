package services

var ServiceGroupApp = new(ServiceGroup)

type ServiceGroup struct {
	AccessPolicyService
	AuditService
	AuthService
	DeviceService
	MappingService
	MaintenanceService
	SessionService
	SettingService
	SSHService
}
