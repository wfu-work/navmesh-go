package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

type AccessPolicy struct {
	commonDomains.BaseDataEntity
	Name      string `json:"name" gorm:"size:128;comment:名称"`
	Scope     string `json:"scope" gorm:"size:32;index;comment:策略范围"`
	TargetID  string `json:"targetId" gorm:"size:128;index;comment:目标ID"`
	AllowSSH  bool   `json:"allowSsh" gorm:"comment:允许SSH"`
	AllowHTTP bool   `json:"allowHttp" gorm:"comment:允许HTTP"`
	Status    int    `json:"status" gorm:"index;comment:状态"`
}

func (AccessPolicy) TableName() string { return "navmesh_access_policies" }

func (s AccessPolicy) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
