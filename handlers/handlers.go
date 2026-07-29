package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"koubo-video-tool/chanjing"
	"koubo-video-tool/llm"
	"koubo-video-tool/scheduler"
	"koubo-video-tool/skill"
	"koubo-video-tool/store"
)

type Handler struct {
	DataDir   string
	Store     *StorePaths
	Chanjing  *chanjing.Client
	Scheduler *scheduler.Scheduler
}

type StorePaths struct {
	Config  string
	Tasks   string
	Avatars string
	Voices  string
}

func New(dataDir string, cj *chanjing.Client, sch *scheduler.Scheduler) *Handler {
	return &Handler{
		DataDir:   dataDir,
		Chanjing:  cj,
		Scheduler: sch,
		Store: &StorePaths{
			Config:  dataDir + "/config.json",
			Tasks:   dataDir + "/tasks.json",
			Avatars: dataDir + "/avatars.json",
			Voices:  dataDir + "/voices.json",
		},
	}
}

func (h *Handler) Register() {
	http.HandleFunc("/api/avatars", h.handleAvatars)
	http.HandleFunc("/api/voices", h.handleVoices)
	http.HandleFunc("/api/scripts/generate", h.handleGenerateScript)
	http.HandleFunc("/api/tasks", h.handleTasks)
	http.HandleFunc("/api/tasks/", h.handleTaskByID)
	http.HandleFunc("/api/settings", h.handleSettings)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	writeJSON(w, map[string]string{"error": msg})
}

// GET /api/avatars
func (h *Handler) handleAvatars(w http.ResponseWriter, r *http.Request) {
	avatars, err := store.LoadAvatars(h.Store.Avatars)
	if err != nil {
		writeError(w, 500, "读取形象列表失败")
		return
	}
	writeJSON(w, avatars)
}

// GET /api/voices
func (h *Handler) handleVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := store.LoadVoices(h.Store.Voices)
	if err != nil {
		writeError(w, 500, "读取人声列表失败")
		return
	}
	writeJSON(w, voices)
}

