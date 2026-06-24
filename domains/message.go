package domains

import commonDomains "github.com/wfu-work/nav-common-go-lib/domains"

type MessageEmailConfig struct {
	commonDomains.BaseDataEntity
	Name       string `json:"name" gorm:"size:128;comment:配置名称"`
	Host       string `json:"host" gorm:"size:255;comment:SMTP服务器"`
	Port       int    `json:"port" gorm:"comment:SMTP端口"`
	Username   string `json:"username" gorm:"size:255;comment:账号"`
	Password   string `json:"-" gorm:"size:512;comment:密码或授权码"`
	FromEmail  string `json:"fromEmail" gorm:"size:255;comment:发件邮箱"`
	FromName   string `json:"fromName" gorm:"size:128;comment:发件名称"`
	Encryption string `json:"encryption" gorm:"size:32;comment:加密方式"`
	IsDefault  bool   `json:"isDefault" gorm:"index;comment:默认配置"`
	Remark     string `json:"remark" gorm:"size:512;comment:备注"`
	Status     int    `json:"status" gorm:"index;comment:状态"`
}

func (MessageEmailConfig) TableName() string { return "navmesh_message_email_configs" }

func (s MessageEmailConfig) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}

type MessageTemplate struct {
	commonDomains.BaseDataEntity
	Code        string `json:"code" gorm:"size:64;uniqueIndex;comment:模板编码"`
	Name        string `json:"name" gorm:"size:128;comment:模板名称"`
	Channel     string `json:"channel" gorm:"size:32;index;comment:发送渠道"`
	Subject     string `json:"subject" gorm:"size:255;comment:邮件主题"`
	Content     string `json:"content" gorm:"type:text;comment:模板内容"`
	Description string `json:"description" gorm:"size:512;comment:说明"`
	Status      int    `json:"status" gorm:"index;comment:状态"`
}

func (MessageTemplate) TableName() string { return "navmesh_message_templates" }

func (s MessageTemplate) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}

type MessageRecipient struct {
	commonDomains.BaseDataEntity
	Name         string `json:"name" gorm:"size:128;comment:姓名"`
	Email        string `json:"email" gorm:"size:255;index;comment:邮箱"`
	Phone        string `json:"phone" gorm:"size:64;comment:手机号"`
	Role         string `json:"role" gorm:"size:128;comment:角色"`
	MessageTypes string `json:"messageTypes" gorm:"size:512;comment:消息类型"`
	DeviceGuids  string `json:"deviceGuids" gorm:"size:2048;comment:通知设备范围，空表示全部设备"`
	Tags         string `json:"tags" gorm:"size:512;comment:标签"`
	Remark       string `json:"remark" gorm:"size:512;comment:备注"`
	Status       int    `json:"status" gorm:"index;comment:状态"`
}

func (MessageRecipient) TableName() string { return "navmesh_message_recipients" }

func (s MessageRecipient) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}

type MessageSendRecord struct {
	commonDomains.BaseDataEntity
	BatchGuid      string `json:"batchGuid" gorm:"size:64;index;comment:发送批次ID"`
	Channel        string `json:"channel" gorm:"size:32;index;comment:发送渠道"`
	TemplateCode   string `json:"templateCode" gorm:"size:64;index;comment:模板编码"`
	TemplateName   string `json:"templateName" gorm:"size:128;comment:模板名称"`
	Subject        string `json:"subject" gorm:"size:255;comment:邮件主题"`
	RecipientGuid  string `json:"recipientGuid" gorm:"size:64;index;comment:接收人ID"`
	RecipientName  string `json:"recipientName" gorm:"size:128;comment:接收人名称"`
	RecipientEmail string `json:"recipientEmail" gorm:"size:255;index;comment:接收邮箱"`
	FromEmail      string `json:"fromEmail" gorm:"size:255;comment:发件邮箱"`
	FromName       string `json:"fromName" gorm:"size:128;comment:发件名称"`
	HTMLContent    string `json:"-" gorm:"type:text;comment:邮件HTML内容"`
	SendStatus     string `json:"sendStatus" gorm:"size:32;index;comment:发送状态"`
	ReceiveStatus  string `json:"receiveStatus" gorm:"size:32;index;comment:接收状态"`
	RetryCount     int    `json:"retryCount" gorm:"comment:重试次数"`
	MaxRetries     int    `json:"maxRetries" gorm:"comment:最大重试次数"`
	ErrorMessage   string `json:"errorMessage" gorm:"type:text;comment:错误信息"`
	LastSendTime   int64  `json:"lastSendTime" gorm:"index;comment:最后发送时间"`
	NextRetryTime  int64  `json:"nextRetryTime" gorm:"index;comment:下次重试时间"`
	SuccessTime    int64  `json:"successTime" gorm:"index;comment:成功时间"`
}

func (MessageSendRecord) TableName() string { return "navmesh_message_send_records" }

func (s MessageSendRecord) GetBaseData() commonDomains.BaseDataEntity {
	return s.BaseDataEntity
}
