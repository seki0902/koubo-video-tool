// Command topic-search-trace runs three real Topic Search requests with the
// production 110-second request deadline. Structured trace JSONL is emitted
// by the topicsearch package to stderr; this command only prints a small
// human-readable result summary to stdout.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"koubo-video-tool/topicsearch"
)

var traceQueries = []string{
	"香港外企招聘 2026 2027届",
	"香港金融机构应届生招聘",
	"江浙沪外企招聘会 2026",
}

func main() {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "DEEPSEEK_API_KEY is not set")
		os.Exit(2)
	}
	apiURL := os.Getenv("DEEPSEEK_API_URL")
	if apiURL == "" {
		apiURL = "https://api.deepseek.com"
	}
	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = "deepseek-v4-flash"
	}

	for _, query := range traceQueries {
		ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
		started := time.Now()
		result, err := topicsearch.NewService().SearchWithConfig(ctx, query, apiURL, apiKey, model, topicsearch.SearchConfig{Provider: "local"})
		cancel()
		if err != nil {
			fmt.Printf("query=%q duration_ms=%d error=%q\n", query, time.Since(started).Milliseconds(), err.Error())
			continue
		}
		fmt.Printf("query=%q duration_ms=%d total_found=%d results=%d\n", query, time.Since(started).Milliseconds(), result.TotalFound, len(result.Results))
	}
}
