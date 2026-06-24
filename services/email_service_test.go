package services

import (
	"strings"
	"testing"

	"navmesh-go/domains"
	"navmesh-go/utils"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRenderTemplateTextReplacesDoubleBraceVariables(t *testing.T) {
	got := utils.RenderTemplateText("版本 {{version}} 已发布，下载：{{ downloadUrl }}", map[string]string{
		"version":     "v1.2.3",
		"downloadUrl": "https://example.com/release",
	})

	if got != "版本 v1.2.3 已发布，下载：https://example.com/release" {
		t.Fatalf("renderTemplateText() = %q", got)
	}
}

func TestDefaultEmailHTMLWrapsContent(t *testing.T) {
	got := utils.DefaultEmailHTML(utils.EmailHTMLInput{
		Title:   "版本通知",
		Subject: "新版本已发布",
		Content: "第一行\n第二行",
		Variables: map[string]string{
			"version": "v1.2.3",
		},
	})

	for _, want := range []string{"<!doctype html>", "新版本已发布", "第一行<br>第二行", "NavMesh 自动通知"} {
		if !strings.Contains(got, want) {
			t.Fatalf("html does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "v1.2.3") {
		t.Fatalf("html should not append variables outside template content:\n%s", got)
	}
}

func TestDefaultEmailHTMLKeepsTemplateHTMLContent(t *testing.T) {
	got := utils.DefaultEmailHTML(utils.EmailHTMLInput{
		Title:   "版本通知",
		Subject: "新版本已发布",
		Content: `<p>版本 <strong>v1.2.3</strong> 已发布</p>`,
	})

	if !strings.Contains(got, `<p>版本 <strong>v1.2.3</strong> 已发布</p>`) {
		t.Fatalf("html template content should be embedded:\n%s", got)
	}
}

func TestNormalizeEmailAddressesTrimsAndDeduplicates(t *testing.T) {
	got := normalizeEmailAddresses([]string{" admin@example.com ", "", "ADMIN@example.com", "ops@example.com"})
	want := []string{"admin@example.com", "ops@example.com"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("address[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestMessageServiceSeedDefaultsCreatesReleaseTemplate(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)

	service.SeedDefaults()

	var tpl domains.MessageTemplate
	if err := db.Where("code = ?", TemplateCodeReleasePublished).First(&tpl).Error; err != nil {
		t.Fatalf("find default release template: %v", err)
	}
	if tpl.Subject != "{{releaseType}} {{version}} 已发布" {
		t.Fatalf("subject = %q", tpl.Subject)
	}
	if !strings.Contains(tpl.Content, "{{downloadUrl}}") {
		t.Fatalf("content should contain downloadUrl variable: %q", tpl.Content)
	}

	var offlineTpl domains.MessageTemplate
	if err := db.Where("code = ?", TemplateCodeDeviceOfflineNotice).First(&offlineTpl).Error; err != nil {
		t.Fatalf("find default offline template: %v", err)
	}
	if offlineTpl.Name != "设备离线通知" {
		t.Fatalf("offline template name = %q", offlineTpl.Name)
	}
	if !strings.Contains(offlineTpl.Content, "{{deviceAlias}}") {
		t.Fatalf("offline content should contain deviceAlias variable: %q", offlineTpl.Content)
	}

	var diskTpl domains.MessageTemplate
	if err := db.Where("code = ?", TemplateCodeDiskUsageHighNotice).First(&diskTpl).Error; err != nil {
		t.Fatalf("find default disk template: %v", err)
	}
	if diskTpl.Name != "磁盘阈值通知" {
		t.Fatalf("disk template name = %q", diskTpl.Name)
	}
	if !strings.Contains(diskTpl.Content, "{{diskUsedPct}}") || !strings.Contains(diskTpl.Content, "{{diskThreshold}}") {
		t.Fatalf("disk content should contain disk variables: %q", diskTpl.Content)
	}
}

func TestMessageServiceSaveTemplateDerivesNameFromCode(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)

	got, err := service.SaveTemplate(SaveMessageTemplateRequest{
		Code:    TemplateCodeDiskUsageHighNotice,
		Channel: "email",
		Subject: "{{deviceAlias}} 磁盘使用率达到 {{diskUsedPct}}%",
		Content: "设备 {{deviceAlias}} 磁盘使用率达到 {{diskUsedPct}}%",
		Status:  int(domains.StatusEnabled),
	})
	if err != nil {
		t.Fatalf("SaveTemplate() error = %v", err)
	}
	if got.Name != "磁盘阈值通知" {
		t.Fatalf("template name = %q, want 磁盘阈值通知", got.Name)
	}
}

func TestMessageServiceSeedDefaultsDoesNotOverrideExistingTemplate(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	existing := domains.MessageTemplate{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "custom-template"},
		Code:           TemplateCodeReleasePublished,
		Name:           "自定义版本通知",
		Channel:        "email",
		Subject:        "自定义主题",
		Content:        "自定义内容",
		Status:         int(domains.StatusEnabled),
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing template: %v", err)
	}

	service.SeedDefaults()

	var tpl domains.MessageTemplate
	if err := db.Where("code = ?", TemplateCodeReleasePublished).First(&tpl).Error; err != nil {
		t.Fatalf("find default release template: %v", err)
	}
	if tpl.Subject != "自定义主题" {
		t.Fatalf("subject = %q, want existing subject", tpl.Subject)
	}
}

func TestMessageServiceSeedDefaultsBackfillsRecipientMessageTypes(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	recipient := domains.MessageRecipient{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-empty"},
		Name:           "历史通知人员",
		Email:          "history@example.com",
		Status:         int(domains.StatusEnabled),
	}
	if err := db.Create(&recipient).Error; err != nil {
		t.Fatalf("create recipient: %v", err)
	}

	service.SeedDefaults()

	var got domains.MessageRecipient
	if err := db.Where("guid = ?", recipient.Guid).First(&got).Error; err != nil {
		t.Fatalf("find recipient: %v", err)
	}
	if got.MessageTypes != TemplateCodeReleasePublished {
		t.Fatalf("messageTypes = %q, want %q", got.MessageTypes, TemplateCodeReleasePublished)
	}
}

func TestMessageServiceListSendRecordsFiltersByStatus(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	records := []domains.MessageSendRecord{
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "record-success"},
			BatchGuid:      "batch-1",
			Channel:        "email",
			TemplateCode:   TemplateCodeReleasePublished,
			Subject:        "版本发布",
			RecipientEmail: "ops@example.com",
			SendStatus:     MessageSendStatusSuccess,
			ReceiveStatus:  MessageReceiveStatusAccepted,
			MaxRetries:     MaxMessageSendRetries,
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "record-failed"},
			BatchGuid:      "batch-1",
			Channel:        "email",
			TemplateCode:   TemplateCodeReleasePublished,
			Subject:        "版本发布",
			RecipientEmail: "admin@example.com",
			SendStatus:     MessageSendStatusFailed,
			ReceiveStatus:  MessageReceiveStatusFailed,
			MaxRetries:     MaxMessageSendRetries,
			ErrorMessage:   "smtp timeout",
		},
	}
	for _, record := range records {
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("create send record: %v", err)
		}
	}

	got, total, err := service.ListSendRecords(map[string]string{"sendStatus": MessageSendStatusFailed, "keyword": "timeout"})
	if err != nil {
		t.Fatalf("ListSendRecords() error = %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].Guid != "record-failed" {
		t.Fatalf("ListSendRecords() total=%d records=%#v", total, got)
	}
}

