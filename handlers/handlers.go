package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"koubo-video-tool/chanjing"
	"koubo-video-tool/llm"
	"koubo-video-tool/scheduler"
	"koubo-video-tool/skill"
	"koubo-video-tool/store"
	"koubo-video-tool/topicsearch"
)

type Handler struct {
	DataDir               string
	Store                 *StorePaths
	Chanjing              chanjing.ClientInterface
	Scheduler             *scheduler.Scheduler
	TopicSearch           TopicSearchFunc
	TopicSearchWithConfig TopicSearchConfigFunc
}

// TopicSearchFunc is injectable for tests and keeps the HTTP layer independent
// from the concrete web-search provider.
type TopicSearchFunc func(ctx context.Context, query, apiURL, apiKey, model string) (topicsearch.Response, error)

// TopicSearchConfigFunc is the production path. It passes the saved search
// provider credentials to the Agent without coupling tests to config storage.
type TopicSearchConfigFunc func(ctx context.Context, query string, cfg store.Config) (topicsearch.Response, error)

type StorePaths struct {
	Config  string
	Tasks   string
	Avatars string
	Voices  string
	Topics  string
}

func New(dataDir string, cj chanjing.ClientInterface, sch *scheduler.Scheduler) *Handler {
	searchService := topicsearch.NewService()
	return &Handler{
		DataDir:   dataDir,
		Chanjing:  cj,
		Scheduler: sch,
		Store: &StorePaths{
			Config:  dataDir + "/config.json",
			Tasks:   dataDir + "/tasks.json",
			Avatars: dataDir + "/avatars.json",
			Voices:  dataDir + "/voices.json",
			Topics:  dataDir + "/topics.json",
		},
		TopicSearchWithConfig: searchService.SearchWithConfigFromStore,
	}
}

func (h *Handler) Register() {
	http.HandleFunc("/api/avatars", h.handleAvatars)
	http.HandleFunc("/api/voices", h.handleVoices)
	http.HandleFunc("/api/scripts/generate", h.handleGenerateScript)
	http.HandleFunc("/api/topic-search", h.handleTopicSearch)
	http.HandleFunc("/api/topics", h.handleTopics)
	http.HandleFunc("/api/tasks", h.handleTasks)
	http.HandleFunc("/api/tasks/", h.handleTaskByID)
	http.HandleFunc("/api/settings", h.handleSettings)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	writeJSON(w, map[string]string{"error": msg})
}

// GET /api/avatars
func (h *Handler) handleAvatars(w http.ResponseWriter, r *http.Request) {
	avatars, err := store.LoadAvatars(h.Store.Avatars)
	if err != nil {
		if !os.IsNotExist(err) {
			writeError(w, 500, "读取形象列表失败")
			return
		}
		avatars = []store.Avatar{}
	}

	// 每次读取时从蝉镜同步主站上传的定制数字人。同步失败时保留本地
	// 缓存，避免临时网络或认证问题导致主界面无法选择已有形象。
	if lister, ok := h.Chanjing.(chanjing.CustomisedPersonLister); ok {
		token, tokenErr := h.Chanjing.GetToken()
		if tokenErr == nil {
			remote, listErr := lister.ListCustomisedPersons(token, 1)
			if listErr != nil && strings.Contains(listErr.Error(), "10400") {
				h.Chanjing.InvalidateTokenCache()
				if token, tokenErr = h.Chanjing.GetToken(); tokenErr == nil {
					remote, listErr = lister.ListCustomisedPersons(token, 1)
				}
			}
			if listErr == nil {
				merged := mergeRemoteAvatars(avatars, remote)
				if err := store.SaveAvatars(h.Store.Avatars, merged); err == nil {
					avatars = merged
				}
			}
		}
	}
	writeJSON(w, avatars)
}

