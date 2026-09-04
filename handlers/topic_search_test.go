package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"koubo-video-tool/topicsearch"
)

func TestTopicSearchEndpointUsesStructuredSearchService(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"llm":{"api_url":"http://llm.test","api_key":"key","model":"model"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	h := New(dir, nil, nil)
	h.TopicSearch = func(_ context.Context, query, apiURL, apiKey, model string) (topicsearch.Response, error) {
		if query != "香港外企" || apiURL != "http://llm.test" || apiKey != "key" || model != "model" {
			t.Fatalf("unexpected search arguments: %q %q %q %q", query, apiURL, apiKey, model)
		}
		return topicsearch.Response{Query: query, TotalFound: 1, Results: []topicsearch.Result{{
			ID: "topic_1", Type: "campus_recruitment", Title: "香港某外企 2027 校招", SourceURL: "https://example.com/career",
		}}}, nil
	}
	req := httptest.NewRequest("POST", "/api/topic-search", strings.NewReader(`{"query":"香港外企"}`))
	rec := httptest.NewRecorder()
	h.handleTopicSearch(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	var response topicsearch.Response
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].SourceURL == "" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestTopicSearchEndpointExplainsSearchFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"llm":{"api_url":"http://llm.test","api_key":"key","model":"model"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	h := New(dir, nil, nil)
	h.TopicSearch = func(context.Context, string, string, string, string) (topicsearch.Response, error) {
		return topicsearch.Response{}, fmt.Errorf("搜索服务没有返回可用结果")
	}
	req := httptest.NewRequest("POST", "/api/topic-search", strings.NewReader(`{"query":"招聘会"}`))
	rec := httptest.NewRecorder()
	h.handleTopicSearch(rec, req)
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "本地联网搜索没有返回可用结果") {
		t.Fatalf("unexpected error response: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTopicsEndpointPersistsAndDeduplicatesByURL(t *testing.T) {
	dir := t.TempDir()
	h := New(dir, nil, nil)
	body := `{"title":"某企业 2027 校招","type":"campus_recruitment","source_url":"https://example.com/career","raw_info":{"deadline":"2026-10-01"}}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/topics", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.handleTopics(rec, req)
		if rec.Code != 200 {
			t.Fatalf("save %d status = %d; body = %s", i, rec.Code, rec.Body.String())
		}
	}
	topics, err := os.ReadFile(filepath.Join(dir, "topics.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved []map[string]any
	if err := json.Unmarshal(topics, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 {
		t.Fatalf("saved topics = %d, want 1", len(saved))
	}
}
