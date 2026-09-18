package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeSearchContinuationAndEvidence(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/anthropic/v1/messages" || r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("incorrect endpoint/auth")
		}
		var req struct {
			Tools []struct {
				Type string `json:"type"`
			}
			Messages []struct {
				Content json.RawMessage `json:"content"`
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if len(req.Tools) != 1 || req.Tools[0].Type != "web_search_20250305" {
			t.Error("missing native tool")
		}
		if calls == 1 {
			w.Write([]byte(`{"content":[{"type":"text","text":"searching"},{"type":"server_tool_use","id":"s1","name":"web_search"},{"type":"web_search_tool_result","tool_use_id":"s1","content":[{"type":"web_search_result","url":"https://example.com/job","encrypted_content":"opaque"}]}],"stop_reason":"pause_turn"}`))
		} else {
			if len(req.Messages) != 2 || !strings.Contains(string(req.Messages[1].Content), "opaque") {
				t.Error("lost server-side search continuation")
			}
			w.Write([]byte(`{"content":[{"type":"text","text":"{\"results\":[]}"}],"stop_reason":"end_turn"}`))
		}
	}))
	defer server.Close()
	result, err := DeepSeekSearchJSONContext(context.Background(), server.URL, "test-key", "model", "search")
	if err != nil || result.Content != `{"results":[]}` || result.WebSearchCallCount != 1 || len(result.WebSearchURLs) != 1 || calls != 2 {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
	}
}

func TestNativeSearchRejectsFailures(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"no search", `{"content":[{"type":"text","text":"{\"results\":[]}"}],"stop_reason":"end_turn"}`, "web_search 未执行"},
		{"tool error", `{"content":[{"type":"server_tool_use","name":"web_search"},{"type":"web_search_tool_result","content":{"type":"web_search_tool_result_error","error_code":"unavailable"}}],"stop_reason":"end_turn"}`, "unavailable"},
		{"truncated", `{"content":[],"stop_reason":"max_tokens"}`, "未完成"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tt.body)) }))
			defer s.Close()
			_, err := DeepSeekSearchJSONContext(context.Background(), s.URL, "key", "model", "search")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
