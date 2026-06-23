package services

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"navmesh-go/domains"
	"navmesh-go/utils"

	"github.com/google/uuid"
	"github.com/wfu-work/nav-common-go-lib/global"
	"go.uber.org/zap"
)

const (
	TemplateCodeReleasePublished    = "release_published_notice"
	TemplateCodeDeviceOfflineNotice = "device_offline_notice"
	TemplateCodeDiskUsageHighNotice = "disk_usage_high_notice"
	debugEmailSubjectPrefix         = "【调试】"
	debugEmailContentPrefix         = `<p style="margin:0 0 18px;padding:12px 14px;background:#f8fafc;border-left:3px solid #94a3b8;color:#475569;">这是一封调试邮件，用于验证消息模板、通知人员和 SMTP 配置。</p>`
	emailDialTimeout                = 10 * time.Second
	MaxMessageSendRetries           = 3
	MessageSendStatusPending        = "pending"
	MessageSendStatusSuccess        = "success"
	MessageSendStatusFailed         = "failed"
	MessageReceiveStatusWaiting     = "waiting"
	MessageReceiveStatusAccepted    = "accepted"
	MessageReceiveStatusFailed      = "failed"
)

type EmailService struct{}

type EmailTemplateInput struct {
	Code      string
	Title     string
	Variables map[string]string
}

type DebugEmailTemplateInput struct {
	TemplateCode   string            `json:"templateCode"`
	RecipientGuids []string          `json:"recipientGuids"`
	Variables      map[string]string `json:"variables"`
}

type EmailTemplatePreviewInput struct {
	Code    string `json:"code"`
	Subject string `json:"subject"`
	Content string `json:"content"`
}

type EmailTemplatePreviewResult struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type EmailSendResult struct {
	Recipients int    `json:"recipients"`
	Subject    string `json:"subject"`
	Successes  int    `json:"successes"`
	Failures   int    `json:"failures"`
}

type EmailHTMLSendInput struct {
	Recipients   []string
	Subject      string
	HTML         string
	TemplateCode string
	TemplateName string
}

type EmailRecipientInput struct {
	Guid  string
	Name  string
	Email string
}

var sendTemplateEmail = func(s EmailService, input EmailTemplateInput) (*EmailSendResult, error) {
	return s.SendTemplate(input)
}

func (s EmailService) SendHTML(input EmailHTMLSendInput) (*EmailSendResult, error) {
	subject := strings.TrimSpace(input.Subject)
	if subject == "" {
		return nil, errors.New("email subject required")
	}
	htmlBody := strings.TrimSpace(input.HTML)
	if htmlBody == "" {
		return nil, errors.New("email html required")
	}
	recipients := emailRecipientsFromAddresses(input.Recipients)
	if len(recipients) == 0 {
		return nil, errors.New("email recipients not configured")
	}
	return s.sendRenderedHTML(emailRenderedInput{
		TemplateCode: utils.FirstNonEmpty(input.TemplateCode, "custom_html"),
		TemplateName: input.TemplateName,
		Subject:      subject,
		HTML:         htmlBody,
		Recipients:   recipients,
	})
}

func (s EmailService) SendTemplate(input EmailTemplateInput) (*EmailSendResult, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return nil, errors.New("template code required")
	}
	tpl, err := ServiceGroupApp.MessageService.GetEnabledTemplate(code)
	if err != nil {
		return nil, err
	}
	recipients, err := ServiceGroupApp.MessageService.EnabledRecipientsByMessageType(code)
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return nil, errors.New("email recipients not configured")
	}
	return s.sendTemplateToRecipients(emailTemplateSendInput{
		Template:   tpl,
		Title:      input.Title,
		Variables:  input.Variables,
		Recipients: recipients,
	})
}

func (s EmailService) DebugSendTemplate(input DebugEmailTemplateInput) (*EmailSendResult, error) {
	code := strings.TrimSpace(input.TemplateCode)
	if code == "" {
		return nil, errors.New("templateCode required")
	}
	tpl, err := ServiceGroupApp.MessageService.GetEnabledTemplate(code)
	if err != nil {
		return nil, err
	}
	recipients, err := ServiceGroupApp.MessageService.EnabledRecipientsByGuids(input.RecipientGuids)
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return nil, errors.New("email recipients not configured")
	}
	variables := debugEmailTemplateVariables(code)
	for key, value := range input.Variables {
		variables[key] = value
	}
	return s.sendTemplateToRecipients(emailTemplateSendInput{
		Template:      tpl,
		Title:         "邮件调试发送",
		Variables:     variables,
		SubjectPrefix: debugEmailSubjectPrefix,
		ContentPrefix: debugEmailContentPrefix,
		Recipients:    recipients,
	})
}

