package services

import (
	"errors"
	"sort"
	"strings"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	commonServices "github.com/wfu-work/nav-common-go-lib/services"
	"gorm.io/gorm"
)

type MessageService struct {
	EmailConfigCrud commonServices.CrudService[domains.MessageEmailConfig]
	TemplateCrud    commonServices.CrudService[domains.MessageTemplate]
	RecipientCrud   commonServices.CrudService[domains.MessageRecipient]
	SendRecordCrud  commonServices.CrudService[domains.MessageSendRecord]
}

func (s MessageService) WithDB(db *gorm.DB) MessageService {
	s.EmailConfigCrud = *s.EmailConfigCrud.WithDB(db)
	s.TemplateCrud = *s.TemplateCrud.WithDB(db)
	s.RecipientCrud = *s.RecipientCrud.WithDB(db)
	s.SendRecordCrud = *s.SendRecordCrud.WithDB(db)
	return s
}

func (s MessageService) DB() *gorm.DB {
	return s.EmailConfigCrud.DB()
}

type SaveMessageEmailConfigRequest struct {
	Guid       string `json:"guid"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	FromEmail  string `json:"fromEmail"`
	FromName   string `json:"fromName"`
	Encryption string `json:"encryption"`
	IsDefault  bool   `json:"isDefault"`
	Remark     string `json:"remark"`
	Status     int    `json:"status"`
}

type SaveMessageTemplateRequest struct {
	Guid        string `json:"guid"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Channel     string `json:"channel"`
	Subject     string `json:"subject"`
	Content     string `json:"content"`
	Description string `json:"description"`
	Status      int    `json:"status"`
}

