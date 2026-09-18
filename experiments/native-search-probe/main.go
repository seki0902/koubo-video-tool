// Run production search with the local configuration; never print keys.
package main

import (
	"context"
	"fmt"
	"koubo-video-tool/store"
	"koubo-video-tool/topicsearch"
	"os"
	"time"
)

func main() {
	c, e := store.LoadConfig("data/config.json")
	if e != nil {
		panic(e)
	}
	queries := os.Args[1:]
	if len(queries) == 0 {
		queries = []string{"外企", "招聘会"}
	}
	for _, q := range queries {
		ctx, cancel := context.WithTimeout(context.Background(), 190*time.Second)
		result, e := topicsearch.NewService().SearchWithConfigFromStore(ctx, q, c)
		cancel()
		if e != nil {
			fmt.Printf("query=%q error=%v\n", q, e)
			os.Exit(1)
		}
		fmt.Printf("query=%q provider=%s calls=%d results=%d\n", q, result.SearchProvider, result.WebSearchCallCount, len(result.Results))
		for _, r := range result.Results {
			fmt.Printf("%s | %s\n", r.Title, r.SourceURL)
		}
	}
}