func (s EmailService) PreviewTemplate(input EmailTemplatePreviewInput) (*EmailTemplatePreviewResult, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return nil, errors.New("template code required")
	}
	subjectTemplate := strings.TrimSpace(input.Subject)
	if subjectTemplate == "" {
		return nil, errors.New("email subject required")
	}
	contentTemplate := strings.TrimSpace(input.Content)
	if contentTemplate == "" {
		return nil, errors.New("email content required")
	}
	variables := debugEmailTemplateVariables(code)
	subject := strings.TrimSpace(utils.RenderTemplateText(subjectTemplate, variables))
	if subject == "" {
		subject = messageTemplateName(code)
	}
	if subject == "" {
		subject = "邮件预览"
	}
	if !strings.HasPrefix(subject, debugEmailSubjectPrefix) {
		subject = debugEmailSubjectPrefix + subject
	}
	body := debugEmailContentPrefix + utils.RenderTemplateText(contentTemplate, variables)
	htmlBody := utils.DefaultEmailHTML(utils.EmailHTMLInput{
		Title:     utils.FirstNonEmpty(messageTemplateName(code), subject, "邮件预览"),
		Subject:   subject,
		Content:   body,
		Variables: variables,
	})
	return &EmailTemplatePreviewResult{Subject: subject, HTML: htmlBody}, nil
}

