package topicsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSearchRunsStructuredPipelineAndDeduplicatesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			first := "http://" + r.Host + "/page/1"
			second := "http://" + r.Host + "/page/2"
			fmt.Fprintf(w, `<a class="result__a" href="%s">官方招聘会</a><a class="result__snippet">面向应届生的招聘会</a><a class="result__a" href="%s">招聘会转载</a>`, first, second)
		case "/page/1", "/page/2":
			fmt.Fprint(w, `<html><body><h1>上海高校毕业生招聘会</h1><p>2026年10月10日，面向应届毕业生。</p></body></html>`)
		case "/v1/chat/completions":
			var request struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			system := request.Messages[0].Content
			content := `{"intent":"recruitment_event","regions":["上海"],"audience":["应届生"],"time_scope":"recent","result_count":8,"past_search":false}`
			if strings.Contains(system, "Query Expansion") {
				content = `{"queries":["上海 2026 招聘会 应届生","Shanghai graduate recruitment fair"]}`
			}
			if strings.Contains(system, "招聘事实抽取") {
				page1 := "http://" + r.Host + "/page/1"
				page2 := "http://" + r.Host + "/page/2"
				content = fmt.Sprintf(`{"results":[{"type":"recruitment_event","title":"上海高校毕业生招聘会","organization":"上海市就业服务中心","time":"2026年10月10日","location":"上海","target_audience":["应届毕业生"],"application_period":"2026年9月1日至2026年9月30日","source_url":"%s"},{"type":"recruitment_event","title":"上海高校毕业生招聘会","organization":"上海市就业服务中心","time":"2026年10月10日","location":"上海","target_audience":["应届毕业生"],"application_period":"2026年9月1日至2026年9月30日","source_url":"%s"}]}`, page1, page2)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService()
	service.SearchURL = server.URL + "/search"
	service.HTTPClient = server.Client()
	response, err := service.Search(context.Background(), "上海招聘会", server.URL, "key", "model")
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalFound != 2 || len(response.Results) != 1 {
		t.Fatalf("unexpected pipeline response: %+v", response)
	}
	if !strings.HasSuffix(response.Results[0].SourceURL, "/page/1") {
		t.Fatalf("unexpected source URL: %+v", response.Results[0])
	}
	if _, err := url.Parse(response.Results[0].SourceURL); err != nil {
		t.Fatal(err)
	}
}

func TestSearchWithTavilyUsesStructuredProviderAndKeepsAIAsGate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tavily":
			if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
				t.Fatalf("unexpected Tavily headers: %+v", r.Header)
			}
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]string{"title": "某集团 2027 校园招聘", "url": "https://example.com/career", "content": "面向 2027 届应届毕业生，招聘技术和产品岗位。"},
			}})
		case "/v1/chat/completions":
			var request struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			content := `{"intent":"campus_recruitment","regions":[],"company_type":[],"audience":["应届毕业生"],"time_scope":"recent","result_count":8,"past_search":false,"queries":["2027 校园招聘"]}`
			if strings.Contains(request.Messages[0].Content, "招聘事实抽取") {
				content = `{"results":[{"type":"campus_recruitment","title":"某集团 2027 校园招聘","organization":"某集团","target_audience":["应届毕业生"],"positions":["技术","产品"],"deadline":"2026年12月31日","source_url":"https://example.com/career"}]}`
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService()
	service.HTTPClient = server.Client()
	service.TavilyURL = server.URL + "/tavily"
	response, err := service.SearchWithConfig(context.Background(), "央企 27 届", server.URL, "llm-key", "model", SearchConfig{Provider: "tavily", APIKey: "tavily-key"})
	if err != nil {
		t.Fatal(err)
	}
	if response.SearchProvider != "tavily" || !response.AIProcessed || len(response.Results) != 1 {
		t.Fatalf("unexpected Tavily response: %+v", response)
	}
}

