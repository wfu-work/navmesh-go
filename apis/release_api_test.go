package apis

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"navmesh-go/domains"
	"navmesh-go/services"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/wfu-work/nav-common-go-lib/global"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReleaseUploadRecordsPublishedEventAndPublishesWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupReleaseAPITestDB(t)
	oldHub := services.ServiceGroupApp.EventHub
	hub := services.NewEventHub()
	services.ServiceGroupApp.EventHub = hub
	t.Cleanup(func() {
		services.ServiceGroupApp.EventHub = oldHub
	})
	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("releaseType", "navmesh"); err != nil {
		t.Fatalf("write releaseType: %v", err)
	}
	if err := writer.WriteField("version", "v0.0.5"); err != nil {
		t.Fatalf("write version: %v", err)
	}
	part, err := writer.CreateFormFile("file", "navmesh-client-linux-amd64")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("#!/bin/sh\n")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/releases/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req

	ReleaseApi{}.Upload(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var event domains.Event
	if err := db.Where("event_type = ?", "release_published").First(&event).Error; err != nil {
		t.Fatalf("find release event: %v", err)
	}
	if event.Title != "边缘客户端版本已发布" {
		t.Fatalf("event title = %q, want 边缘客户端版本已发布", event.Title)
	}
	if !strings.Contains(event.Message, "v0.0.5") || !strings.Contains(event.Message, "linux/amd64") {
		t.Fatalf("event message = %q, want version and platform", event.Message)
	}

	select {
	case notification := <-ch:
		if notification.Type != "event.created" {
			t.Fatalf("notification type = %q, want event.created", notification.Type)
		}
		if notification.Data.EventType != "release_published" {
			t.Fatalf("notification event type = %q, want release_published", notification.Data.EventType)
		}
	case <-time.After(time.Second):
		t.Fatal("expected release published websocket notification")
	}
}

func setupReleaseAPITestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := global.NAV_DB
	oldViper := global.NAV_VIPER
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&domains.Release{}, &domains.Event{}, &domains.AuditLog{}); err != nil {
		t.Fatalf("migrate release api tables: %v", err)
	}
	cfg := viper.New()
	cfg.Set("local.oss-path", t.TempDir())
	global.NAV_DB = db
	global.NAV_VIPER = cfg
	t.Cleanup(func() {
		global.NAV_DB = oldDB
		global.NAV_VIPER = oldViper
	})
	return db
}

func TestRenderInstallScriptInjectsDefaultDownloadBase(t *testing.T) {
	script := []byte(`#!/bin/sh
DOWNLOAD_BASE=""
echo "$DOWNLOAD_BASE"
`)

	rendered := string(renderInstallScript(script, `https://navmesh.example.com/api/downloads`))

	if !strings.Contains(rendered, `DOWNLOAD_BASE="https://navmesh.example.com/api/downloads"`) {
		t.Fatalf("rendered script did not include default download base:\n%s", rendered)
	}
}

func TestShellDoubleQuotedEscapesSpecialCharacters(t *testing.T) {
	got := shellDoubleQuoted(`https://example.com/a"b$c\path`)
	want := `"https://example.com/a\"b\$c\\path"`
	if got != want {
		t.Fatalf("shellDoubleQuoted() = %q, want %q", got, want)
	}
}
