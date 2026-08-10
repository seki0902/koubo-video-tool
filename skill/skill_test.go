package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"koubo-video-tool/store"
)

func TestBuildPromptLocal_SelectsRecruitmentLibrary(t *testing.T) {
	root := writeSkillRepo(t)

	cfg := store.Config{}
	cfg.Skill.Source = "local"
	cfg.Skill.LocalPath = root

	prompt, files, err := BuildPrompt(&cfg, "深圳海归人才专场招聘会")
	if err != nil {
		t.Fatal(err)
	}

	wantFiles := []string{
		"SKILL.md",
		"docs/structure-templates.md",
		"docs/jargon-lexicon.md",
		"docs/emotional-protocol.md",
		"docs/domain-knowledge.md",
		"library/招聘会活动.md",
		"examples/input-sample.md",
	}
	if strings.Join(files, ",") != strings.Join(wantFiles, ",") {
		t.Fatalf("files = %#v, want %#v", files, wantFiles)
	}

	if !strings.Contains(prompt, "root-marker") {
		t.Fatalf("prompt missing root content: %s", prompt)
	}
	if !strings.Contains(prompt, "recruitment-marker") {
		t.Fatalf("prompt missing recruitment library content: %s", prompt)
	}
	if !strings.Contains(prompt, "本工具运行约束") {
		t.Fatalf("prompt missing runtime guardrails: %s", prompt)
	}
	if strings.Contains(prompt, "strategy-marker") {
		t.Fatalf("prompt should not include fallback library content: %s", prompt)
	}
}

func TestBuildPromptLocal_DefaultsToPracticeLibrary(t *testing.T) {
	root := writeSkillRepo(t)

	cfg := store.Config{}
	cfg.Skill.Source = "local"
	cfg.Skill.LocalPath = filepath.Join(root, "library")

	prompt, files, err := BuildPrompt(&cfg, "留学生秋招 timeline")
	if err != nil {
		t.Fatal(err)
	}

	if len(files) < 6 || files[5] != "library/实操攻略.md" {
		t.Fatalf("unexpected files = %#v", files)
	}
	if !strings.Contains(prompt, "practice-marker") {
		t.Fatalf("prompt missing practice library content: %s", prompt)
	}
}

func TestReadLocalDir_LegacyFallback(t *testing.T) {
	dir := t.TempDir()

	mustWriteFile(t, filepath.Join(dir, "01-基础规范", "01-写作总则.md"), "# 写作总则\n内容第一")
	mustWriteFile(t, filepath.Join(dir, "02-素材库", "金句库.md"), "# 金句\n学无止境")

	result, err := readLocalDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "写作总则") || !strings.Contains(result, "金句") {
		t.Fatalf("legacy result missing content: %s", result)
	}
}

func TestRawToAPI(t *testing.T) {
	tests := []struct {
		rawURL     string
		wantAPI    string
		wantBranch string
	}{
		{
			"https://raw.githubusercontent.com/owner/repo/main/skills",
			"https://api.github.com/repos/owner/repo/contents/skills",
			"main",
		},
		{
			"https://github.com/owner/repo/tree/main/skills",
			"https://api.github.com/repos/owner/repo/contents/skills",
			"main",
		},
		{
			"https://api.github.com/repos/owner/repo/contents",
			"https://api.github.com/repos/owner/repo/contents",
			"",
		},
		{
			"not-a-github-url",
			"",
			"",
		},
	}
	for _, tt := range tests {
		gotAPI, gotBranch := rawToAPI(tt.rawURL)
		if gotAPI != tt.wantAPI || gotBranch != tt.wantBranch {
			t.Errorf("rawToAPI(%q) = (%q, %q), want (%q, %q)",
				tt.rawURL, gotAPI, gotBranch, tt.wantAPI, tt.wantBranch)
		}
	}
}

func writeSkillRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "SKILL.md"), "# root\nroot-marker")
	mustWriteFile(t, filepath.Join(root, "docs", "structure-templates.md"), "# docs\nstructure-marker")
	mustWriteFile(t, filepath.Join(root, "docs", "jargon-lexicon.md"), "# docs\njargon-marker")
	mustWriteFile(t, filepath.Join(root, "docs", "emotional-protocol.md"), "# docs\nemotion-marker")
	mustWriteFile(t, filepath.Join(root, "docs", "domain-knowledge.md"), "# docs\ndomain-marker")
	mustWriteFile(t, filepath.Join(root, "examples", "input-sample.md"), "# example\ninput-marker")
	mustWriteFile(t, filepath.Join(root, "examples", "output-sample.md"), "")
	mustWriteFile(t, filepath.Join(root, "library", "招聘会活动.md"), "# 招聘会\nrecruitment-marker")
	mustWriteFile(t, filepath.Join(root, "library", "实操攻略.md"), "# 实操\npractice-marker")
	mustWriteFile(t, filepath.Join(root, "library", "平台工具.md"), "# 平台\nplatform-marker")
	mustWriteFile(t, filepath.Join(root, "library", "单企业直招.md"), "# 单企业\nsingle-marker")
	mustWriteFile(t, filepath.Join(root, "library", "认知打破.md"), "# 认知\ncognition-marker")
	mustWriteFile(t, filepath.Join(root, "library", "赛道红利.md"), "# 赛道\nredline-marker")

	return root
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