func (s EmailService) ResendRecord(record *domains.MessageSendRecord) (*domains.MessageSendRecord, error) {
	if record == nil {
		return nil, errors.New("send record required")
	}
	if strings.TrimSpace(record.Guid) == "" {
		return nil, errors.New("send record guid required")
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
	email := strings.TrimSpace(record.RecipientEmail)
	if email == "" {
		return nil, errors.New("recipient email required")
	}
	subject := strings.TrimSpace(record.Subject)
	if subject == "" {
		return nil, errors.New("email subject required")
	}
	htmlBody := strings.TrimSpace(record.HTMLContent)
	if htmlBody == "" {
		return nil, errors.New("email html required")
	}
	config, configErr := s.defaultEmailConfig()
	retryCount := record.RetryCount + 1
	now := domains.NowMilli()
	updates := map[string]any{
		"send_status":     MessageSendStatusPending,
		"receive_status":  MessageReceiveStatusWaiting,
		"retry_count":     retryCount,
		"error_message":   "",
		"last_send_time":  now,
		"next_retry_time": 0,
	}
	if config != nil {
		updates["from_email"] = config.FromEmail
		updates["from_name"] = config.FromName
	}
	if err := ServiceGroupApp.MessageService.UpdateSendRecordStatus(record.Guid, updates); err != nil {
		return nil, err
	}
	if configErr != nil {
		_ = markEmailRecordFailed(record.Guid, configErr)
		updated, _ := ServiceGroupApp.MessageService.GetSendRecord(record.Guid)
		return updated, configErr
	}
	if err := s.sendHTML(*config, []string{email}, subject, htmlBody); err != nil {
		_ = markEmailRecordFailed(record.Guid, err)
		updated, _ := ServiceGroupApp.MessageService.GetSendRecord(record.Guid)
		return updated, err
	}
	if err := markEmailRecordSuccess(record.Guid); err != nil {
		return nil, err
	}
	return ServiceGroupApp.MessageService.GetSendRecord(record.Guid)
}

type emailRenderedInput struct {
	TemplateCode string
	TemplateName string
	Subject      string
	HTML         string
	Recipients   []EmailRecipientInput
}

type emailTemplateSendInput struct {
	Template      *domains.MessageTemplate
	Title         string
	Variables     map[string]string
	SubjectPrefix string
	ContentPrefix string
	Recipients    []domains.MessageRecipient
}

func (s EmailService) sendTemplateToRecipients(input emailTemplateSendInput) (*EmailSendResult, error) {
	if input.Template == nil {
		return nil, errors.New("email template required")
	}
	if len(input.Recipients) == 0 {
		return nil, errors.New("email recipients not configured")
	}
	tpl := input.Template
	variables := utils.NormalizeTemplateVariables(input.Variables)
	subject := utils.RenderTemplateText(tpl.Subject, variables)
	if strings.TrimSpace(subject) == "" {
		subject = strings.TrimSpace(input.Title)
	}
	if strings.TrimSpace(subject) == "" {
		subject = tpl.Name
	}
	if prefix := strings.TrimSpace(input.SubjectPrefix); prefix != "" && !strings.HasPrefix(subject, prefix) {
		subject = prefix + subject
	}
	body := strings.TrimSpace(input.ContentPrefix) + utils.RenderTemplateText(tpl.Content, variables)
	htmlBody := utils.DefaultEmailHTML(utils.EmailHTMLInput{
		Title:     utils.FirstNonEmpty(input.Title, subject, tpl.Name),
		Subject:   subject,
		Content:   body,
		Variables: variables,
	})
	emailRecipients := make([]EmailRecipientInput, 0, len(input.Recipients))
	for _, item := range input.Recipients {
		emailRecipients = append(emailRecipients, EmailRecipientInput{Guid: item.Guid, Name: item.Name, Email: item.Email})
	}
	return s.sendRenderedHTML(emailRenderedInput{
		TemplateCode: tpl.Code,
		TemplateName: tpl.Name,
		Subject:      subject,
		HTML:         htmlBody,
		Recipients:   emailRecipients,
	})
}

func (s EmailService) sendRenderedHTML(input emailRenderedInput) (*EmailSendResult, error) {
	recipients := normalizeEmailRecipients(input.Recipients)
	if len(recipients) == 0 {
		return nil, errors.New("email recipients not configured")
	}
	subject := strings.TrimSpace(input.Subject)
	if subject == "" {
		return nil, errors.New("email subject required")
	}
	htmlBody := strings.TrimSpace(input.HTML)
	if htmlBody == "" {
		return nil, errors.New("email html required")
	}
	config, configErr := s.defaultEmailConfig()
	batchGuid := uuid.NewString()
	result := &EmailSendResult{Recipients: len(recipients), Subject: subject}
	for _, recipient := range recipients {
		record := domains.MessageSendRecord{
			BatchGuid:      batchGuid,
			Channel:        "email",
			TemplateCode:   strings.TrimSpace(input.TemplateCode),
			TemplateName:   strings.TrimSpace(input.TemplateName),
			Subject:        subject,
			RecipientGuid:  strings.TrimSpace(recipient.Guid),
			RecipientName:  strings.TrimSpace(recipient.Name),
			RecipientEmail: strings.TrimSpace(recipient.Email),
			HTMLContent:    htmlBody,
			SendStatus:     MessageSendStatusPending,
			ReceiveStatus:  MessageReceiveStatusWaiting,
			MaxRetries:     MaxMessageSendRetries,
			LastSendTime:   domains.NowMilli(),
		}
		if config != nil {
			record.FromEmail = config.FromEmail
			record.FromName = config.FromName
		}
		created, err := ServiceGroupApp.MessageService.CreateSendRecord(record)
		if err != nil {
			result.Failures++
			continue
		}
		if configErr != nil {
			result.Failures++
			_ = markEmailRecordFailed(created.Guid, configErr)
			continue
		}
		if err := s.sendHTML(*config, []string{recipient.Email}, subject, htmlBody); err != nil {
			result.Failures++
			_ = markEmailRecordFailed(created.Guid, err)
			continue
		}
		result.Successes++
		_ = markEmailRecordSuccess(created.Guid)
	}
	if result.Failures > 0 {
		return result, fmt.Errorf("email send failed: %d/%d", result.Failures, result.Recipients)
	}
	return result, nil
}

func markEmailRecordSuccess(guid string) error {
	now := domains.NowMilli()
	return ServiceGroupApp.MessageService.UpdateSendRecordStatus(guid, map[string]any{
		"send_status":     MessageSendStatusSuccess,
		"receive_status":  MessageReceiveStatusAccepted,
		"error_message":   "",
		"next_retry_time": 0,
		"success_time":    now,
		"last_send_time":  now,
	})
}

func markEmailRecordFailed(guid string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return ServiceGroupApp.MessageService.UpdateSendRecordStatus(guid, map[string]any{
		"send_status":    MessageSendStatusFailed,
		"receive_status": MessageReceiveStatusFailed,
		"error_message":  message,
		"last_send_time": domains.NowMilli(),
	})
}

func (s EmailService) NotifyReleasePublished(release *domains.Release, downloadURL string) {
	if release == nil {
		return
	}
	title := ReleasePublishedTitle(release)
	variables := ReleasePublishedTemplateVariables(release, downloadURL)
	result, err := sendTemplateEmail(s, EmailTemplateInput{
		Code:      TemplateCodeReleasePublished,
		Title:     title,
		Variables: variables,
	})
	if err != nil {
		if global.NAV_LOG != nil {
			fields := []zap.Field{zap.String("releaseGuid", release.Guid), zap.Error(err)}
			if result != nil {
				fields = append(fields, zap.Int("successes", result.Successes), zap.Int("failures", result.Failures))
			}
			global.NAV_LOG.Warn("send release published email failed", fields...)
		}
		return
	}
	if global.NAV_LOG != nil {
		global.NAV_LOG.Info("send release published email success", zap.String("releaseGuid", release.Guid), zap.Int("recipients", result.Recipients), zap.Int("successes", result.Successes))
	}
}

func (s EmailService) NotifyDeviceOffline(device *domains.Device, message string, now int64) {
	if device == nil {
		return
	}
	variables := DeviceOfflineTemplateVariables(device, message, now)
	result, err := sendTemplateEmail(s, EmailTemplateInput{
		Code:      TemplateCodeDeviceOfflineNotice,
		Title:     "设备离线通知",
		Variables: variables,
	})
	if err != nil {
		if global.NAV_LOG != nil {
			fields := []zap.Field{zap.String("deviceGuid", device.Guid), zap.Error(err)}
			if result != nil {
				fields = append(fields, zap.Int("successes", result.Successes), zap.Int("failures", result.Failures))
			}
			global.NAV_LOG.Warn("send device offline email failed", fields...)
		}
		return
	}
	if global.NAV_LOG != nil {
		global.NAV_LOG.Info("send device offline email success", zap.String("deviceGuid", device.Guid), zap.Int("recipients", result.Recipients), zap.Int("successes", result.Successes))
	}
}

func (s EmailService) NotifyDiskUsageHigh(device *domains.Device, req HeartbeatRequest, message string, now int64) {
	if device == nil {
		return
	}
	variables := DiskUsageHighTemplateVariables(device, req, message, now)
	result, err := sendTemplateEmail(s, EmailTemplateInput{
		Code:      TemplateCodeDiskUsageHighNotice,
		Title:     "磁盘阈值通知",
		Variables: variables,
	})
	if err != nil {
		if global.NAV_LOG != nil {
			fields := []zap.Field{zap.String("deviceGuid", device.Guid), zap.Error(err)}
			if result != nil {
				fields = append(fields, zap.Int("successes", result.Successes), zap.Int("failures", result.Failures))
			}
			global.NAV_LOG.Warn("send disk usage high email failed", fields...)
		}
		return
	}
	if global.NAV_LOG != nil {
		global.NAV_LOG.Info("send disk usage high email success", zap.String("deviceGuid", device.Guid), zap.Int("recipients", result.Recipients), zap.Int("successes", result.Successes))
	}
}

func (s EmailService) defaultEmailConfig() (*domains.MessageEmailConfig, error) {
	db := ServiceGroupApp.MessageService.DB()
	if db == nil {
		return nil, errors.New("database not initialized")
	}
	var row domains.MessageEmailConfig
	err := db.Where("status = ?", int(domains.StatusEnabled)).
		Order("is_default DESC, update_time DESC").
		First(&row).Error
	if err != nil {
		return nil, errors.New("email config not configured")
	}
	if strings.TrimSpace(row.Host) == "" || row.Port <= 0 || strings.TrimSpace(row.FromEmail) == "" {
		return nil, errors.New("email config incomplete")
	}
	return &row, nil
}

func (s EmailService) sendHTML(config domains.MessageEmailConfig, recipients []string, subject string, htmlBody string) error {
	host := strings.TrimSpace(config.Host)
	addr := net.JoinHostPort(host, fmt.Sprint(config.Port))
	headers := map[string]string{
		"From":         formatEmailAddress(config.FromName, config.FromEmail),
		"To":           strings.Join(recipients, ", "),
		"Subject":      mime.QEncoding.Encode("UTF-8", subject),
		"MIME-Version": "1.0",
		"Content-Type": `text/html; charset="UTF-8"`,
	}
	var message strings.Builder
	for _, key := range []string{"From", "To", "Subject", "MIME-Version", "Content-Type"} {
		message.WriteString(key)
		message.WriteString(": ")
		message.WriteString(headers[key])
		message.WriteString("\r\n")
	}
	message.WriteString("\r\n")
	message.WriteString(htmlBody)
	auth := smtp.Auth(nil)
	username := strings.TrimSpace(config.Username)
	password := strings.TrimSpace(config.Password)
	if username != "" || password != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}
	switch strings.ToLower(strings.TrimSpace(config.Encryption)) {
	case "ssl", "tls":
		return sendMailTLS(addr, host, auth, config.FromEmail, recipients, []byte(message.String()))
	case "starttls":
		return sendMailStartTLS(addr, host, auth, config.FromEmail, recipients, []byte(message.String()))
	default:
		return sendMailPlain(addr, host, auth, config.FromEmail, recipients, []byte(message.String()))
	}
}