func TestSearchWithLocalToolRunsDeepSeekToolLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			fmt.Fprintf(w, `<a class="result__a" href="http://%s/page">上海高校毕业生招聘会</a><a class="result__snippet">2026年10月10日，面向应届毕业生。</a>`, r.Host)
		case "/page":
			fmt.Fprint(w, `<html><body>上海高校毕业生招聘会，2026年10月10日，面向应届毕业生。</body></html>`)
		case "/v1/chat/completions":
			var request struct {
				Messages []struct {
					Role string `json:"role"`
				} `json:"messages"`
				Tools      []any  `json:"tools"`
				ToolChoice string `json:"tool_choice"`
				Thinking   struct {
					Type string `json:"type"`
				} `json:"thinking"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			hasToolResult := false
			for _, message := range request.Messages {
				if message.Role == "tool" {
					hasToolResult = true
				}
			}
			if !hasToolResult && (len(request.Tools) != 1 || request.ToolChoice != "required" || request.Thinking.Type != "disabled") {
				t.Fatalf("local tool was not registered correctly: tools=%d choice=%q thinking=%q", len(request.Tools), request.ToolChoice, request.Thinking.Type)
			}
			w.Header().Set("Content-Type", "application/json")
			if !hasToolResult {
				json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role":    "assistant",
							"content": "",
							"tool_calls": []any{map[string]any{
								"id":   "call_local_1",
								"type": "function",
								"function": map[string]string{
									"name":      "web_search",
									"arguments": `{"query":"上海 2026 招聘会"}`,
								},
							}},
						},
					}},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{
					"finish_reason": "stop",
					"message": map[string]string{
						"role":    "assistant",
						"content": `{"results":[{"type":"recruitment_event","title":"上海高校毕业生招聘会","organization":"上海市就业服务中心","time":"2026年10月10日","location":"上海","target_audience":["应届毕业生"],"application_period":"2026年9月1日至2026年9月30日","source_url":"http://` + r.Host + `/page"}]}`,
					},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewService()
	service.SearchURL = server.URL + "/search"
	service.HTTPClient = server.Client()
	response, err := service.SearchWithConfig(context.Background(), "上海招聘会", server.URL, "llm-key", "model", SearchConfig{Provider: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if response.SearchProvider != "local" || !response.AIProcessed || len(response.Results) != 1 {
		t.Fatalf("unexpected local tool response: %+v", response)
	}
}

func TestSearchWithDeepSeekResponsesUsesBuiltInWebSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["input"] == nil || request["tools"] == nil || request["tool_choice"] == nil {
			t.Fatalf("Responses request is missing search fields: %#v", request)
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		sourceURL := scheme + "://" + r.Host + "/career"
		content := fmt.Sprintf(`{"results":[{"type":"campus_recruitment","title":"香港 Global Company 2027 校招","organization":"Global Company","target_audience":["2027届"],"source_url":"%s","deadline":"2026-10-01","key_info":["面向应届毕业生"]}]}`, sourceURL)
		json.NewEncoder(w).Encode(map[string]any{
			"output": []any{
				map[string]any{
					"type":   "web_search_call",
					"status": "completed",
					"action": map[string]string{"type": "open_page", "url": sourceURL},
				},
				map[string]any{"type": "message", "content": []any{
					map[string]any{"type": "output_text", "text": content},
				}},
			},
		})
	}))
	defer server.Close()

	service := NewService()
	response, err := service.searchWithDeepSeekResponses(context.Background(), "香港外企", server.URL, "llm-key", "deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	if response.SearchProvider != "deepseek_responses" || len(response.Results) != 1 {
		t.Fatalf("unexpected Responses response: %+v", response)
	}
	if response.Results[0].SourceURL != server.URL+"/career" {
		t.Fatalf("unexpected source URL: %+v", response.Results[0])
	}
}

func TestSearchWithDeepSeekResponsesAcceptsSearchResultURLWithoutOpenPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"output": []any{
				map[string]any{
					"type":   "web_search_call",
					"status": "completed",
					"action": map[string]any{"type": "search", "queries": []string{"香港外企校招"}},
				},
				map[string]any{"type": "message", "content": []any{
					map[string]any{"type": "output_text", "text": `{"results":[{"type":"campus_recruitment","title":"香港外企 2027 校招","organization":"Global Company","location":"香港","target_audience":["2027届"],"deadline":"2026年12月31日","source_url":"https://not-proven.example/career"}]}`},
				}},
			},
		})
	}))
	defer server.Close()

	response, err := NewService().searchWithDeepSeekResponses(context.Background(), "香港外企", server.URL, "llm-key", "deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].SourceURL != "https://not-proven.example/career" {
		t.Fatalf("search result URL was incorrectly discarded: %+v", response.Results)
	}
}

func TestSearchWithDeepSeekResponsesRequiresWebSearchCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"output": []any{
				map[string]any{"type": "message", "content": []any{
					map[string]any{"type": "output_text", "text": `{"results":[]}`},
				}},
			},
		})
	}))
	defer server.Close()

	if _, err := NewService().searchWithDeepSeekResponses(context.Background(), "香港外企", server.URL, "llm-key", "deepseek-v4-flash"); err == nil || !strings.Contains(err.Error(), "web_search") {
		t.Fatalf("expected missing web_search error, got %v", err)
	}
}

func TestSearchWithDeepSeekResponsesRetriesWhenFewerThanFiveVerifiedResults(t *testing.T) {
	var server *httptest.Server
	calls := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		calls++
		count := 1
		if calls > 1 {
			count = 5
		}
		output := make([]any, 0, count+1)
		results := make([]Result, 0, count)
		for i := 1; i <= count; i++ {
			url := fmt.Sprintf("%s/career-%d", server.URL, i)
			output = append(output, map[string]any{
				"type":   "web_search_call",
				"status": "completed",
				"action": map[string]string{"type": "open_page", "url": url},
			})
			results = append(results, Result{
				Type:      "campus_recruitment",
				Title:     fmt.Sprintf("香港 Global Company %d 校招", i),
				SourceURL: url,
				Deadline:  "2026年12月31日",
			})
		}
		encoded, err := json.Marshal(extracted{Results: results})
		if err != nil {
			t.Fatal(err)
		}
		output = append(output, map[string]any{"type": "message", "content": []any{
			map[string]any{"type": "output_text", "text": string(encoded)},
		}})
		json.NewEncoder(w).Encode(map[string]any{
			"output": output,
		})
	}))
	defer server.Close()

	response, err := NewService().searchWithDeepSeekResponses(context.Background(), "香港外企", server.URL, "llm-key", "deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(response.Results) < 5 {
		t.Fatalf("expected a retry with at least five results, calls=%d response=%+v", calls, response)
	}
}

func TestFilterDeepSeekResultsHonorsQueryScope(t *testing.T) {
	results := []Result{
		{Type: "campus_recruitment", Title: "香港外企 2027 校招", Organization: "Global Company", Location: "香港", SourceURL: "https://example.com/hong-kong-career", Deadline: "2026年12月31日"},
		{Type: "campus_recruitment", Title: "深圳某企业 2027 校招", Organization: "某企业", Location: "深圳", SourceURL: "https://example.com/shenzhen-career", Deadline: "2026年12月31日"},
	}

	filtered := filterDeepSeekResults(results, "香港外企招聘 2026 2027届")
	if len(filtered) != 1 || filtered[0].SourceURL != results[0].SourceURL {
		t.Fatalf("query scope filter kept unrelated result: %+v", filtered)
	}
}

func TestSearchReturnsErrorWhenAIExtractionFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			fmt.Fprintf(w, `<a class="result__a" href="http://%s/page">某企业校招</a><a class="result__snippet">面向应届毕业生</a>`, r.Host)
			return
		}
		if r.URL.Path == "/page" {
			fmt.Fprint(w, `<html><body>某企业校招，面向应届毕业生</body></html>`)
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			http.Error(w, `{"error":{"message":"mock llm failure"}}`, http.StatusBadGateway)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	service := NewService()
	service.SearchURL = server.URL + "/search"
	service.HTTPClient = server.Client()
	if _, err := service.Search(context.Background(), "香港外企", server.URL, "llm-key", "model"); err == nil || !strings.Contains(err.Error(), "AI 整理失败") {
		t.Fatalf("expected mandatory AI extraction error, got %v", err)
	}
}

func TestParseSearchHTMLAcceptsAttributeOrderAndUnwrapsURL(t *testing.T) {
	raw := `<a href="//html.duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fcareer" class="result__a">某企业 <b>2027</b> 校招</a><a class="result__snippet">面向应届毕业生开放报名</a>`
	items := parseSearchHTML(raw)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].URL != "https://example.com/career" || items[0].Title != "某企业 2027 校招" {
		t.Fatalf("unexpected candidate: %+v", items[0])
	}
}

func TestParseBingHTML(t *testing.T) {
	raw := `<li class="b_algo"><h2><a href="https://example.com/career">某企业 2027 校招</a></h2><div class="b_caption"><p>面向应届毕业生开放报名</p></div></li>`
	items := parseBingHTML(raw)
	if len(items) != 1 || items[0].URL != "https://example.com/career" || items[0].Snippet == "" {
		t.Fatalf("unexpected Bing candidate: %+v", items)
	}
}

func TestParseBraveHTML(t *testing.T) {
	raw := `<div class="snippet" data-type="web"><a href="https://example.com/hk-campus" class="result-link l1"><div class="title search-snippet-title line-clamp-1" title="香港应届生校招 2027">香港应届生校招 2027</div></a></div>`
	items := parseBraveHTML(raw)
	if len(items) != 1 || items[0].URL != "https://example.com/hk-campus" || items[0].Title != "香港应届生校招 2027" {
		t.Fatalf("unexpected Brave candidate: %+v", items)
	}
}

func TestParseBaiduHTML(t *testing.T) {
	raw := `<h3 class="c-title"><a href="http://www.baidu.com/link?url=abc"><em>香港</em>外企 2027 校招</a></h3>`
	items := parseBaiduHTML(raw)
	if len(items) != 1 || items[0].URL != "http://www.baidu.com/link?url=abc" || items[0].Title != "香港 外企 2027 校招" {
		t.Fatalf("unexpected Baidu candidate: %+v", items)
	}
}

func TestFilterAndDeduplicateRejectsUnknownAndExpiredResults(t *testing.T) {
	candidates := []candidate{
		{Title: "官方校招", URL: "https://example.gov.cn/career"},
		{Title: "旧信息", URL: "https://old.example.com/career"},
	}
	results := []Result{
		{Type: "campus_recruitment", Title: "官方校招", Organization: "某集团", SourceURL: candidates[0].URL, Deadline: "2026-10-01"},
		{Type: "campus_recruitment", Title: "官方校招转载", Organization: "某集团", SourceURL: candidates[1].URL, Deadline: "2026-08-01"},
		{Type: "campus_recruitment", Title: "不在候选证据中", SourceURL: "https://not-in-search.example"},
	}
	filtered := filterAndDeduplicate(results, candidates, QueryIntent{ResultCount: 8}, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if len(filtered) != 1 || filtered[0].SourceURL != candidates[0].URL || filtered[0].SourceType != "official" {
		t.Fatalf("unexpected filtered results: %+v", filtered)
	}
}

func TestApplicationWindowOpenOnlyKeepsCurrentlyOpenResults(t *testing.T) {
	now := time.Date(2026, 9, 4, 15, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	candidates := []candidate{
		{Title: "正在报名的校招", URL: "https://example.com/open"},
		{Title: "尚未开始的校招", URL: "https://example.com/future"},
		{Title: "已经截止的校招", URL: "https://example.com/closed"},
		{Title: "滚动招聘", URL: "https://example.com/rolling"},
		{Title: "没有报名时间的校招", URL: "https://example.com/unknown"},
	}
	results := []Result{
		{Type: "campus_recruitment", Title: "正在报名的校招", SourceURL: candidates[0].URL, ApplicationPeriod: "2026年9月1日至2026年9月30日"},
		{Type: "campus_recruitment", Title: "尚未开始的校招", SourceURL: candidates[1].URL, ApplicationPeriod: "2026年9月10日至2026年9月30日"},
		{Type: "campus_recruitment", Title: "已经截止的校招", SourceURL: candidates[2].URL, Deadline: "2026年8月31日"},
		{Type: "campus_recruitment", Title: "滚动招聘", SourceURL: candidates[3].URL, ApplicationPeriod: "滚动招聘"},
		{Type: "campus_recruitment", Title: "没有报名时间的校招", SourceURL: candidates[4].URL},
	}

	filtered := filterAndDeduplicate(results, candidates, QueryIntent{ResultCount: 8, PastSearch: true}, now)
	if len(filtered) != 2 || filtered[0].SourceURL != candidates[0].URL || filtered[1].SourceURL != candidates[3].URL {
		t.Fatalf("application window filter kept unavailable results: %+v", filtered)
	}
}

func TestApplicationWindowOpenHandlesCommonDateForms(t *testing.T) {
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{name: "future start", result: Result{ApplicationPeriod: "2026-09-10起开放"}, want: false},
		{name: "started no end", result: Result{ApplicationPeriod: "2026年9月1日起开放"}, want: true},
		{name: "future deadline", result: Result{Deadline: "申请截止：2026/10/01"}, want: true},
		{name: "month deadline", result: Result{ApplicationPeriod: "申请截止：2026年10月"}, want: true},
		{name: "expired deadline", result: Result{Deadline: "申请截止：2026/08/31"}, want: false},
		{name: "rolling", result: Result{ApplicationPeriod: "滚动招聘，招满即止"}, want: true},
		{name: "missing window", result: Result{Time: "2027年7月入职"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := applicationWindowOpen(test.result, now); got != test.want {
				t.Fatalf("applicationWindowOpen()=%v, want %v for %+v", got, test.want, test.result)
			}
		})
	}
}