func TestMessageServiceDeleteSendRecordRemovesRecord(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	record := domains.MessageSendRecord{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "record-delete"},
		Channel:        "email",
		TemplateCode:   TemplateCodeReleasePublished,
		Subject:        "版本发布",
		RecipientEmail: "ops@example.com",
		SendStatus:     MessageSendStatusSuccess,
		ReceiveStatus:  MessageReceiveStatusAccepted,
		MaxRetries:     MaxMessageSendRetries,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("create send record: %v", err)
	}

	if err := service.DeleteSendRecord(record.Guid); err != nil {
		t.Fatalf("DeleteSendRecord() error = %v", err)
	}

	var count int64
	if err := db.Model(&domains.MessageSendRecord{}).Where("guid = ?", record.Guid).Count(&count).Error; err != nil {
		t.Fatalf("count send records: %v", err)
	}
	if count != 0 {
		t.Fatalf("send record count = %d, want 0", count)
	}
}

func TestMessageServiceEnabledRecipientsByMessageTypeFiltersSelections(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	recipients := []domains.MessageRecipient{
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-release", UpdateTime: 300},
			Name:           "版本通知人员",
			Email:          "release@example.com",
			MessageTypes:   TemplateCodeReleasePublished,
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-other", UpdateTime: 200},
			Name:           "其他通知人员",
			Email:          "other@example.com",
			MessageTypes:   TemplateCodeDeviceOfflineNotice,
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-empty", UpdateTime: 100},
			Name:           "未配置类型人员",
			Email:          "empty@example.com",
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-disabled", UpdateTime: 400},
			Name:           "禁用人员",
			Email:          "disabled@example.com",
			MessageTypes:   TemplateCodeReleasePublished,
			Status:         int(domains.StatusDisabled),
		},
	}
	for _, recipient := range recipients {
		if err := db.Create(&recipient).Error; err != nil {
			t.Fatalf("create recipient: %v", err)
		}
	}

	got, err := service.EnabledRecipientsByMessageType(TemplateCodeReleasePublished)
	if err != nil {
		t.Fatalf("EnabledRecipientsByMessageType() error = %v", err)
	}
	if len(got) != 1 || got[0].Guid != "recipient-release" {
		t.Fatalf("EnabledRecipientsByMessageType() = %#v", got)
	}
}

