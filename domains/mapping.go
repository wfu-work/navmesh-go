package domains

type SSHAlias struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	DeviceGuid   string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	Alias        string `json:"alias" gorm:"size:128;uniqueIndex;comment:设备别名"`
	Domain       string `json:"domain" gorm:"size:255;uniqueIndex;comment:SSH 域名"`
	EntrypointIP string `json:"entrypointIp" gorm:"size:64;index;comment:入口IP"`
	Status       int    `json:"status" gorm:"index;comment:状态"`
	CreateTime   int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime   int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (SSHAlias) TableName() string { return "navmesh_ssh_aliases" }

type SSHEntrypoint struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	IP         string `json:"ip" gorm:"size:64;uniqueIndex;comment:入口IPv4或IPv6"`
	DeviceGuid string `json:"deviceGuid" gorm:"size:64;index;comment:绑定设备ID"`
	Status     int    `json:"status" gorm:"index;comment:状态"`
	CreateTime int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (SSHEntrypoint) TableName() string { return "navmesh_ssh_entrypoints" }

type PortMapping struct {
	ID             uint   `json:"id" gorm:"primaryKey"`
	Guid           string `json:"guid" gorm:"size:64;uniqueIndex;comment:映射ID"`
	DeviceGuid     string `json:"deviceGuid" gorm:"size:64;index;comment:设备ID"`
	Name           string `json:"name" gorm:"size:128;comment:名称"`
	PublicHost     string `json:"publicHost" gorm:"size:255;uniqueIndex;comment:外部域名"`
	TargetHost     string `json:"targetHost" gorm:"size:64;comment:目标主机"`
	TargetPort     int    `json:"targetPort" gorm:"comment:目标端口"`
	Protocol       string `json:"protocol" gorm:"size:32;comment:协议"`
	IsCustomDomain bool   `json:"isCustomDomain" gorm:"comment:是否自定义域名"`
	Status         int    `json:"status" gorm:"index;comment:状态"`
	CreateTime     int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime     int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (PortMapping) TableName() string { return "navmesh_port_mappings" }

type CustomDomain struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Domain      string `json:"domain" gorm:"size:255;uniqueIndex;comment:自定义域名"`
	MappingGuid string `json:"mappingGuid" gorm:"size:64;index;comment:映射ID"`
	VerifyToken string `json:"verifyToken" gorm:"size:128;comment:校验Token"`
	Verified    bool   `json:"verified" gorm:"index;comment:是否已验证"`
	Status      int    `json:"status" gorm:"index;comment:状态"`
	CreateTime  int64  `json:"createTime" gorm:"comment:创建时间"`
	UpdateTime  int64  `json:"updateTime" gorm:"comment:更新时间"`
}

func (CustomDomain) TableName() string { return "navmesh_custom_domains" }
