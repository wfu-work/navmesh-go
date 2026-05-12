package services

var ServiceGroupApp = new(ServiceGroup)

type ServiceGroup struct {
	AccessPolicyService
	AuditService
	DeviceService
	MappingService
	MaintenanceService
	SessionService
	SettingService
	SSHService
}