func TestMessageServiceEnabledRecipientsByMessageTypeAndDeviceFiltersDeviceScope(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	recipients := []domains.MessageRecipient{
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-all", UpdateTime: 400},
			Name:           "全部设备通知人员",
			Email:          "all@example.com",
			MessageTypes:   TemplateCodeDeviceOfflineNotice,
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-device-a", UpdateTime: 300},
			Name:           "设备A通知人员",
			Email:          "device-a@example.com",
			MessageTypes:   TemplateCodeDeviceOfflineNotice + "," + TemplateCodeDiskUsageHighNotice,
			DeviceGuids:    "device-a, device-b, device-a",
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-device-c", UpdateTime: 200},
			Name:           "设备C通知人员",
			Email:          "device-c@example.com",
			MessageTypes:   TemplateCodeDeviceOfflineNotice,
			DeviceGuids:    "device-c",
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-release", UpdateTime: 100},
			Name:           "版本通知人员",
			Email:          "release@example.com",
			MessageTypes:   TemplateCodeReleasePublished,
			DeviceGuids:    "device-a",
			Status:         int(domains.StatusEnabled),
		},
	}
	for _, recipient := range recipients {
		if err := db.Create(&recipient).Error; err != nil {
			t.Fatalf("create recipient: %v", err)
		}
	}

	got, err := service.EnabledRecipientsByMessageTypeAndDevice(TemplateCodeDeviceOfflineNotice, "device-a")
	if err != nil {
		t.Fatalf("EnabledRecipientsByMessageTypeAndDevice() error = %v", err)
	}
	if gotGuids := recipientGuids(got); strings.Join(gotGuids, ",") != "recipient-all,recipient-device-a" {
		t.Fatalf("device-a recipients = %#v, want all and device-a", gotGuids)
	}

	got, err = service.EnabledRecipientsByMessageTypeAndDevice(TemplateCodeDeviceOfflineNotice, "device-c")
	if err != nil {
		t.Fatalf("EnabledRecipientsByMessageTypeAndDevice() error = %v", err)
	}
	if gotGuids := recipientGuids(got); strings.Join(gotGuids, ",") != "recipient-all,recipient-device-c" {
		t.Fatalf("device-c recipients = %#v, want all and device-c", gotGuids)
	}
}

func TestMessageServiceEnabledRecipientsByGuidsFiltersRecipients(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	recipients := []domains.MessageRecipient{
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-enabled", UpdateTime: 300},
			Name:           "启用人员",
			Email:          "enabled@example.com",
			Status:         int(domains.StatusEnabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-disabled", UpdateTime: 200},
			Name:           "禁用人员",
			Email:          "disabled@example.com",
			Status:         int(domains.StatusDisabled),
		},
		{
			BaseDataEntity: commonDomains.BaseDataEntity{Guid: "recipient-empty-email", UpdateTime: 100},
			Name:           "空邮箱人员",
			Status:         int(domains.StatusEnabled),
		},
	}
	for _, recipient := range recipients {
		if err := db.Create(&recipient).Error; err != nil {
			t.Fatalf("create recipient: %v", err)
		}
	}

	got, err := service.EnabledRecipientsByGuids([]string{"recipient-enabled", "recipient-disabled", "recipient-empty-email"})
	if err != nil {
		t.Fatalf("EnabledRecipientsByGuids() error = %v", err)
	}
	if len(got) != 1 || got[0].Guid != "recipient-enabled" {
		t.Fatalf("EnabledRecipientsByGuids() = %#v", got)
	}
}