type SaveMessageRecipientRequest struct {
	Guid         string `json:"guid"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Role         string `json:"role"`
	MessageTypes string `json:"messageTypes"`
	DeviceGuids  string `json:"deviceGuids"`
	Tags         string `json:"tags"`
	Remark       string `json:"remark"`
	Status       int    `json:"status"`
}

func (s MessageService) ListEmailConfigs(params map[string]string) ([]domains.MessageEmailConfig, int64, error) {
	db := s.DB().Model(&domains.MessageEmailConfig{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR host LIKE ? OR username LIKE ? OR from_email LIKE ? OR remark LIKE ?", like, like, like, like, like)
	}
	if statusParam, ok := params["status"]; ok && strings.TrimSpace(statusParam) != "" {
		db = db.Where("status = ?", utils.Str2Int(statusParam))
	}
	return pageMessageQuery[domains.MessageEmailConfig](db, params, "is_default DESC, update_time DESC")
}

func (s MessageService) SaveEmailConfig(req SaveMessageEmailConfigRequest) (*domains.MessageEmailConfig, error) {
	req = normalizeEmailConfigRequest(req)
	if req.Name == "" {
		return nil, errors.New("name required")
	}
	if req.Host == "" {
		return nil, errors.New("host required")
	}
	if req.Port <= 0 {
		return nil, errors.New("port required")
	}
	if req.FromEmail == "" {
		return nil, errors.New("fromEmail required")
	}
	now := domains.NowMilli()
	var row domains.MessageEmailConfig
	err := s.DB().Where("guid = ?", req.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.MessageEmailConfig{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}}
		if req.Guid != "" {
			row.Guid = req.Guid
		}
	}
	row.Name = req.Name
	row.Host = req.Host
	row.Port = req.Port
	row.Username = req.Username
	if req.Password != "" {
		row.Password = req.Password
	}
	row.FromEmail = req.FromEmail
	row.FromName = req.FromName
	row.Encryption = req.Encryption
	row.IsDefault = req.IsDefault
	row.Remark = req.Remark
	row.Status = req.Status
	row.UpdateTime = now
	err = s.DB().Transaction(func(tx *gorm.DB) error {
		if row.IsDefault {
			if err := tx.Model(&domains.MessageEmailConfig{}).Where("guid <> ?", row.Guid).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Save(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s MessageService) SetDefaultEmailConfig(guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	now := domains.NowMilli()
	return s.DB().Transaction(func(tx *gorm.DB) error {
		var row domains.MessageEmailConfig
		if err := tx.Where("guid = ?", guid).First(&row).Error; err != nil {
			return errors.New("email config not found")
		}
		if err := tx.Model(&domains.MessageEmailConfig{}).Where("guid <> ?", guid).Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&domains.MessageEmailConfig{}).Where("guid = ?", guid).Updates(map[string]any{
			"is_default":  true,
			"status":      int(domains.StatusEnabled),
			"update_time": now,
		}).Error
	})
}

func (s MessageService) DisableEmailConfig(guid string) error {
	return s.disable(&domains.MessageEmailConfig{}, guid)
}

func (s MessageService) DeleteEmailConfig(guid string) error {
	return s.delete(&domains.MessageEmailConfig{}, guid, "email config not found")
}

func (s MessageService) ListTemplates(params map[string]string) ([]domains.MessageTemplate, int64, error) {
	db := s.DB().Model(&domains.MessageTemplate{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("code LIKE ? OR name LIKE ? OR subject LIKE ? OR description LIKE ?", like, like, like, like)
	}
	if channel := strings.TrimSpace(params["channel"]); channel != "" {
		db = db.Where("channel = ?", channel)
	}
	if statusParam, ok := params["status"]; ok && strings.TrimSpace(statusParam) != "" {
		db = db.Where("status = ?", utils.Str2Int(statusParam))
	}
	return pageMessageQuery[domains.MessageTemplate](db, params, "update_time DESC")
}

func (s MessageService) GetTemplate(identity string) (*domains.MessageTemplate, error) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return nil, errors.New("template identity required")
	}
	var row domains.MessageTemplate
	if err := s.DB().Where("guid = ? OR code = ?", identity, identity).First(&row).Error; err != nil {
		return nil, errors.New("template not found")
	}
	return &row, nil
}

func (s MessageService) GetEnabledTemplate(code string) (*domains.MessageTemplate, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("template code required")
	}
	var row domains.MessageTemplate
	if err := s.DB().Where("code = ? AND channel = ? AND status = ?", code, "email", int(domains.StatusEnabled)).First(&row).Error; err != nil {
		return nil, errors.New("enabled email template not found")
	}
	return &row, nil
}

func (s MessageService) SaveTemplate(req SaveMessageTemplateRequest) (*domains.MessageTemplate, error) {
	req = normalizeTemplateRequest(req)
	if req.Code == "" {
		return nil, errors.New("code required")
	}
	if req.Subject == "" {
		return nil, errors.New("subject required")
	}
	if req.Content == "" {
		return nil, errors.New("content required")
	}
	if err := s.ensureTemplateCodeAvailable(req.Code, req.Guid); err != nil {
		return nil, err
	}
	now := domains.NowMilli()
	var row domains.MessageTemplate
	query := s.DB()
	if req.Guid != "" {
		query = query.Where("guid = ?", req.Guid)
	} else {
		query = query.Where("code = ?", req.Code)
	}
	err := query.First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.MessageTemplate{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}}
		if req.Guid != "" {
			row.Guid = req.Guid
		}
	}
	row.Code = req.Code
	row.Name = req.Name
	row.Channel = req.Channel
	row.Subject = req.Subject
	row.Content = req.Content
	row.Description = req.Description
	row.Status = req.Status
	row.UpdateTime = now
	return &row, s.DB().Save(&row).Error
}

func (s MessageService) DisableTemplate(guid string) error {
	return s.disable(&domains.MessageTemplate{}, guid)
}

func (s MessageService) DeleteTemplate(guid string) error {
	return s.delete(&domains.MessageTemplate{}, guid, "template not found")
}

func (s MessageService) ListRecipients(params map[string]string) ([]domains.MessageRecipient, int64, error) {
	db := s.DB().Model(&domains.MessageRecipient{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR email LIKE ? OR phone LIKE ? OR role LIKE ? OR message_types LIKE ? OR tags LIKE ? OR remark LIKE ?", like, like, like, like, like, like, like)
	}
	if messageType := normalizeMessageTypes(params["messageType"]); messageType != "" {
		db = db.Where("message_types = ? OR message_types LIKE ? OR message_types LIKE ? OR message_types LIKE ?", messageType, messageType+",%", "%,"+messageType, "%,"+messageType+",%")
	}
	if tag := strings.TrimSpace(params["tag"]); tag != "" {
		db = db.Where("tags LIKE ?", "%"+tag+"%")
	}
	if statusParam, ok := params["status"]; ok && strings.TrimSpace(statusParam) != "" {
		db = db.Where("status = ?", utils.Str2Int(statusParam))
	}
	return pageMessageQuery[domains.MessageRecipient](db, params, "update_time DESC")
}

func (s MessageService) SaveRecipient(req SaveMessageRecipientRequest) (*domains.MessageRecipient, error) {
	req = normalizeRecipientRequest(req)
	if req.Name == "" {
		return nil, errors.New("name required")
	}
	if req.Email == "" {
		return nil, errors.New("email required")
	}
	if req.MessageTypes == "" {
		return nil, errors.New("messageTypes required")
	}
	now := domains.NowMilli()
	var row domains.MessageRecipient
	err := s.DB().Where("guid = ?", req.Guid).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = domains.MessageRecipient{BaseDataEntity: commonDomains.BaseDataEntity{CreateTime: now}}
		if req.Guid != "" {
			row.Guid = req.Guid
		}
	}
	row.Name = req.Name
	row.Email = req.Email
	row.Phone = req.Phone
	row.Role = req.Role
	row.MessageTypes = normalizeMessageTypes(req.MessageTypes)
	row.DeviceGuids = normalizeCSVList(req.DeviceGuids)
	row.Tags = normalizeTags(req.Tags)
	row.Remark = req.Remark
	row.Status = req.Status
	row.UpdateTime = now
	return &row, s.DB().Save(&row).Error
}

func (s MessageService) DisableRecipient(guid string) error {
	return s.disable(&domains.MessageRecipient{}, guid)
}

func (s MessageService) DeleteRecipient(guid string) error {
	return s.delete(&domains.MessageRecipient{}, guid, "recipient not found")
}

func (s MessageService) EnabledRecipients() ([]domains.MessageRecipient, error) {
	var rows []domains.MessageRecipient
	err := s.DB().Where("status = ? AND email <> ?", int(domains.StatusEnabled), "").Order("update_time DESC, id ASC").Find(&rows).Error
	return rows, err
}

func (s MessageService) EnabledRecipientsByMessageType(messageType string) ([]domains.MessageRecipient, error) {
	messageType = strings.TrimSpace(messageType)
	if messageType == "" {
		return nil, errors.New("messageType required")
	}
	rows, err := s.EnabledRecipients()
	if err != nil {
		return nil, err
	}
	filtered := make([]domains.MessageRecipient, 0, len(rows))
	for _, row := range rows {
		if recipientAcceptsMessageType(row.MessageTypes, messageType) {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func (s MessageService) EnabledRecipientsByMessageTypeAndDevice(messageType string, deviceGuid string) ([]domains.MessageRecipient, error) {
	rows, err := s.EnabledRecipientsByMessageType(messageType)
	if err != nil {
		return nil, err
	}
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return rows, nil
	}
	filtered := make([]domains.MessageRecipient, 0, len(rows))
	for _, row := range rows {
		if recipientAcceptsDevice(row.DeviceGuids, deviceGuid) {
			filtered = append(filtered, row)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		allDevicesI := normalizeCSVList(filtered[i].DeviceGuids) == ""
		allDevicesJ := normalizeCSVList(filtered[j].DeviceGuids) == ""
		if allDevicesI != allDevicesJ {
			return allDevicesI
		}
		if filtered[i].UpdateTime != filtered[j].UpdateTime {
			return filtered[i].UpdateTime > filtered[j].UpdateTime
		}
		return filtered[i].Id < filtered[j].Id
	})
	return filtered, nil
}

func (s MessageService) EnabledRecipientsByGuids(guids []string) ([]domains.MessageRecipient, error) {
	normalized := normalizeStringList(guids)
	if len(normalized) == 0 {
		return nil, errors.New("recipientGuids required")
	}
	var rows []domains.MessageRecipient
	err := s.DB().
		Where("status = ? AND email <> ? AND guid IN ?", int(domains.StatusEnabled), "", normalized).
		Order("update_time DESC").
		Find(&rows).Error
	return rows, err
}

func (s MessageService) ListSendRecords(params map[string]string) ([]domains.MessageSendRecord, int64, error) {
	db := s.DB().Model(&domains.MessageSendRecord{})
	if keyword := strings.TrimSpace(utils.FirstNonEmpty(params["keyword"], params["content"])); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where(
			"subject LIKE ? OR template_code LIKE ? OR template_name LIKE ? OR recipient_name LIKE ? OR recipient_email LIKE ? OR error_message LIKE ? OR batch_guid LIKE ?",
			like, like, like, like, like, like, like,
		)
	}
	if sendStatus := strings.TrimSpace(params["sendStatus"]); sendStatus != "" {
		db = db.Where("send_status = ?", sendStatus)
	}
	if receiveStatus := strings.TrimSpace(params["receiveStatus"]); receiveStatus != "" {
		db = db.Where("receive_status = ?", receiveStatus)
	}
	if templateCode := strings.TrimSpace(params["templateCode"]); templateCode != "" {
		db = db.Where("template_code = ?", templateCode)
	}
	if batchGuid := strings.TrimSpace(params["batchGuid"]); batchGuid != "" {
		db = db.Where("batch_guid = ?", batchGuid)
	}
	if recipient := strings.TrimSpace(params["recipient"]); recipient != "" {
		like := "%" + recipient + "%"
		db = db.Where("recipient_name LIKE ? OR recipient_email LIKE ?", like, like)
	}
	return pageMessageQuery[domains.MessageSendRecord](db, params, "update_time DESC")
}

func (s MessageService) GetSendRecord(guid string) (*domains.MessageSendRecord, error) {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return nil, errors.New("guid required")
	}
	var row domains.MessageSendRecord
	if err := s.DB().Where("guid = ?", guid).First(&row).Error; err != nil {
		return nil, errors.New("send record not found")
	}
	return &row, nil
}

func (s MessageService) RetrySendRecord(guid string) (*domains.MessageSendRecord, error) {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return nil, errors.New("guid required")
	}
	var record domains.MessageSendRecord
	if err := s.DB().Where("guid = ?", guid).First(&record).Error; err != nil {
		return nil, errors.New("send record not found")
	}
	if record.SendStatus == MessageSendStatusSuccess {
		return nil, errors.New("send record already successful")
	}
	maxRetries := record.MaxRetries
	if maxRetries <= 0 {
		maxRetries = MaxMessageSendRetries
	}
	if record.RetryCount >= maxRetries {
		return nil, errors.New("send record retry limit exceeded")
	}
	return ServiceGroupApp.EmailService.ResendRecord(&record)
}

func (s MessageService) DeleteSendRecord(guid string) error {
	return s.delete(&domains.MessageSendRecord{}, guid, "send record not found")
}

func (s MessageService) CreateSendRecord(record domains.MessageSendRecord) (*domains.MessageSendRecord, error) {
	if strings.TrimSpace(record.TemplateCode) == "" {
		return nil, errors.New("templateCode required")
	}
	if strings.TrimSpace(record.RecipientEmail) == "" {
		return nil, errors.New("recipientEmail required")
	}
	now := domains.NowMilli()
	if record.MaxRetries <= 0 {
		record.MaxRetries = MaxMessageSendRetries
	}
	if record.SendStatus == "" {
		record.SendStatus = MessageSendStatusPending
	}
	if record.ReceiveStatus == "" {
		record.ReceiveStatus = MessageReceiveStatusWaiting
	}
	record.BatchGuid = strings.TrimSpace(record.BatchGuid)
	record.Channel = utils.FirstNonEmpty(record.Channel, "email")
	record.TemplateCode = strings.TrimSpace(record.TemplateCode)
	record.TemplateName = strings.TrimSpace(record.TemplateName)
	record.Subject = strings.TrimSpace(record.Subject)
	record.RecipientGuid = strings.TrimSpace(record.RecipientGuid)
	record.RecipientName = strings.TrimSpace(record.RecipientName)
	record.RecipientEmail = strings.TrimSpace(record.RecipientEmail)
	record.FromEmail = strings.TrimSpace(record.FromEmail)
	record.FromName = strings.TrimSpace(record.FromName)
	record.HTMLContent = strings.TrimSpace(record.HTMLContent)
	record.ErrorMessage = strings.TrimSpace(record.ErrorMessage)
	record.CreateTime = now
	record.UpdateTime = now
	if err := s.DB().Create(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s MessageService) UpdateSendRecordStatus(guid string, updates map[string]any) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	if len(updates) == 0 {
		return nil
	}
	updates["update_time"] = domains.NowMilli()
	return s.DB().Model(&domains.MessageSendRecord{}).Where("guid = ?", guid).Updates(updates).Error
}

func (s MessageService) SeedDefaults() {
	db := s.DB()
	if db == nil {
		return
	}
	now := domains.NowMilli()
	for _, item := range defaultMessageTemplates(now) {
		var count int64
		if err := db.Model(&domains.MessageTemplate{}).Where("code = ?", item.Code).Count(&count).Error; err != nil || count > 0 {
			continue
		}
		_ = db.Create(&item).Error
	}
	_ = db.Model(&domains.MessageRecipient{}).
		Where("message_types = ? OR message_types IS NULL", "").
		Update("message_types", TemplateCodeReleasePublished).Error
}

func defaultMessageTemplates(now int64) []domains.MessageTemplate {
	return []domains.MessageTemplate{
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: TemplateCodeReleasePublished, CreateTime: now, UpdateTime: now},
			Code:           TemplateCodeReleasePublished,
			Name:           "版本发布通知",
			Channel:        "email",
			Subject:        "{{releaseType}} {{version}} 已发布",
			Content: `<p style="margin:0 0 16px;">有新的 {{releaseType}} 版本发布，请根据现场设备情况安排升级。</p>
<p style="margin:0 0 10px;"><strong>版本号：</strong>{{version}}</p>
<p style="margin:0 0 10px;"><strong>适用范围：</strong>{{deviceScope}}</p>
<p style="margin:0 0 16px;"><strong>目标平台：</strong>{{platform}}</p>
<p style="margin:0 0 10px;color:#475569;"><strong>更新说明：</strong>{{changeLog}}</p>
<p style="margin:0;"><strong>下载地址：</strong><a href="{{downloadUrl}}" style="color:#2563eb;text-decoration:none;font-weight:700;">{{downloadUrl}}</a></p>`,
			Description: "版本管理发布新版本后自动发送，模板编码不要修改。",
			Status:      int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: TemplateCodeDeviceOfflineNotice, CreateTime: now, UpdateTime: now},
			Code:           TemplateCodeDeviceOfflineNotice,
			Name:           "设备离线通知",
			Channel:        "email",
			Subject:        "{{deviceAlias}} 已离线",
			Content: `<p style="margin:0 0 16px;">设备 {{deviceAlias}} 已超过心跳阈值未上报，请尽快检查现场网络、电源和客户端进程。</p>
<p style="margin:0 0 10px;"><strong>设备编号：</strong>{{deviceSncode}}</p>
<p style="margin:0 0 10px;"><strong>设备类型：</strong>{{deviceType}}</p>
<p style="margin:0 0 16px;"><strong>最后在线：</strong>{{lastSeenTime}}</p>
<p style="margin:0 0 10px;color:#475569;"><strong>事件时间：</strong>{{time}}</p>
<p style="margin:0;color:#475569;"><strong>事件说明：</strong>{{eventMessage}}</p>`,
			Description: "设备心跳超时离线后发送，可用于通知运维人员排查现场网络、电源或客户端状态。",
			Status:      int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: TemplateCodeDiskUsageHighNotice, CreateTime: now, UpdateTime: now},
			Code:           TemplateCodeDiskUsageHighNotice,
			Name:           "磁盘阈值通知",
			Channel:        "email",
			Subject:        "{{deviceAlias}} 磁盘使用率达到 {{diskUsedPct}}%",
			Content: `<p style="margin:0 0 16px;">设备 {{deviceAlias}} 的磁盘使用率已达到 {{diskUsedPct}}%，超过 {{diskThreshold}}% 告警阈值，请及时清理日志、数据文件或扩容磁盘。</p>
<p style="margin:0 0 10px;"><strong>设备编号：</strong>{{deviceSncode}}</p>
<p style="margin:0 0 10px;"><strong>设备类型：</strong>{{deviceType}}</p>
<p style="margin:0 0 16px;"><strong>磁盘使用：</strong>已用 {{diskUsed}} / 总量 {{diskTotal}}，剩余 {{diskFree}}</p>
<p style="margin:0 0 10px;color:#475569;"><strong>事件时间：</strong>{{time}}</p>
<p style="margin:0;color:#475569;"><strong>事件说明：</strong>{{eventMessage}}</p>`,
			Description: "设备心跳上报磁盘使用率达到告警阈值后发送，同一设备每天最多触发一次。",
			Status:      int(domains.StatusEnabled),
		},
	}
}

func (s MessageService) disable(model any, guid string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	return s.DB().Model(model).Where("guid = ?", guid).Updates(map[string]any{
		"status":      int(domains.StatusDisabled),
		"update_time": domains.NowMilli(),
	}).Error
}

func (s MessageService) delete(model any, guid string, notFound string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return errors.New("guid required")
	}
	var count int64
	if err := s.DB().Model(model).Where("guid = ?", guid).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New(notFound)
	}
	return s.DB().Unscoped().Where("guid = ?", guid).Delete(model).Error
}

func (s MessageService) ensureTemplateCodeAvailable(code string, currentGuid string) error {
	var existing domains.MessageTemplate
	err := s.DB().Where("code = ?", code).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if currentGuid != "" && existing.Guid == currentGuid {
		return nil
	}
	return errors.New("code already exists")
}

func normalizeEmailConfigRequest(req SaveMessageEmailConfigRequest) SaveMessageEmailConfigRequest {
	req.Guid = strings.TrimSpace(req.Guid)
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	req.FromEmail = strings.TrimSpace(req.FromEmail)
	req.FromName = strings.TrimSpace(req.FromName)
	req.Encryption = strings.ToLower(strings.TrimSpace(req.Encryption))
	req.Remark = strings.TrimSpace(req.Remark)
	if req.Port <= 0 {
		req.Port = 465
	}
	if req.Encryption == "" {
		req.Encryption = "ssl"
	}
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	return req
}

func normalizeTemplateRequest(req SaveMessageTemplateRequest) SaveMessageTemplateRequest {
	req.Guid = strings.TrimSpace(req.Guid)
	req.Code = strings.TrimSpace(req.Code)
	req.Name = messageTemplateName(req.Code)
	req.Channel = strings.ToLower(strings.TrimSpace(req.Channel))
	req.Subject = strings.TrimSpace(req.Subject)
	req.Content = strings.TrimSpace(req.Content)
	req.Description = strings.TrimSpace(req.Description)
	if req.Channel == "" {
		req.Channel = "email"
	}
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	return req
}

func messageTemplateName(code string) string {
	switch strings.TrimSpace(code) {
	case TemplateCodeReleasePublished:
		return "版本发布通知"
	case TemplateCodeDeviceOfflineNotice:
		return "设备离线通知"
	case TemplateCodeDiskUsageHighNotice:
		return "磁盘阈值通知"
	default:
		return strings.TrimSpace(code)
	}
}

func normalizeRecipientRequest(req SaveMessageRecipientRequest) SaveMessageRecipientRequest {
	req.Guid = strings.TrimSpace(req.Guid)
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Role = strings.TrimSpace(req.Role)
	req.MessageTypes = normalizeMessageTypes(req.MessageTypes)
	req.DeviceGuids = normalizeCSVList(req.DeviceGuids)
	req.Tags = strings.TrimSpace(req.Tags)
	req.Remark = strings.TrimSpace(req.Remark)
	if req.Status == 0 {
		req.Status = int(domains.StatusEnabled)
	}
	return req
}

func normalizeMessageTypes(messageTypes string) string {
	parts := strings.Split(messageTypes, ",")
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		messageType := strings.ToLower(strings.TrimSpace(part))
		if messageType == "" || seen[messageType] {
			continue
		}
		seen[messageType] = true
		out = append(out, messageType)
	}
	return strings.Join(out, ",")
}

func normalizeStringList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func normalizeCSVList(value string) string {
	return strings.Join(normalizeStringList(strings.Split(value, ",")), ",")
}

func recipientAcceptsMessageType(messageTypes string, messageType string) bool {
	messageType = strings.ToLower(strings.TrimSpace(messageType))
	if messageType == "" {
		return false
	}
	for _, item := range strings.Split(messageTypes, ",") {
		if strings.ToLower(strings.TrimSpace(item)) == messageType {
			return true
		}
	}
	return false
}

func recipientAcceptsDevice(deviceGuids string, deviceGuid string) bool {
	deviceGuid = strings.TrimSpace(deviceGuid)
	if deviceGuid == "" {
		return true
	}
	deviceGuids = normalizeCSVList(deviceGuids)
	if deviceGuids == "" {
		return true
	}
	for _, item := range strings.Split(deviceGuids, ",") {
		if strings.TrimSpace(item) == deviceGuid {
			return true
		}
	}
	return false
}

func pageMessageQuery[T any](db *gorm.DB, params map[string]string, order string) ([]T, int64, error) {
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := ParsePage(params, DefaultMaxPageSize)
	var items []T
	err := db.Order(order).Limit(page.Size).Offset((page.Page - 1) * page.Size).Find(&items).Error
	return items, total, err
}
