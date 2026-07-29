package chanjing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

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
	c.tokenMu.Unlock()

	body := map[string]string{
		"app_id":     c.AppID,
		"secret_key": c.SecretKey,
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

	var tr tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("get token decode: %w", err)
	}
	if tr.Code != 0 {
		return "", fmt.Errorf("get token failed: code=%d", tr.Code)
	}

	c.tokenMu.Lock()
	c.tokenCache = tr.Data.AccessToken
	c.tokenExpiry = time.Now().Add(tokenCacheDuration)
	c.tokenMu.Unlock()

	return tr.Data.AccessToken, nil
}

// --- Create Video ---

type PersonParams struct {
	ID         string `json:"id"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FigureType string `json:"figure_type"`
	DriveMode  string `json:"drive_mode"`
	Backway    int    `json:"backway"`
}

type TTSParams struct {
	Text     []string `json:"text"`
	Speed    float64  `json:"speed"`
	AudioMan string   `json:"audio_man"`
}

type AudioParams struct {
	TTS      TTSParams `json:"tts"`
	WavURL   string    `json:"wav_url"`
	Type     string    `json:"type"`
	Volume   int       `json:"volume"`
	Language string    `json:"language"`
}

type SubtitleConfig struct {
	Show bool `json:"show"`
}

type CreateVideoRequest struct {
	Person                 PersonParams   `json:"person"`
	Audio                  AudioParams    `json:"audio"`
	BgColor                string         `json:"bg_color"`
	ScreenWidth            int            `json:"screen_width"`
	ScreenHeight           int            `json:"screen_height"`
	SubtitleConfig         SubtitleConfig `json:"subtitle_config"`
	AddComplianceWatermark bool           `json:"add_compliance_watermark"`
	Source                 int            `json:"source"`
}

type createVideoResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
}

func (c *Client) CreateVideo(token string, req CreateVideoRequest) (string, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("create video marshal: %w", err)
	}

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
	var cr createVideoResp
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("create video decode: %w (body=%s)", err, string(body))
	}
	if cr.Code != 0 {
		return "", fmt.Errorf("create video failed: code=%d, msg=%s", cr.Code, cr.Msg)
	}
	return cr.Data, nil
}

// --- Get Video Status ---

type VideoStatus struct {
	Status   int    `json:"status"` // 0=排队 10=生成中 20=成功 30=失败
	Progress int    `json:"progress"`
	VideoURL string `json:"video_url"`
	Msg      string `json:"msg"`
	Duration int    `json:"duration"`
}

type videoStatusResp struct {
	Code int         `json:"code"`
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

	var vs videoStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&vs); err != nil {
		return VideoStatus{}, fmt.Errorf("get video status decode: %w", err)
	}
	if vs.Code != 0 {
		return VideoStatus{}, fmt.Errorf("get video status failed: code=%d", vs.Code)
	}
	return vs.Data, nil
}
