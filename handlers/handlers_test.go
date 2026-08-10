package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"koubo-video-tool/chanjing"
	"koubo-video-tool/scheduler"
	"koubo-video-tool/store"
)

type mockChanjing struct {
	token       string
	lastRequest chanjing.CreateVideoRequest
	appID       string
	secretKey   string
}

func (m *mockChanjing) GetToken() (string, error) { return m.token, nil }
func (m *mockChanjing) CreateVideo(token string, req chanjing.CreateVideoRequest) (string, error) {
	m.lastRequest = req
	return "task-mock-001", nil
}
func (m *mockChanjing) GetVideoStatus(token, taskID string) (chanjing.VideoStatus, error) {
	return chanjing.VideoStatus{Status: 30, Progress: 100, VideoURL: "https://res.chanjing.cc/v.mp4"}, nil
}
func (m *mockChanjing) Configure(appID, secretKey string) {
	m.appID = appID
	m.secretKey = secretKey
}
func (m *mockChanjing) InvalidateTokenCache() {}

func setupTestData(t *testing.T, dir string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, "tasks.json"), []byte("[]"), 0644)
	cfg := `{"chanjing":{"app_id":"test","secret_key":""},"llm":{"api_url":"https://api.deepseek.com","api_key":"sk-test","model":"test"},"skill":{"source":"local","local_path":"` + dir + `"}}`
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0644)
}

