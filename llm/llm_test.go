package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateScript(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message message `json:"message"`
			}{{Message: message{Role: "assistant", Content: "这是生成的稿件"}}},
		})
	}))
	defer ts.Close()

	result, err := GenerateScript(ts.URL, "test-key", "test-model", "AI的未来", "你是一个口播稿写手")
	if err != nil {
		t.Fatal(err)
	}
	if result != "这是生成的稿件" {
		t.Errorf("result = %q", result)
	}
}

func TestGenerateScript_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(chatResponse{
			Error: &struct {
				Message string `json:"message"`
			}{Message: "rate limit exceeded"},
		})
	}))
	defer ts.Close()

	_, err := GenerateScript(ts.URL, "key", "model", "topic", "system")
	if err == nil {
		t.Error("expected error")
	}
}
