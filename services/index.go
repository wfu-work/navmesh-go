package services

var ServiceGroupApp = new(ServiceGroup)

type ServiceGroup struct {
	AccessPolicyService
	AuditService
	DeviceService
	EventService
	GroupService
	MappingService
	MaintenanceService
	SessionService
	SettingService
	SSHService
}
