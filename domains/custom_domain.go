package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

type CustomDomain struct {
	commonDomains.BaseDataEntity
	Domain      string `json:"domain" gorm:"size:255;uniqueIndex;comment:自定义域名"`
	MappingGuid string `json:"mappingGuid" gorm:"size:64;index;comment:映射ID"`
	VerifyToken string `json:"verifyToken" gorm:"size:128;comment:校验Token"`
	Verified    bool   `json:"verified" gorm:"index;comment:是否已验证"`
	Status      int    `json:"status" gorm:"index;comment:状态"`
}

func (CustomDomain) TableName() string { return "navmesh_custom_domains" }

func (s CustomDomain) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