func sendMailTLS(addr string, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	dialer := &net.Dialer{Timeout: emailDialTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	return sendSMTP(client, auth, from, to, msg)
}

func sendMailStartTLS(addr string, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	client, err := dialSMTP(addr, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	return sendSMTP(client, auth, from, to, msg)
}

func sendMailPlain(addr string, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	client, err := dialSMTP(addr, host)
	if err != nil {
		return err
	}
	defer client.Close()
	return sendSMTP(client, auth, from, to, msg)
}

func dialSMTP(addr string, host string) (*smtp.Client, error) {
	dialer := &net.Dialer{Timeout: emailDialTimeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func sendSMTP(client *smtp.Client, auth smtp.Auth, from string, to []string, msg []byte) error {
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func normalizeEmailAddresses(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		email := strings.TrimSpace(value)
		if email == "" {
			continue
		}
		key := strings.ToLower(email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, email)
	}
	return result
}

func emailRecipientsFromAddresses(values []string) []EmailRecipientInput {
	recipients := make([]EmailRecipientInput, 0, len(values))
	for _, email := range normalizeEmailAddresses(values) {
		recipients = append(recipients, EmailRecipientInput{Email: email})
	}
	return recipients
}

func normalizeEmailRecipients(values []EmailRecipientInput) []EmailRecipientInput {
	result := make([]EmailRecipientInput, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		email := strings.TrimSpace(value.Email)
		if email == "" {
			continue
		}
		key := strings.ToLower(email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		value.Email = email
		value.Guid = strings.TrimSpace(value.Guid)
		value.Name = strings.TrimSpace(value.Name)
		result = append(result, value)
	}
	return result
}

func formatEmailAddress(name string, email string) string {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" {
		return email
	}
	return mime.QEncoding.Encode("UTF-8", name) + " <" + email + ">"
}

func debugEmailTemplateVariables(code string) map[string]string {
	now := time.Now()
	nowText := now.Format("2006-01-02 15:04:05")
	lastSeen := now.Add(-12 * time.Minute).Format("2006-01-02 15:04:05")
	values := map[string]string{
		"eventTitle":    "NavMesh 邮件调试",
		"eventMessage":  "这是一封调试邮件，不代表真实设备事件。",
		"time":          nowText,
		"templateCode":  strings.TrimSpace(code),
		"deviceSncode":  "DEBUG-SN-001",
		"deviceAlias":   "调试设备",
		"deviceType":    "ssh",
		"hostIp":        "192.168.1.100",
		"wanIp":         "203.0.113.10",
		"clientVersion": "v0.0.0-debug",
		"lastSeenTime":  lastSeen,
		"releaseType":   "边缘客户端",
		"version":       "v0.0.0-debug",
		"platform":      "linux/arm64",
		"os":            "linux",
		"arch":          "arm64",
		"deviceScope":   "调试设备",
		"fileName":      "navmesh-client-debug",
		"downloadUrl":   "https://navmesh.example.com/debug-release",
		"changeLog":     "这是一封调试邮件，不代表真实版本发布。",
		"publishedAt":   nowText,
		"releaseGuid":   "debug-release-guid",
		"releaseSize":   "0",
		"diskUsedPct":   "92.5",
		"diskThreshold": "90",
		"diskFree":      "8.0 GB",
		"diskTotal":     "100.0 GB",
		"diskUsed":      "92.0 GB",
	}
	switch strings.TrimSpace(code) {
	case TemplateCodeDeviceOfflineNotice:
		values["eventTitle"] = "设备离线调试"
		values["eventMessage"] = "调试设备已超过心跳阈值未上报。"
	case TemplateCodeDiskUsageHighNotice:
		values["eventTitle"] = "磁盘阈值调试"
		values["eventMessage"] = "调试设备磁盘使用率已达到 92.5%，超过 90% 告警阈值。"
	}
	return values
}