func mergeRemoteAvatars(local []store.Avatar, remote []chanjing.CustomisedPerson) []store.Avatar {
	merged := make([]store.Avatar, 0, len(local)+len(remote))
	indices := make(map[string]int, len(local)+len(remote))
	for _, avatar := range local {
		if strings.TrimSpace(avatar.ID) == "" {
			continue
		}
		indices[avatar.ID] = len(merged)
		merged = append(merged, avatar)
	}
	for _, person := range remote {
		if strings.TrimSpace(person.ID) == "" || person.Status != 2 || person.IsOpen == 0 {
			continue
		}
		preview := strings.TrimSpace(person.PicURL)
		if preview == "" {
			preview = strings.TrimSpace(person.PreviewURL)
		}
		avatar := store.Avatar{ID: person.ID, Name: person.Name, PreviewURL: preview}
		if strings.TrimSpace(avatar.Name) == "" {
			avatar.Name = avatar.ID
		}
		if index, exists := indices[avatar.ID]; exists {
			merged[index] = avatar
		} else {
			indices[avatar.ID] = len(merged)
			merged = append(merged, avatar)
		}
	}
	return merged
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

	prompt, files, err := skill.BuildPrompt(&cfg, req.Topic)
	if err != nil {
		writeError(w, 500, "Skill 源读取失败: "+err.Error())
		return
	}
	if len(files) > 0 {
		log.Printf("skill files loaded: %s", strings.Join(files, ", "))
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

// POST /api/topic-search
func (h *Handler) handleTopicSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		writeError(w, http.StatusBadRequest, "搜索词不能为空")
		return
	}
	cfg, err := store.LoadConfig(h.Store.Config)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取配置失败")
		return
	}
	if strings.TrimSpace(cfg.LLM.APIURL) == "" || strings.TrimSpace(cfg.LLM.APIKey) == "" {
		writeError(w, http.StatusBadRequest, "大模型未配置，请在设置中配置 API 地址和密钥")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Second)
	defer cancel()
	var result topicsearch.Response
	if h.TopicSearch != nil {
		result, err = h.TopicSearch(ctx, query, cfg.LLM.APIURL, cfg.LLM.APIKey, cfg.LLM.Model)
	} else if h.TopicSearchWithConfig != nil {
		result, err = h.TopicSearchWithConfig(ctx, query, cfg)
	} else {
		result, err = topicsearch.NewService().SearchWithConfigFromStore(ctx, query, cfg)
	}
	if err != nil {
		log.Printf("topic search failed for %q: %v", query, err)
		writeError(w, http.StatusBadGateway, topicSearchErrorMessage(err))
		return
	}
	if result.Results == nil {
		result.Results = []topicsearch.Result{}
	}
	writeJSON(w, result)
}

func topicSearchErrorMessage(err error) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "联网搜索工具未配置") || strings.Contains(message, "未配置 API Key"):
		return "搜索服务未配置，请使用本地联网搜索，或选择 Tavily/Brave 并填写对应 API Key"
	case strings.Contains(message, "AI 整理失败"):
		return "已调用 AI，但搜索结果整理失败，请检查大模型配置后重试"
	case strings.Contains(message, "web_search 未执行"):
		return "DeepSeek 内置搜索没有返回可验证的搜索调用，请重试或检查模型配置"
	case strings.Contains(message, "Responses 请求失败") || strings.Contains(message, "context deadline exceeded"):
		return "DeepSeek 搜索请求超时或失败，请稍后重试"
	case strings.Contains(message, "搜索服务没有返回"):
		return "本地联网搜索没有返回可用结果，请更换关键词后重试"
	default:
		return "搜索失败，请稍后重试"
	}
}

