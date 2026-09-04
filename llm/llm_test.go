package llm

import (
	"context"
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

func TestResponsesJSONContextUsesBuiltInWebSearch(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		tools, ok := request["tools"].([]any)
		if !ok || len(tools) != 1 || tools[0].(map[string]any)["type"] != "web_search" {
			t.Fatalf("unexpected tools: %#v", request["tools"])
		}
		choice, ok := request["tool_choice"].(map[string]any)
		if !ok || choice["type"] != "web_search" {
			t.Fatalf("unexpected tool_choice: %#v", request["tool_choice"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"output": []any{
				map[string]any{
					"type":   "web_search_call",
					"status": "completed",
					"action": map[string]string{"type": "open_page", "url": ts.URL + "/career"},
				},
				map[string]any{"type": "message", "content": []any{
					map[string]any{"type": "output_text", "text": `{"results":[]}`},
				}},
			},
		})
	}))
	defer ts.Close()

	result, err := ResponsesJSONContext(context.Background(), ts.URL, "test-key", "deepseek-v4-flash", "use web_search")
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != `{"results":[]}` || result.WebSearchCallCount != 1 || len(result.WebSearchURLs) != 1 || result.WebSearchURLs[0] != ts.URL+"/career" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
