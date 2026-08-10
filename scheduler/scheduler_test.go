package scheduler

import (
	"fmt"
	"testing"
	"time"

	"koubo-video-tool/chanjing"
	"koubo-video-tool/store"
)

type mockClient struct {
	token     string
	statuses  []chanjing.VideoStatus
	index     int
	refreshes int
}

func (m *mockClient) GetToken() (string, error)         { return m.token, nil }
func (m *mockClient) Configure(appID, secretKey string) {}
func (m *mockClient) InvalidateTokenCache()             { m.refreshes++ }
func (m *mockClient) CreateVideo(token string, req chanjing.CreateVideoRequest) (string, error) {
	return "task-001", nil
}
func (m *mockClient) GetVideoStatus(token, taskID string) (chanjing.VideoStatus, error) {
	if m.index >= len(m.statuses) {
		return chanjing.VideoStatus{Status: 30, Progress: 100, VideoURL: "https://res.chanjing.cc/v.mp4"}, nil
	}
	s := m.statuses[m.index]
	m.index++
	return s, nil
}

func TestNextInterval(t *testing.T) {
	tests := []struct {
		count    int
		expected time.Duration
	}{
		{0, 2 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second},
		{10, 30 * time.Second},
	}

	for _, tt := range tests {
		got := nextInterval(tt.count)
		if got != tt.expected {
			t.Errorf("nextInterval(%d) = %v, want %v", tt.count, got, tt.expected)
		}
	}
}

func TestMaxPolls(t *testing.T) {
	if maxPolls != 30 {
		t.Errorf("maxPolls = %d, want 30", maxPolls)
	}
}

func TestSchedulerUpdate_Callback(t *testing.T) {
	var received store.Task
	mock := &mockClient{token: "test-token"}
	sch := New(mock, func(t store.Task) {
		received = t
	})

	sch.update("task-001", "done", 100, "https://res.chanjing.cc/v.mp4", "")
	sch.Stop()

	if received.TaskID != "task-001" {
		t.Errorf("TaskID = %q, want task-001", received.TaskID)
	}
	if received.Status != "done" {
		t.Errorf("Status = %q, want done", received.Status)
	}
	if received.VideoURL != "https://res.chanjing.cc/v.mp4" {
		t.Errorf("VideoURL = %q", received.VideoURL)
	}
	if received.Progress != 100 {
		t.Errorf("Progress = %d, want 100", received.Progress)
	}
}

func TestScheduler_Status30CompletesWithVideoURL(t *testing.T) {
	updates := make(chan store.Task, 1)
	mock := &mockClient{
		token: "test-token",
		statuses: []chanjing.VideoStatus{{
			Status:   30,
			Progress: 100,
			VideoURL: "https://res.chanjing.cc/result.mp4",
		}},
	}
	sch := New(mock, func(task store.Task) { updates <- task })
	defer sch.Stop()

	sch.Add("task-success", "test-token")
	select {
	case task := <-updates:
		if task.Status != "done" || task.VideoURL != "https://res.chanjing.cc/result.mp4" {
			t.Fatalf("unexpected successful update: %+v", task)
		}
		if task.CompletedAt == "" {
			t.Fatal("successful task must set completed_at")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler update")
	}
}

func TestScheduler_Status4xFails(t *testing.T) {
	updates := make(chan store.Task, 1)
	mock := &mockClient{
		token:    "test-token",
		statuses: []chanjing.VideoStatus{{Status: 40, Progress: 100, Msg: "参数异常"}},
	}
	sch := New(mock, func(task store.Task) { updates <- task })
	defer sch.Stop()

	sch.Add("task-failed", "test-token")
	select {
	case task := <-updates:
		if task.Status != "failed" || task.Error != "参数异常" {
			t.Fatalf("unexpected failed update: %+v", task)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler update")
	}
}

func TestScheduler_GeneratingDoesNotSetCompletedAt(t *testing.T) {
	var received store.Task
	sch := New(&mockClient{}, func(task store.Task) { received = task })
	sch.update("task-generating", "generating", 75, "", "")
	sch.Stop()

	if received.CompletedAt != "" {
		t.Fatalf("generating task has completed_at: %s", received.CompletedAt)
	}
}

func TestScheduler_RefreshUpdatesActiveStatus(t *testing.T) {
	updates := make(chan store.Task, 1)
	mock := &mockClient{
		token: "test-token",
		statuses: []chanjing.VideoStatus{{
			Status:   10,
			Progress: 75,
		}},
	}
	sch := New(mock, func(task store.Task) { updates <- task })
	defer sch.Stop()

	task, err := sch.Refresh("task-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "generating" || task.Progress != 75 {
		t.Fatalf("unexpected refreshed task: %+v", task)
	}
	select {
	case got := <-updates:
		if got.TaskID != "task-refresh" || got.Status != "generating" {
			t.Fatalf("unexpected callback task: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh callback")
	}
}

type refreshClient struct {
	mockClient
	failFirst bool
}

func (m *refreshClient) GetVideoStatus(token, taskID string) (chanjing.VideoStatus, error) {
	if m.failFirst {
		m.failFirst = false
		return chanjing.VideoStatus{}, fmt.Errorf("get video status failed: code=10400, msg=AccessToken已失效")
	}
	return chanjing.VideoStatus{Status: 30, Progress: 100, VideoURL: "https://res.chanjing.cc/v.mp4"}, nil
}

func TestScheduler_RetriesOnExpiredToken(t *testing.T) {
	updates := make(chan store.Task, 1)
	mock := &refreshClient{failFirst: true}
	sch := New(mock, func(task store.Task) { updates <- task })
	defer sch.Stop()

	sch.Add("task-refresh", "test-token")
	select {
	case task := <-updates:
		if task.Status != "done" {
			t.Fatalf("unexpected task status: %+v", task)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduler update")
	}
	if mock.refreshes == 0 {
		t.Fatal("expected token cache refresh")
	}
}
