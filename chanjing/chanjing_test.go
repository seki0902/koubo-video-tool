package chanjing

import (
	"encoding/json"
	"testing"
)

func TestCreateVideoRequest_JSON(t *testing.T) {
	req := CreateVideoRequest{
		Person: PersonConfig{
			ID:         "C-6a951f1434884368bc4942f7f74885ff",
			X:          0,
			Y:          0,
			Width:      1080,
			Height:     1920,
			FigureType: "stand_body",
			DriveMode:  "random",
			Backway:    1,
		},
		Audio: AudioConfig{
			TTS: TTSConfig{
				Text:     []string{"测试稿子内容"},
				Speed:    1.2,
				AudioMan: "C-fa39b63eaefa4d3689526a1dfd5a25f3",
			},
			WavURL:   "",
			Type:     "tts",
			Volume:   100,
			Language: "cn",
		},
		BgColor:                "#ffffff",
		ScreenWidth:            1080,
		ScreenHeight:           1920,
		SubtitleConfig:         SubtitleConfig{Show: false},
		AddComplianceWatermark: false,
		Source:                 1,
	}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	json.Unmarshal(b, &parsed)

	// 验证顶层字段
	if parsed["source"].(float64) != 1 {
		t.Error("source mismatch")
	}
	if _, ok := parsed["audio_source"]; ok {
		t.Error("system voice request must omit audio_source")
	}
	if parsed["bg_color"] != "#ffffff" {
		t.Error("bg_color mismatch")
	}

	// 验证 person 嵌套
	person, ok := parsed["person"].(map[string]any)
	if !ok {
		t.Fatal("person missing")
	}
	if person["id"] != "C-6a951f1434884368bc4942f7f74885ff" {
		t.Error("person.id mismatch")
	}
	if person["figure_type"] != "stand_body" {
		t.Error("person.figure_type mismatch")
	}

	// 验证 audio 嵌套
	audio, ok := parsed["audio"].(map[string]any)
	if !ok {
		t.Fatal("audio missing")
	}
	if audio["type"] != "tts" {
		t.Error("audio.type mismatch")
	}

	// 验证 audio.tts 嵌套
	tts, ok := audio["tts"].(map[string]any)
	if !ok {
		t.Fatal("audio.tts missing")
	}
	if tts["audio_man"] != "C-fa39b63eaefa4d3689526a1dfd5a25f3" {
		t.Error("audio_man mismatch")
	}
	if _, ok := tts["source"]; ok {
		t.Error("system voice request must omit source")
	}
	if _, ok := tts["audio_source"]; ok {
		t.Error("system voice request must omit audio_source")
	}
	// text 必须是数组
	textArr, ok := tts["text"].([]any)
	if !ok {
		t.Fatalf("tts.text should be array, got %T: %v", tts["text"], tts["text"])
	}
	if textArr[0].(string) != "测试稿子内容" {
		t.Error("tts.text[0] mismatch")
	}

	t.Logf("Request JSON:\n%s", string(b))
}

func TestCreateVideoRequest_WebVoiceIncludesTopLevelAudioSource(t *testing.T) {
	b, err := json.Marshal(CreateVideoRequest{
		Audio: AudioConfig{TTS: TTSConfig{
			Text:     []string{"测试"},
			Speed:    1,
			AudioMan: "C-web-custom",
		}},
		Source:      1,
		AudioSource: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["source"] != float64(1) || parsed["audio_source"] != float64(1) {
		t.Fatalf("top-level Web resource sources missing: %s", string(b))
	}
	audio := parsed["audio"].(map[string]any)
	tts := audio["tts"].(map[string]any)
	if _, ok := tts["source"]; ok {
		t.Fatalf("source must not be nested under audio.tts: %s", string(b))
	}
	if _, ok := tts["audio_source"]; ok {
		t.Fatalf("audio_source must not be nested under audio.tts: %s", string(b))
	}
}

func TestVideoStatus_Struct(t *testing.T) {
	// 对齐 /open/v1/video?id= 的返回格式
	jsonStr := `{"code":0,"msg":"success","data":{"status":30,"progress":100,"video_url":"https://res.chanjing.cc/v.mp4","msg":"success","duration":58000,"id":"task-123","queue_status":"completed"}}`

	var resp videoStatusResp
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Status != 30 {
		t.Errorf("status = %d, want 30", resp.Data.Status)
	}
	if resp.Data.VideoURL != "https://res.chanjing.cc/v.mp4" {
		t.Errorf("video_url mismatch")
	}
	if resp.Data.QueueStatus != "completed" {
		t.Errorf("queue_status mismatch")
	}
}
