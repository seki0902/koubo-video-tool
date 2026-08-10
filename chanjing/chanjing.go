package chanjing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ClientInterface 定义蝉镜视频操作接口，便于测试时 mock
type ClientInterface interface {
	GetToken() (string, error)
	CreateVideo(token string, req CreateVideoRequest) (string, error)
	GetVideoStatus(token, taskID string) (VideoStatus, error)
	Configure(appID, secretKey string)
	InvalidateTokenCache()
}

const BaseURL = "https://open-api.chanjing.cc/open/v1"

const tokenCacheDuration = 50 * time.Minute

type Client struct {
	AppID       string
	SecretKey   string
	HTTPClient  *http.Client
	tokenMu     sync.Mutex
	tokenCache  string
	tokenExpiry time.Time
}

func NewClient(appID, secretKey string) *Client {
	return &Client{
		AppID:     appID,
		SecretKey: secretKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Configure(appID, secretKey string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.AppID = appID
	c.SecretKey = secretKey
	c.tokenCache = ""
	c.tokenExpiry = time.Time{}
}

// InvalidateTokenCache 清空本地 token 缓存，便于轮询时强制重新取 token。
func (c *Client) InvalidateTokenCache() {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.tokenCache = ""
	c.tokenExpiry = time.Time{}
}

// --- Token ---

type tokenResp struct {
	Code int `json:"code"`
	Data struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

func (c *Client) GetToken() (string, error) {
	c.tokenMu.Lock()
	if time.Now().Before(c.tokenExpiry) && c.tokenCache != "" {
		tok := c.tokenCache
		c.tokenMu.Unlock()
		return tok, nil
	}
	appID, secretKey := c.AppID, c.SecretKey
	c.tokenMu.Unlock()
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(secretKey) == "" {
		return "", fmt.Errorf("蝉镜 AppID 或 SecretKey 未配置")
	}

	body := map[string]string{
		"app_id":     appID,
		"secret_key": secretKey,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("get token marshal: %w", err)
	}

	resp, err := c.HTTPClient.Post(BaseURL+"/access_token", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("get token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var tr tokenResp
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", fmt.Errorf("get token decode: %w (body=%s)", err, string(respBody))
	}
	if tr.Code != 0 {
		return "", fmt.Errorf("get token failed: code=%d, body=%s", tr.Code, string(respBody))
	}

	c.tokenMu.Lock()
	c.tokenCache = tr.Data.AccessToken
	c.tokenExpiry = time.Now().Add(tokenCacheDuration)
	c.tokenMu.Unlock()

	return tr.Data.AccessToken, nil
}

// --- Create Video ---
// 端点: POST /open/v1/create_video
// 结构与 n8n 工作流已验证的格式保持一致

type PersonConfig struct {
	ID         string `json:"id"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FigureType string `json:"figure_type"`
	DriveMode  string `json:"drive_mode"`
	Backway    int    `json:"backway"`
}

type TTSConfig struct {
	Text     []string `json:"text"`
	Speed    float64  `json:"speed"`
	AudioMan string   `json:"audio_man"`
}

type AudioConfig struct {
	TTS      TTSConfig `json:"tts"`
	WavURL   string    `json:"wav_url"`
	Type     string    `json:"type"`
	Volume   int       `json:"volume"`
	Language string    `json:"language"`
}

type SubtitleConfig struct {
	Show bool `json:"show"`
}

type CreateVideoRequest struct {
	Person                 PersonConfig   `json:"person"`
	Audio                  AudioConfig    `json:"audio"`
	BgColor                string         `json:"bg_color"`
	ScreenWidth            int            `json:"screen_width"`
	ScreenHeight           int            `json:"screen_height"`
	SubtitleConfig         SubtitleConfig `json:"subtitle_config"`
	AddComplianceWatermark bool           `json:"add_compliance_watermark"`
	Source                 int            `json:"source"`
	AudioSource            int            `json:"audio_source,omitempty"`
}

type createVideoResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"` // 返回视频任务 ID
}

func (c *Client) CreateVideo(token string, req CreateVideoRequest) (string, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("create video marshal: %w", err)
	}

	fmt.Printf("[DEBUG] === 蝉镜 CreateVideo 请求 ===\n")
	fmt.Printf("[DEBUG] URL: %s\n", BaseURL+"/create_video")
	fmt.Printf("[DEBUG] Body: %s\n", string(b))

	httpReq, err := http.NewRequest("POST", BaseURL+"/create_video", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("create video new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("access_token", token)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("create video request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[DEBUG] HTTP %d\n", resp.StatusCode)
	fmt.Printf("[DEBUG] Response: %s\n", string(body))

	var cr createVideoResp
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("create video decode: %w (body=%s)", err, string(body))
	}
	if cr.Code != 0 {
		return "", fmt.Errorf("create video failed: code=%d, msg=%s, body=%s", cr.Code, cr.Msg, string(body))
	}
	return cr.Data, nil
}

// --- Get Video Status ---
// 端点: GET /open/v1/video?id={taskID}
// 结构与 n8n 工作流已验证的格式保持一致

type VideoStatus struct {
	Status      int    `json:"status"` // 10=生成中，30=成功，4X=参数异常，5X=服务异常
	Progress    int    `json:"progress"`
	VideoURL    string `json:"video_url"`
	Msg         string `json:"msg"`
	Duration    int    `json:"duration"`
	ID          string `json:"id"`
	QueueStatus string `json:"queue_status"`
	QueueDesc   string `json:"queue_desc"`
}

type videoStatusResp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data VideoStatus `json:"data"`
}

func (c *Client) GetVideoStatus(token, taskID string) (VideoStatus, error) {
	url := fmt.Sprintf("%s/video?id=%s", BaseURL, taskID)
	httpReq, _ := http.NewRequest("GET", url, nil)
	httpReq.Header.Set("access_token", token)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return VideoStatus{}, fmt.Errorf("get video status request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var vs videoStatusResp
	if err := json.Unmarshal(body, &vs); err != nil {
		return VideoStatus{}, fmt.Errorf("get video status decode: %w (body=%s)", err, string(body))
	}
	if vs.Code != 0 {
		return VideoStatus{}, fmt.Errorf("get video status failed: code=%d, msg=%s, body=%s", vs.Code, vs.Msg, string(body))
	}
	return vs.Data, nil
}
