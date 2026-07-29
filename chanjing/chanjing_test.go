package chanjing

import (
	"encoding/json"
	"testing"
)

func TestCreateVideoRequest_JSON(t *testing.T) {
	req := CreateVideoRequest{
		Person: PersonParams{
			ID:         "C-test",
			X:          0, Y: 0,
			Width:      1080, Height: 1920,
			FigureType: "stand_body",
			DriveMode:  "random",
			Backway:    1,
		},
		Audio: AudioParams{
			TTS:      TTSParams{Text: []string{"测试稿子"}, Speed: 1.0, AudioMan: "C-voice"},
			WavURL:   "",
			Type:     "tts",
			Volume:   100,
			Language: "cn",
		},
		BgColor:        "#ffffff",
		ScreenWidth:    1080,
		ScreenHeight:   1920,
		SubtitleConfig: SubtitleConfig{Show: true},
		Source:         1,
	}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	json.Unmarshal(b, &parsed)

	if parsed["source"].(float64) != 1 {
		t.Error("source should be 1")
	}
	if parsed["bg_color"] != "#ffffff" {
		t.Error("bg_color mismatch")
	}
	t.Logf("Request JSON:\n%s", string(b))
}

func TestVideoStatus_Struct(t *testing.T) {
	jsonStr := `{"code":0,"data":{"status":20,"progress":100,"video_url":"https://res.chanjing.cc/v.mp4","msg":"success","duration":58000}}`

	var resp videoStatusResp
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Status != 20 {
		t.Errorf("status = %d, want 20", resp.Data.Status)
	}
	if resp.Data.VideoURL != "https://res.chanjing.cc/v.mp4" {
		t.Errorf("video_url mismatch")
	}
}
