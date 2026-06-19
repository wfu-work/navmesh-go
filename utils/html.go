package utils

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"text/template"
	"time"
)

type EmailHTMLInput struct {
	Title     string
	Subject   string
	Content   string
	Variables map[string]string
}

func DefaultEmailHTML(input EmailHTMLInput) string {
	title := html.EscapeString(FirstNonEmpty(input.Title, input.Subject, "NavMesh 通知"))
	subject := html.EscapeString(FirstNonEmpty(input.Subject, input.Title, "通知"))
	content := emailHTMLContent(input.Content)
	sentAt := html.EscapeString(time.Now().Format("2006-01-02 15:04:05"))
	return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>` + title + `</title>
  <style>
    body, table, td, div, p, h1 { box-sizing: border-box; }
    p { margin: 0 0 14px; }
    .email-shell, .email-shell table { table-layout: fixed; }
    .email-content, .email-content * { max-width: 100% !important; overflow-wrap: anywhere; word-break: break-word; }
    @media screen and (max-width: 480px) {
      .email-outer { padding: 14px 8px !important; }
      .email-header { padding: 18px 16px 14px !important; }
      .email-body { padding: 20px 16px !important; }
      .email-footer { padding: 14px 16px 18px !important; }
      .email-title { font-size: 18px !important; line-height: 1.45 !important; }
      .email-badge-cell { display: none !important; }
    }
  </style>
</head>
<body style="margin:0;padding:0;background:#f4f7fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','PingFang SC','Microsoft YaHei',Arial,sans-serif;color:#1f2937;">
  <table class="email-outer" role="presentation" width="100%" cellpadding="0" cellspacing="0" style="width:100%;background:#f4f7fb;padding:24px 12px;">
    <tr>
      <td align="center">
        <table class="email-shell" role="presentation" width="100%" cellpadding="0" cellspacing="0" style="width:100%;background:#ffffff;border:1px solid #dbe5ef;border-radius:12px;overflow:hidden;">
          <tr>
            <td style="height:4px;background:#2563eb;font-size:0;line-height:0;">&nbsp;</td>
          </tr>
          <tr>
            <td class="email-header" style="padding:22px 24px 18px;border-bottom:1px solid #edf2f7;background:#fbfdff;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="width:100%;">
                <tr>
                  <td style="vertical-align:top;">
                    <div style="font-size:13px;line-height:1.5;font-weight:700;color:#64748b;letter-spacing:.2px;">NavMesh 自动通知</div>
                    <h1 class="email-title" style="margin:8px 0 0;font-size:20px;line-height:1.45;font-weight:700;color:#0f172a;overflow-wrap:anywhere;word-break:break-word;">` + subject + `</h1>
                  </td>
                  <td class="email-badge-cell" align="right" style="width:80px;vertical-align:top;">
                    <span style="display:inline-block;padding:4px 8px;border:1px solid #bfdbfe;border-radius:12px;background:#eff6ff;color:#1d4ed8;font-size:12px;line-height:1.4;font-weight:700;">通知</span>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td class="email-body" style="padding:24px;background:#ffffff;">
              <div class="email-content" style="font-size:15px;line-height:1.85;color:#334155;overflow-wrap:anywhere;word-break:break-word;">` + content + `</div>
            </td>
          </tr>
          <tr>
            <td class="email-footer" style="padding:16px 24px 20px;border-top:1px solid #edf2f7;background:#f8fafc;">
              <div style="color:#64748b;font-size:12px;line-height:1.75;">这是一封由 NavMesh 自动发送的通知邮件，请勿直接回复。</div>
              <div style="margin-top:4px;color:#94a3b8;font-size:12px;line-height:1.75;">发送时间：` + sentAt + `</div>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}

func RenderTemplateText(text string, variables map[string]string) string {
	tpl, err := template.New("email").Option("missingkey=zero").Parse(text)
	if err == nil {
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, variables); err == nil {
			return buf.String()
		}
	}
	result := text
	for key, value := range variables {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
		result = strings.ReplaceAll(result, "{{ "+key+" }}", value)
	}
	return result
}

func NormalizeTemplateVariables(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.Trim(key, "{} ")
		key = strings.TrimSpace(key)
		if key != "" {
			result[key] = strings.TrimSpace(value)
		}
	}
	return result
}

func emailHTMLContent(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "-"
	}
	if containsHTMLTag(trimmed) {
		return trimmed
	}
	escaped := html.EscapeString(trimmed)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return escaped
}

var htmlTagPattern = regexp.MustCompile(`(?is)<\s*/?\s*(p|br|div|span|strong|b|em|i|ul|ol|li|a|table|thead|tbody|tr|td|th|h[1-6]|section|article|blockquote|code|pre)\b[^>]*>`)

func containsHTMLTag(text string) bool {
	return htmlTagPattern.MatchString(text)
}
