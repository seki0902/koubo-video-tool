package main

import (
	"strings"
	"testing"
)

func TestDesktopShortcutUsesActualExecutablePath(t *testing.T) {
	actualExe := `C:\Tools\koubo\custom-name.exe`

	content := desktopShortcutContent(actualExe)

	if !strings.Contains(content, actualExe) {
		t.Fatalf("shortcut content should point to actual exe path %q, got %q", actualExe, content)
	}
	if strings.Contains(content, `\koubo-video-tool.exe`) {
		t.Fatalf("shortcut content should not hard-code the release exe name: %q", content)
	}
}
