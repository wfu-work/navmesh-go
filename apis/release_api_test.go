package apis

import (
	"strings"
	"testing"
)

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