func TestMessageServiceSaveRecipientNormalizesDeviceGuids(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)

	got, err := service.SaveRecipient(SaveMessageRecipientRequest{
		Name:         "离线通知人员",
		Email:        "offline@example.com",
		MessageTypes: TemplateCodeDeviceOfflineNotice,
		DeviceGuids:  " device-a,device-b,device-a,, ",
		Status:       int(domains.StatusEnabled),
	})
	if err != nil {
		t.Fatalf("SaveRecipient() error = %v", err)
	}
	if got.DeviceGuids != "device-a,device-b" {
		t.Fatalf("DeviceGuids = %q, want normalized device list", got.DeviceGuids)
	}
}

func TestDebugEmailTemplateVariablesContainTemplateValues(t *testing.T) {
	values := debugEmailTemplateVariables(TemplateCodeDiskUsageHighNotice)

	for _, key := range []string{"deviceAlias", "deviceSncode", "lastSeenTime", "version", "downloadUrl", "templateCode", "diskUsedPct", "diskThreshold", "diskFree"} {
		if strings.TrimSpace(values[key]) == "" {
			t.Fatalf("debug variable %s should not be empty: %#v", key, values)
		}
	}
	if values["templateCode"] != TemplateCodeDiskUsageHighNotice {
		t.Fatalf("templateCode = %q, want %q", values["templateCode"], TemplateCodeDiskUsageHighNotice)
	}
	if values["eventTitle"] != "磁盘阈值调试" {
		t.Fatalf("eventTitle = %q, want 磁盘阈值调试", values["eventTitle"])
	}
}

func TestEmailServicePreviewTemplateRendersDefaultHTML(t *testing.T) {
	result, err := EmailService{}.PreviewTemplate(EmailTemplatePreviewInput{
		Code:    TemplateCodeDiskUsageHighNotice,
		Subject: "{{deviceAlias}} 磁盘使用率达到 {{diskUsedPct}}%",
		Content: "<p>设备 {{deviceAlias}} 已超过 {{diskThreshold}}% 阈值，当前 {{diskUsedPct}}%。</p>",
	})
	if err != nil {
		t.Fatalf("PreviewTemplate() error = %v", err)
	}
	for _, want := range []string{"【调试】调试设备", "92.5%", "90%", "这是一封调试邮件"} {
		if !strings.Contains(result.Subject+result.HTML, want) {
			t.Fatalf("preview should contain %q: %#v", want, result)
		}
	}
	if !strings.Contains(result.HTML, "<!doctype html>") || !strings.Contains(result.HTML, "NavMesh 自动通知") {
		t.Fatalf("preview html should use default email wrapper:\n%s", result.HTML)
	}
	if strings.Contains(result.HTML, "{{") || strings.Contains(result.Subject, "{{") {
		t.Fatalf("preview should render template variables: %#v", result)
	}
}

func TestMessageServiceRetrySendRecordRejectsSuccessfulRecord(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	record := domains.MessageSendRecord{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "record-success"},
		Channel:        "email",
		TemplateCode:   TemplateCodeReleasePublished,
		Subject:        "版本发布",
		RecipientEmail: "ops@example.com",
		HTMLContent:    "<p>ok</p>",
		SendStatus:     MessageSendStatusSuccess,
		ReceiveStatus:  MessageReceiveStatusAccepted,
		MaxRetries:     MaxMessageSendRetries,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("create send record: %v", err)
	}

	if _, err := service.RetrySendRecord(record.Guid); err == nil || !strings.Contains(err.Error(), "already successful") {
		t.Fatalf("RetrySendRecord() error = %v, want already successful", err)
	}
}

func TestMessageServiceRetrySendRecordRejectsRetryLimit(t *testing.T) {
	db := setupMessageServiceTestDB(t)
	service := MessageService{}.WithDB(db)
	record := domains.MessageSendRecord{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "record-failed"},
		Channel:        "email",
		TemplateCode:   TemplateCodeReleasePublished,
		Subject:        "版本发布",
		RecipientEmail: "ops@example.com",
		HTMLContent:    "<p>failed</p>",
		SendStatus:     MessageSendStatusFailed,
		ReceiveStatus:  MessageReceiveStatusFailed,
		RetryCount:     MaxMessageSendRetries,
		MaxRetries:     MaxMessageSendRetries,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("create send record: %v", err)
	}

	if _, err := service.RetrySendRecord(record.Guid); err == nil || !strings.Contains(err.Error(), "retry limit") {
		t.Fatalf("RetrySendRecord() error = %v, want retry limit", err)
	}
}

