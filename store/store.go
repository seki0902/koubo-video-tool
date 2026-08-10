package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"koubo-video-tool/crypto"
)

// Config 应用设置
type Config struct {
	Chanjing struct {
		AppID     string `json:"app_id"`
		SecretKey string `json:"secret_key"`
	} `json:"chanjing"`
	LLM struct {
		APIURL string `json:"api_url"`
		APIKey string `json:"api_key"`
		Model  string `json:"model"`
	} `json:"llm"`
	Skill struct {
		Source           string `json:"source"` // "local" | "github"
		LocalPath        string `json:"local_path"`
		GithubURL        string `json:"github_url"`
		GithubCacheUntil int64  `json:"github_cache_until"`
	} `json:"skill"`
}

// Task 视频生成任务
type Task struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"` // pending|generating|done|failed|timeout
	Progress    int    `json:"progress"`
	VideoURL    string `json:"video_url"`
	Error       string `json:"error"`
	Script      string `json:"script,omitempty"`
	Params      any    `json:"params"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at"`
	PollCount   int    `json:"poll_count"`
}

// Avatar 数字人形象
type Avatar struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PreviewURL string `json:"preview_url"`
}

// Voice 人声
type Voice struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Gender          string `json:"gender"`
	Description     string `json:"description"`
	PreviewAudioURL string `json:"preview_audio_url"`
	AudioSource     int    `json:"audio_source,omitempty"`
}

// MaskedValue 用于前端传输时隐藏密钥的真实值
const MaskedValue = "\x00masked\x00"

var mu sync.RWMutex

var (
	ErrTaskNotFound   = errors.New("任务不存在")
	ErrTaskInProgress = errors.New("生成中的任务不能删除")
)

func read(path string, v any) error {
	mu.RLock()
	defer mu.RUnlock()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}

func write(path string, v any) error {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func LoadConfig(path string) (Config, error) {
	var c Config
	if err := read(path, &c); err != nil {
		return c, err
	}
	// 解密敏感字段（旧明文无 "ENC:" 前缀则直接返回，下次保存自动加密）
	key, err := crypto.Decrypt(c.Chanjing.SecretKey)
	if err != nil {
		return Config{}, fmt.Errorf("解密蝉镜 SecretKey 失败: %w", err)
	}
	c.Chanjing.SecretKey = key

	key, err = crypto.Decrypt(c.LLM.APIKey)
	if err != nil {
		return Config{}, fmt.Errorf("解密 LLM API Key 失败: %w", err)
	}
	c.LLM.APIKey = key
	return c, nil
}

func SaveConfig(path string, c Config) error {
	// 加密敏感字段后再写盘
	if encrypted, err := crypto.Encrypt(c.Chanjing.SecretKey); err == nil {
		c.Chanjing.SecretKey = encrypted
	}
	if encrypted, err := crypto.Encrypt(c.LLM.APIKey); err == nil {
		c.LLM.APIKey = encrypted
	}
	return write(path, c)
}

func LoadTasks(path string) ([]Task, error) {
	var tasks []Task
	if err := read(path, &tasks); err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, err
	}
	return tasks, nil
}

func SaveTasks(path string, tasks []Task) error {
	return write(path, tasks)
}

// PruneExpiredTasks removes terminal records older than cutoff and keeps active jobs.
func PruneExpiredTasks(path string, cutoff time.Time) (int, error) {
	mu.Lock()
	defer mu.Unlock()

	tasks, err := loadTasksUnlocked(path)
	if err != nil {
		return 0, err
	}
	kept := make([]Task, 0, len(tasks))
	removed := 0
	for _, task := range tasks {
		createdAt, parseErr := time.Parse(time.RFC3339, task.CreatedAt)
		if isActiveTask(task) || parseErr != nil || !createdAt.Before(cutoff) {
			kept = append(kept, task)
			continue
		}
		removed++
	}
	if removed == 0 {
		return 0, nil
	}
	return removed, saveTasksUnlocked(path, kept)
}

func DeleteTask(path, taskID string) error {
	mu.Lock()
	defer mu.Unlock()

	tasks, err := loadTasksUnlocked(path)
	if err != nil {
		return err
	}
	kept := make([]Task, 0, len(tasks))
	found := false
	for _, task := range tasks {
		if task.TaskID != taskID {
			kept = append(kept, task)
			continue
		}
		found = true
		if isActiveTask(task) {
			return ErrTaskInProgress
		}
	}
	if !found {
		return ErrTaskNotFound
	}
	return saveTasksUnlocked(path, kept)
}

// ClearTerminalTasks clears history without hiding tasks that are still running.
func ClearTerminalTasks(path string) (int, error) {
	mu.Lock()
	defer mu.Unlock()

	tasks, err := loadTasksUnlocked(path)
	if err != nil {
		return 0, err
	}
	kept := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if isActiveTask(task) {
			kept = append(kept, task)
		}
	}
	removed := len(tasks) - len(kept)
	if removed == 0 {
		return 0, nil
	}
	return removed, saveTasksUnlocked(path, kept)
}

func isActiveTask(task Task) bool {
	return task.Status == "pending" || task.Status == "generating"
}

func loadTasksUnlocked(path string) ([]Task, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var tasks []Task
	if err := json.NewDecoder(f).Decode(&tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func saveTasksUnlocked(path string, tasks []Task) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(tasks)
}

// UpdateTask 原子地读取、修改、写回单个任务，避免并发覆盖
func UpdateTask(path string, taskID string, fn func(*Task)) error {
	mu.Lock()
	defer mu.Unlock()

	var tasks []Task
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := json.NewDecoder(f).Decode(&tasks); err != nil {
		f.Close()
		return err
	}
	f.Close()

	found := false
	for i := range tasks {
		if tasks[i].TaskID == taskID {
			fn(&tasks[i])
			found = true
			break
		}
	}
	if !found {
		return nil // 任务可能已被删除，静默跳过
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(tasks)
}

func LoadAvatars(path string) ([]Avatar, error) {
	var a []Avatar
	if err := read(path, &a); err != nil {
		if os.IsNotExist(err) {
			return []Avatar{}, nil
		}
		return nil, err
	}
	return a, nil
}

func LoadVoices(path string) ([]Voice, error) {
	var v []Voice
	if err := read(path, &v); err != nil {
		if os.IsNotExist(err) {
			return []Voice{}, nil
		}
		return nil, err
	}
	return v, nil
}
