package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")

	c := Config{}
	c.Chanjing.AppID = "test_ak"
	c.Chanjing.SecretKey = "test_sk"
	c.LLM.APIURL = "https://api.deepseek.com"
	c.LLM.APIKey = "sk-test"
	c.LLM.Model = "deepseek-chat"
	c.Skill.Source = "local"
	c.Skill.LocalPath = "/tmp/skills"

	if err := SaveConfig(p, c); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.Chanjing.AppID != "test_ak" {
		t.Errorf("AppID = %q, want %q", got.Chanjing.AppID, "test_ak")
	}
	if got.LLM.Model != "deepseek-chat" {
		t.Errorf("Model = %q, want %q", got.LLM.Model, "deepseek-chat")
	}
}

func TestSaveAndLoadTasks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tasks.json")

	tasks := []Task{
		{TaskID: "t1", Status: "pending", CreatedAt: "2026-07-29T10:00:00Z"},
		{TaskID: "t2", Status: "done", VideoURL: "https://example.com/v.mp4", CreatedAt: "2026-07-29T11:00:00Z"},
	}

	if err := SaveTasks(p, tasks); err != nil {
		t.Fatalf("SaveTasks: %v", err)
	}

	got, err := LoadTasks(p)
	if err != nil {
		t.Fatalf("LoadTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[1].VideoURL != "https://example.com/v.mp4" {
		t.Errorf("VideoURL = %q", got[1].VideoURL)
	}
}

func TestLoadTasks_FileNotExist(t *testing.T) {
	tasks, err := LoadTasks(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("len = %d, want 0", len(tasks))
	}
}

func TestTaskHistoryRetentionAndDeletion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	tasks := []Task{
		{TaskID: "old-done", Status: "done", CreatedAt: "2026-06-01T00:00:00Z"},
		{TaskID: "recent-done", Status: "done", CreatedAt: "2026-08-01T00:00:00Z"},
		{TaskID: "old-active", Status: "generating", CreatedAt: "2026-06-01T00:00:00Z"},
	}
	if err := SaveTasks(path, tasks); err != nil {
		t.Fatal(err)
	}
	removed, err := PruneExpiredTasks(path, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))
	if err != nil || removed != 1 {
		t.Fatalf("PruneExpiredTasks removed=%d err=%v", removed, err)
	}
	if err := DeleteTask(path, "old-active"); err != ErrTaskInProgress {
		t.Fatalf("active delete err=%v, want %v", err, ErrTaskInProgress)
	}
	if err := DeleteTask(path, "recent-done"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTasks(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TaskID != "old-active" {
		t.Fatalf("remaining tasks=%+v", got)
	}
}

func TestClearTerminalTasksKeepsActiveTasks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	if err := SaveTasks(path, []Task{{TaskID: "done", Status: "done"}, {TaskID: "failed", Status: "failed"}, {TaskID: "active", Status: "pending"}}); err != nil {
		t.Fatal(err)
	}
	removed, err := ClearTerminalTasks(path)
	if err != nil || removed != 2 {
		t.Fatalf("ClearTerminalTasks removed=%d err=%v", removed, err)
	}
	got, err := LoadTasks(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TaskID != "active" {
		t.Fatalf("remaining tasks=%+v", got)
	}
}

func TestLoadAvatarsAndVoices(t *testing.T) {
	dir := t.TempDir()

	ap := filepath.Join(dir, "avatars.json")
	avatars := []Avatar{{ID: "C-001", Name: "女主播"}}
	if err := write(ap, avatars); err != nil {
		t.Fatal(err)
	}
	gotA, err := LoadAvatars(ap)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotA) != 1 || gotA[0].Name != "女主播" {
		t.Errorf("avatars mismatch")
	}

	vp := filepath.Join(dir, "voices.json")
	voices := []Voice{{ID: "C-v1", Name: "沉稳男声", Gender: "男"}}
	if err := write(vp, voices); err != nil {
		t.Fatal(err)
	}
	gotV, err := LoadVoices(vp)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotV) != 1 || gotV[0].Name != "沉稳男声" {
		t.Errorf("voices mismatch")
	}
}

func TestConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")

	c1 := Config{}
	c1.Skill.Source = "github"
	c1.Skill.GithubURL = "https://raw.githubusercontent.com/user/repo/main"

	if err := SaveConfig(p, c1); err != nil {
		t.Fatal(err)
	}
	c2, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Skill.GithubURL != c1.Skill.GithubURL {
		t.Error("github URL mismatch after round-trip")
	}

	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Error("config.json not written to disk")
	}
}

func TestLoadConfig_InvalidEncryptedSecret(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"chanjing":{"app_id":"test","secret_key":"ENC:not-valid"}}`)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadConfig(p); err == nil {
		t.Fatal("LoadConfig should report an invalid encrypted SecretKey")
	}
}