// GET /api/topics, POST /api/topics
func (h *Handler) handleTopics(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		topics, err := store.LoadTopics(h.Store.Topics)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "读取选题库失败")
			return
		}
		writeJSON(w, map[string]any{"topics": topics})
	case http.MethodPost:
		var req struct {
			Title     string             `json:"title"`
			Type      string             `json:"type"`
			SourceURL string             `json:"source_url"`
			RawInfo   topicsearch.Result `json:"raw_info"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 128<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "请求格式错误")
			return
		}
		parsedURL, urlErr := url.Parse(strings.TrimSpace(req.SourceURL))
		validURL := urlErr == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") && parsedURL.Host != ""
		if strings.TrimSpace(req.Title) == "" || !validURL {
			writeError(w, http.StatusBadRequest, "选题标题不能为空，原文链接必须是有效的 HTTP(S) 地址")
			return
		}
		topic := store.SavedTopic{
			Title: req.Title, Type: req.Type, SourceURL: req.SourceURL,
			RawInfo: req.RawInfo, CreatedAt: time.Now().UTC().Format(time.RFC3339), Status: "saved",
		}
		saved, added, err := store.AddTopic(h.Store.Topics, topic)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "保存选题失败")
			return
		}
		writeJSON(w, map[string]any{"saved": added, "topic": saved})
	default:
		writeError(w, http.StatusMethodNotAllowed, "仅支持 GET / POST")
	}
}

const taskRetention = 30 * 24 * time.Hour

// POST /api/tasks, GET /api/tasks, DELETE /api/tasks
func (h *Handler) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		h.createTask(w, r)
	case "GET":
		_, _ = store.PruneExpiredTasks(h.Store.Tasks, time.Now().UTC().Add(-taskRetention))
		tasks, err := store.LoadTasks(h.Store.Tasks)
		if err != nil {
			writeJSON(w, map[string]any{"tasks": []store.Task{}})
			return
		}
		writeJSON(w, map[string]any{"tasks": tasks})
	case "DELETE":
		removed, err := store.ClearTerminalTasks(h.Store.Tasks)
		if err != nil {
			writeError(w, 500, "清空任务记录失败")
			return
		}
		writeJSON(w, map[string]any{"removed": removed})
	default:
		writeError(w, 405, "仅支持 POST / GET / DELETE")
	}
}

// GET /api/tasks/:id, POST /api/tasks/:id/retry, POST /api/tasks/:id/refresh
func (h *Handler) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if path == "" {
		writeError(w, 400, "缺少 task_id")
		return
	}

	// POST /api/tasks/:id/retry
	if strings.HasSuffix(path, "/retry") && r.Method == "POST" {
		taskID := strings.TrimSuffix(path, "/retry")
		h.retryTask(w, r, taskID)
		return
	}
	if strings.HasSuffix(path, "/refresh") && r.Method == "POST" {
		taskID := strings.TrimSuffix(path, "/refresh")
		h.refreshTask(w, r, taskID)
		return
	}
	if r.Method == "DELETE" {
		err := store.DeleteTask(h.Store.Tasks, path)
		switch {
		case errors.Is(err, store.ErrTaskNotFound):
			writeError(w, 404, err.Error())
		case errors.Is(err, store.ErrTaskInProgress):
			writeError(w, 409, err.Error())
		case err != nil:
			writeError(w, 500, "删除任务记录失败")
		default:
			writeJSON(w, map[string]bool{"deleted": true})
		}
		return
	}

	taskID := path
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

func (h *Handler) refreshTask(w http.ResponseWriter, r *http.Request, taskID string) {
	if h.Scheduler == nil {
		writeError(w, 503, "浠诲姟鍒锋柊鍔熻兘涓嶅彲鐢?")
		return
	}
	task, err := h.Scheduler.Refresh(taskID)
	if err != nil {
		writeError(w, 503, "鍒锋柊澶辫触: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"status": "ok",
		"task":   task,
	})
}

func (h *Handler) retryTask(w http.ResponseWriter, r *http.Request, taskID string) {
	tasks, err := store.LoadTasks(h.Store.Tasks)
	if err != nil {
		writeError(w, 500, "读取任务列表失败")
		return
	}

	var oldTask *store.Task
	for i := range tasks {
		if tasks[i].TaskID == taskID {
			oldTask = &tasks[i]
			break
		}
	}
	if oldTask == nil {
		writeError(w, 404, "任务不存在")
		return
	}

	// Params 经 JSON 存取后变成 map[string]any，序列化回来再反序列化回结构体
	paramsBytes, err := json.Marshal(oldTask.Params)
	if err != nil {
		writeError(w, 500, "解析任务参数失败")
		return
	}
	var cjReq chanjing.CreateVideoRequest
	if err := json.Unmarshal(paramsBytes, &cjReq); err != nil {
		writeError(w, 500, "还原任务参数失败: "+err.Error())
		return
	}
	if cjReq.AudioSource == 0 {
		audioSource, err := h.voiceAudioSource(cjReq.Audio.TTS.AudioMan)
		if err != nil {
			writeError(w, 500, "读取人声配置失败")
			return
		}
		cjReq.AudioSource = audioSource
	}

	token, err := h.Chanjing.GetToken()
	if err != nil {
		writeError(w, 503, "蝉镜认证失败: "+err.Error())
		return
	}

	newTaskID, err := h.Chanjing.CreateVideo(token, cjReq)
	if err != nil {
		writeError(w, 503, "重试失败: "+err.Error())
		return
	}

	newTask := store.Task{
		TaskID:    newTaskID,
		Status:    "pending",
		Script:    oldTask.Script,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Params:    cjReq,
	}
	if newTask.Script == "" && len(cjReq.Audio.TTS.Text) > 0 {
		newTask.Script = cjReq.Audio.TTS.Text[0]
	}

	tasks = append(tasks, newTask)
	if err := store.SaveTasks(h.Store.Tasks, tasks); err != nil {
		writeError(w, 500, "保存任务列表失败")
		return
	}

	h.Scheduler.Add(newTaskID, token)

	writeJSON(w, map[string]string{
		"task_id": newTaskID,
		"status":  "pending",
	})
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

	// defaults — 对齐 n8n 工作流
	if req.Speed == 0 {
		req.Speed = 1.0
	}
	if req.Volume == 0 {
		req.Volume = 100
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
	if req.BgColor == "" {
		req.BgColor = "#ffffff"
	}

	audioSource, err := h.voiceAudioSource(req.AudioManID)
	if err != nil {
		writeError(w, 500, "读取人声配置失败")
		return
	}

	scw, sch := 1080, 1920
	if req.Resolution == "720P" {
		scw, sch = 720, 1280
	}

	cjReq := chanjing.CreateVideoRequest{
		Person: chanjing.PersonConfig{
			ID:         req.AvatarID,
			X:          0,
			Y:          0,
			Width:      scw,
			Height:     sch,
			FigureType: req.FigureType,
			DriveMode:  req.DriveMode,
			Backway:    req.Backway,
		},
		Audio: chanjing.AudioConfig{
			TTS: chanjing.TTSConfig{
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
		AudioSource:            audioSource,
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
		Script:    req.Script,
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

func (h *Handler) voiceAudioSource(voiceID string) (int, error) {
	voices, err := store.LoadVoices(h.Store.Voices)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	for _, voice := range voices {
		if voice.ID == voiceID {
			return voice.AudioSource, nil
		}
	}
	return 0, nil
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
		if cfg.Search.APIKey != "" {
			cfg.Search.APIKey = store.MaskedValue
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
		if cfg.Search.APIKey == store.MaskedValue {
			old, err := store.LoadConfig(h.Store.Config)
			if err == nil {
				cfg.Search.APIKey = old.Search.APIKey
			}
		}
		if strings.TrimSpace(cfg.Chanjing.AppID) == "" || strings.TrimSpace(cfg.Chanjing.SecretKey) == "" {
			writeError(w, 400, "蝉镜 AppID 和 SecretKey 不能为空")
			return
		}
		if err := store.SaveConfig(h.Store.Config, cfg); err != nil {
			writeError(w, 500, "保存设置失败")
			return
		}
		h.Chanjing.Configure(cfg.Chanjing.AppID, cfg.Chanjing.SecretKey)
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		writeError(w, 405, "仅支持 GET / PUT")
	}
}