// POST /api/scripts/generate
func (h *Handler) handleGenerateScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "仅支持 POST")
		return
	}

	var req struct {
		Topic string `json:"topic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.Topic) == "" {
		writeError(w, 400, "话题不能为空")
		return
	}

	cfg, err := store.LoadConfig(h.Store.Config)
	if err != nil {
		writeError(w, 500, "读取配置失败")
		return
	}

	if cfg.LLM.APIURL == "" || cfg.LLM.APIKey == "" {
		writeError(w, 400, "大模型未配置，请在设置中配置 API 地址和密钥")
		return
	}

	prompt, err := skill.BuildPrompt(&cfg)
	if err != nil {
		writeError(w, 500, "Skill 源读取失败: "+err.Error())
		return
	}

	// GitHub 源同步成功后持久化缓存时间
	if cfg.Skill.Source == "github" {
		store.SaveConfig(h.Store.Config, cfg)
	}

	script, err := llm.GenerateScript(cfg.LLM.APIURL, cfg.LLM.APIKey, cfg.LLM.Model, req.Topic, prompt)
	if err != nil {
		writeError(w, 500, "生成失败: "+err.Error())
		return
	}

	writeJSON(w, map[string]string{"script": script})
}

// POST /api/tasks, GET /api/tasks
func (h *Handler) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		h.createTask(w, r)
	case "GET":
		tasks, err := store.LoadTasks(h.Store.Tasks)
		if err != nil {
			writeJSON(w, map[string]any{"tasks": []store.Task{}})
			return
		}
		writeJSON(w, map[string]any{"tasks": tasks})
	default:
		writeError(w, 405, "仅支持 POST / GET")
	}
}

// GET /api/tasks/:id
func (h *Handler) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if taskID == "" {
		writeError(w, 400, "缺少 task_id")
		return
	}

	tasks, err := store.LoadTasks(h.Store.Tasks)
	if err != nil {
		writeError(w, 500, "读取任务列表失败")
		return
	}
	for _, t := range tasks {
		if t.TaskID == taskID {
			writeJSON(w, t)
			return
		}
	}
	writeError(w, 404, "任务不存在")
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Script              string  `json:"script"`
		AvatarID            string  `json:"avatar_id"`
		AudioManID          string  `json:"audio_man_id"`
		Speed               float64 `json:"speed"`
		Volume              int     `json:"volume"`
		BgColor             string  `json:"bg_color"`
		Resolution          string  `json:"resolution"`
		SubtitleEnabled     bool    `json:"subtitle_enabled"`
		FigureType          string  `json:"figure_type"`
		DriveMode           string  `json:"drive_mode"`
		Backway             int     `json:"backway"`
		Language            string  `json:"language"`
		ComplianceWatermark bool    `json:"compliance_watermark"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, 400, "读取请求体失败")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, 400, "请求格式错误: "+err.Error())
		return
	}

	if req.Script == "" {
		writeError(w, 400, "稿件不能为空")
		return
	}

	// defaults
	if req.Speed == 0 {
		req.Speed = 1.0
	}
	if req.Volume == 0 {
		req.Volume = 100
	}
	if req.BgColor == "" {
		req.BgColor = "#ffffff"
	}
	if req.FigureType == "" {
		req.FigureType = "stand_body"
	}
	if req.DriveMode == "" {
		req.DriveMode = "random"
	}
	if req.Backway == 0 {
		req.Backway = 1
	}
	if req.Language == "" {
		req.Language = "cn"
	}

	scw, sch := 1080, 1920
	if req.Resolution == "720P" {
		scw, sch = 720, 1280
	}

	cjReq := chanjing.CreateVideoRequest{
		Person: chanjing.PersonParams{
			ID: req.AvatarID, X: 0, Y: 0,
			Width: scw, Height: sch,
			FigureType: req.FigureType,
			DriveMode:  req.DriveMode,
			Backway:    req.Backway,
		},
		Audio: chanjing.AudioParams{
			TTS: chanjing.TTSParams{
				Text:     []string{req.Script},
				Speed:    req.Speed,
				AudioMan: req.AudioManID,
			},
			WavURL:   "",
			Type:     "tts",
			Volume:   req.Volume,
			Language: req.Language,
		},
		BgColor:                req.BgColor,
		ScreenWidth:            scw,
		ScreenHeight:           sch,
		SubtitleConfig:         chanjing.SubtitleConfig{Show: req.SubtitleEnabled},
		AddComplianceWatermark: req.ComplianceWatermark,
		Source:                 1,
	}

	token, err := h.Chanjing.GetToken()
	if err != nil {
		writeError(w, 503, "蝉镜认证失败: "+err.Error())
		return
	}

	taskID, err := h.Chanjing.CreateVideo(token, cjReq)
	if err != nil {
		writeError(w, 503, "创建视频任务失败: "+err.Error())
		return
	}

	newTask := store.Task{
		TaskID:    taskID,
		Status:    "pending",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Params:    cjReq,
	}

	tasks, err := store.LoadTasks(h.Store.Tasks)
	if err != nil {
		writeError(w, 500, "读取任务列表失败")
		return
	}
	tasks = append(tasks, newTask)
	if err := store.SaveTasks(h.Store.Tasks, tasks); err != nil {
		writeError(w, 500, "保存任务列表失败")
		return
	}

	h.Scheduler.Add(taskID, token)

	writeJSON(w, map[string]string{
		"task_id": taskID,
		"status":  "pending",
	})
}

// GET /api/settings, PUT /api/settings
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		cfg, err := store.LoadConfig(h.Store.Config)
		if err != nil {
			writeJSON(w, store.Config{})
			return
		}
		if cfg.Chanjing.SecretKey != "" {
			cfg.Chanjing.SecretKey = store.MaskedValue
		}
		if cfg.LLM.APIKey != "" {
			cfg.LLM.APIKey = store.MaskedValue
		}
		writeJSON(w, cfg)

	case "PUT":
		var cfg store.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, 400, "请求格式错误")
			return
		}
		if cfg.Chanjing.SecretKey == store.MaskedValue {
			old, err := store.LoadConfig(h.Store.Config)
			if err == nil {
				cfg.Chanjing.SecretKey = old.Chanjing.SecretKey
			}
		}
		if cfg.LLM.APIKey == store.MaskedValue {
			old, err := store.LoadConfig(h.Store.Config)
			if err == nil {
				cfg.LLM.APIKey = old.LLM.APIKey
			}
		}
		if err := store.SaveConfig(h.Store.Config, cfg); err != nil {
			writeError(w, 500, "保存设置失败")
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		writeError(w, 405, "仅支持 GET / PUT")
	}
}