func TestReleasePublishedTemplateVariables(t *testing.T) {
	release := &domains.Release{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "release-guid", UpdateTime: 1710000000000},
		ReleaseType:    domains.ReleaseTypeNavmesh,
		DeviceType:     "rain",
		Version:        "v0.0.5",
		OS:             "linux",
		Arch:           "arm64",
		FileName:       "navmesh-client-linux-arm64",
		ChangeLog:      "修复升级流程",
	}

	vars := ReleasePublishedTemplateVariables(release, "https://navmesh.example.com/api/downloads/releases/release-guid")

	assertVar(t, vars, "releaseType", "边缘客户端")
	assertVar(t, vars, "version", "v0.0.5")
	assertVar(t, vars, "platform", "linux/arm64")
	assertVar(t, vars, "deviceScope", "rain")
	assertVar(t, vars, "downloadUrl", "https://navmesh.example.com/api/downloads/releases/release-guid")
	assertVar(t, vars, "changeLog", "修复升级流程")
	assertVar(t, vars, "eventTitle", "边缘客户端版本已发布")
}

func TestDeviceOfflineTemplateVariables(t *testing.T) {
	lastSeenAt := int64(1710000000000)
	eventAt := int64(1710000600000)
	device := &domains.Device{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "device-guid"},
		Sncode:         "SN-001",
		Alias:          "边缘站点",
		DeviceType:     "ssh",
		HostIP:         "192.168.1.10",
		WanIP:          "203.0.113.10",
		ClientVersion:  "v1.2.3",
		LastSeenTime:   lastSeenAt,
	}

	vars := DeviceOfflineTemplateVariables(device, "设备心跳超过 300 秒未更新", eventAt)

	assertVar(t, vars, "eventTitle", "设备离线")
	assertVar(t, vars, "eventMessage", "设备心跳超过 300 秒未更新")
	assertVar(t, vars, "deviceAlias", "边缘站点")
	assertVar(t, vars, "deviceSncode", "SN-001")
	assertVar(t, vars, "deviceType", "ssh")
	assertVar(t, vars, "hostIp", "192.168.1.10")
	assertVar(t, vars, "wanIp", "203.0.113.10")
	assertVar(t, vars, "clientVersion", "v1.2.3")
	assertVar(t, vars, "lastSeenTime", formatTemplateTime(lastSeenAt))
	assertVar(t, vars, "time", formatTemplateTime(eventAt))
}

func TestDiskUsageHighTemplateVariables(t *testing.T) {
	device := &domains.Device{
		BaseDataEntity: commonDomains.BaseDataEntity{Guid: "device-guid"},
		Sncode:         "SN-001",
		Alias:          "边缘站点",
		DeviceType:     "ssh",
		HostIP:         "192.168.1.10",
		WanIP:          "203.0.113.10",
		ClientVersion:  "v1.2.3",
	}
	req := HeartbeatRequest{
		HostIP:        "192.168.1.20",
		DiskTotal:     100 * 1024 * 1024 * 1024,
		DiskUsed:      92 * 1024 * 1024 * 1024,
		DiskFree:      8 * 1024 * 1024 * 1024,
		DiskUsedPct:   92.3,
		ClientVersion: "v1.2.4",
	}

	vars := DiskUsageHighTemplateVariables(device, req, "磁盘使用率 92.3%，已达到 90% 告警阈值", 1710000000000)

	assertVar(t, vars, "eventTitle", "磁盘空间不足")
	assertVar(t, vars, "deviceAlias", "边缘站点")
	assertVar(t, vars, "hostIp", "192.168.1.20")
	assertVar(t, vars, "clientVersion", "v1.2.4")
	assertVar(t, vars, "diskUsedPct", "92.3")
	assertVar(t, vars, "diskThreshold", "90")
	assertVar(t, vars, "diskFree", "8.0 GB")
	assertVar(t, vars, "diskTotal", "100.0 GB")
	assertVar(t, vars, "diskUsed", "92.0 GB")
}

func assertVar(t *testing.T, values map[string]string, key string, want string) {
	t.Helper()
	if got := values[key]; got != want {
		t.Fatalf("variable %s = %q, want %q", key, got, want)
	}
}

func recipientGuids(recipients []domains.MessageRecipient) []string {
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		out = append(out, recipient.Guid)
	}
	return out
}

func setupMessageServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domains.MessageTemplate{}, &domains.MessageRecipient{}, &domains.MessageSendRecord{}); err != nil {
		t.Fatalf("migrate message tables: %v", err)
	}
	return db
}