func TestCreateTask_Success(t *testing.T) {
	dir := t.TempDir()
	setupTestData(t, dir)

	mockCJ := &mockChanjing{token: "test-token"}
	sch := scheduler.New(mockCJ, func(_ store.Task) {})
	defer sch.Stop()

	h := New(dir, mockCJ, sch)
	h.Store = &StorePaths{
		Config:  filepath.Join(dir, "config.json"),
		Tasks:   filepath.Join(dir, "tasks.json"),
		Avatars: filepath.Join(dir, "avatars.json"),
		Voices:  filepath.Join(dir, "voices.json"),
	}

	body := `{"script":"测试稿子内容","avatar_id":"C-test","audio_man_id":"C-voice","speed":1.0,"volume":100,"bg_color":"#ffffff","resolution":"1080P","subtitle_enabled":true,"figure_type":"stand_body","drive_mode":"random","backway":1,"language":"cn","compliance_watermark":false}`

	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.createTask(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["task_id"] != "task-mock-001" {
		t.Errorf("task_id = %q, want task-mock-001", resp["task_id"])
	}

	tasks, err := store.LoadTasks(filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Script != "测试稿子内容" {
		t.Fatalf("saved task script mismatch: %+v", tasks)
	}
}

func TestRefreshTask_UpdatesStoredStatus(t *testing.T) {
	dir := t.TempDir()
	setupTestData(t, dir)

	taskPath := filepath.Join(dir, "tasks.json")
	if err := store.SaveTasks(taskPath, []store.Task{{
		TaskID:   "task-refresh-001",
		Status:   "generating",
		Progress: 75,
		Script:   "娴嬭瘯绋垮瓙",
	}}); err != nil {
		t.Fatal(err)
	}

	mockCJ := &mockChanjing{token: "test-token"}
	sch := scheduler.New(mockCJ, func(t store.Task) {
		store.UpdateTask(taskPath, t.TaskID, func(existing *store.Task) {
			existing.Status = t.Status
			existing.Progress = t.Progress
			existing.VideoURL = t.VideoURL
			existing.Error = t.Error
			existing.CompletedAt = t.CompletedAt
		})
	})
	defer sch.Stop()

	h := New(dir, mockCJ, sch)
	h.Store = &StorePaths{
		Config:  filepath.Join(dir, "config.json"),
		Tasks:   taskPath,
		Avatars: filepath.Join(dir, "avatars.json"),
		Voices:  filepath.Join(dir, "voices.json"),
	}

	req := httptest.NewRequest("POST", "/api/tasks/task-refresh-001/refresh", nil)
	rec := httptest.NewRecorder()
	h.handleTaskByID(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	tasks, err := store.LoadTasks(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Status != "done" || tasks[0].CompletedAt == "" {
		t.Fatalf("refresh did not update task: %+v", tasks)
	}
}

func TestCreateTask_EmptyScript(t *testing.T) {
	dir := t.TempDir()
	setupTestData(t, dir)

	mockCJ := &mockChanjing{token: "test-token"}
	sch := scheduler.New(mockCJ, func(_ store.Task) {})
	defer sch.Stop()

	h := New(dir, mockCJ, sch)
	h.Store = &StorePaths{
		Config: filepath.Join(dir, "config.json"),
		Tasks:  filepath.Join(dir, "tasks.json"),
	}

	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"script":"","avatar_id":"C-test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.createTask(rec, req)

	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateTask_UsesVoiceSourceMetadata(t *testing.T) {
	tests := []struct {
		name            string
		voice           store.Voice
		wantAudioSource int
	}{
		{
			name:  "system voice",
			voice: store.Voice{ID: "C-system", Name: "系统人声"},
		},
		{
			name:            "Web custom voice",
			voice:           store.Voice{ID: "C-web", Name: "Web 自定义人声", AudioSource: 1},
			wantAudioSource: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			setupTestData(t, dir)
			voiceData, err := json.Marshal([]store.Voice{tt.voice})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "voices.json"), voiceData, 0644); err != nil {
				t.Fatal(err)
			}

			mockCJ := &mockChanjing{token: "test-token"}
			sch := scheduler.New(mockCJ, func(_ store.Task) {})
			defer sch.Stop()
			h := New(dir, mockCJ, sch)

			body := `{"script":"测试稿子","avatar_id":"C-avatar","audio_man_id":"` + tt.voice.ID + `"}`
			req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.createTask(rec, req)

			if rec.Code != 200 {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			if got := mockCJ.lastRequest.Source; got != 1 {
				t.Errorf("top-level source = %d, want 1", got)
			}
			if got := mockCJ.lastRequest.AudioSource; got != tt.wantAudioSource {
				t.Errorf("top-level audio_source = %d, want %d", got, tt.wantAudioSource)
			}
		})
	}
}

func TestRetryTask_PreservesWebVoiceSourceFields(t *testing.T) {
	dir := t.TempDir()
	setupTestData(t, dir)
	voiceData, err := json.Marshal([]store.Voice{{ID: "C-custom", AudioSource: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "voices.json"), voiceData, 0644); err != nil {
		t.Fatal(err)
	}
	var legacyParams map[string]any
	if err := json.Unmarshal([]byte(`{"audio":{"tts":{"audio_man":"C-custom","source":1,"audio_source":1}},"source":1}`), &legacyParams); err != nil {
		t.Fatal(err)
	}
	oldTask := store.Task{
		TaskID: "old-system-task",
		Status: "failed",
		Script: "旧稿件内容",
		Params: legacyParams,
	}
	if err := store.SaveTasks(filepath.Join(dir, "tasks.json"), []store.Task{oldTask}); err != nil {
		t.Fatal(err)
	}

	mockCJ := &mockChanjing{token: "test-token"}
	sch := scheduler.New(mockCJ, func(_ store.Task) {})
	defer sch.Stop()
	h := New(dir, mockCJ, sch)
	req := httptest.NewRequest("POST", "/api/tasks/old-system-task/retry", nil)
	rec := httptest.NewRecorder()

	h.retryTask(rec, req, oldTask.TaskID)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	retriedJSON, err := json.Marshal(mockCJ.lastRequest)
	if err != nil {
		t.Fatal(err)
	}
	var retried map[string]any
	if err := json.Unmarshal(retriedJSON, &retried); err != nil {
		t.Fatal(err)
	}
	if retried["source"] != float64(1) {
		t.Fatalf("retry lost Web person source: %s", retriedJSON)
	}
	if retried["audio_source"] != float64(1) {
		t.Fatalf("retry did not migrate Web voice audio_source to top level: %s", retriedJSON)
	}

	tasks, err := store.LoadTasks(filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[1].Script == "" {
		t.Fatalf("retry task script not saved: %+v", tasks)
	}
	if tasks[1].Script != "旧稿件内容" {
		t.Fatalf("retry task script mismatch: %+v", tasks[1])
	}
}

func TestSettingsUpdate_RefreshesRuntimeCredentials(t *testing.T) {
	dir := t.TempDir()
	setupTestData(t, dir)
	mockCJ := &mockChanjing{}
	h := New(dir, mockCJ, nil)
	body := `{"chanjing":{"app_id":"44602067","secret_key":"new-secret"},"llm":{},"skill":{}}`
	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleSettings(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if mockCJ.appID != "44602067" || mockCJ.secretKey != "new-secret" {
		t.Fatalf("runtime credentials were not refreshed")
	}
}

func TestSettingsUpdate_RejectsBlankChanjingCredentials(t *testing.T) {
	dir := t.TempDir()
	setupTestData(t, dir)
	configPath := filepath.Join(dir, "config.json")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	h := New(dir, &mockChanjing{}, nil)
	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"chanjing":{"app_id":"","secret_key":""}}`))
	rec := httptest.NewRecorder()

	h.handleSettings(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("blank settings request overwrote the existing config")
	}
}
