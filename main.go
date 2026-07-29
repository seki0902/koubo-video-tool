package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"koubo-video-tool/chanjing"
	"koubo-video-tool/handlers"
	"koubo-video-tool/scheduler"
	"koubo-video-tool/store"
)

//go:embed frontend/* skills/*
var embedded embed.FS

func main() {
	exeDir := exePath()
	dataDir := filepath.Join(exeDir, "data")
	os.MkdirAll(dataDir, 0755)

	initDataFiles(dataDir)

	cfg, _ := store.LoadConfig(filepath.Join(dataDir, "config.json"))
	cjClient := chanjing.NewClient(cfg.Chanjing.AppID, cfg.Chanjing.SecretKey)

	sch := scheduler.New(cjClient, func(t store.Task) {
		tasksPath := filepath.Join(dataDir, "tasks.json")
		store.UpdateTask(tasksPath, t.TaskID, func(existing *store.Task) {
			existing.Status = t.Status
			existing.Progress = t.Progress
			existing.VideoURL = t.VideoURL
			existing.Error = t.Error
			existing.CompletedAt = t.CompletedAt
		})
	})

	// 恢复未完成任务
	tasks, _ := store.LoadTasks(filepath.Join(dataDir, "tasks.json"))
	sch.Restore(tasks)

	h := handlers.New(dataDir, cjClient, sch)
	h.Register()

	frontendFS, _ := fs.Sub(embedded, "frontend")
	http.Handle("/", http.FileServer(http.FS(frontendFS)))

	port := "8899"
	url := fmt.Sprintf("http://localhost:%s", port)

	fmt.Printf("服务启动: %s\n", url)
	fmt.Printf("数据目录: %s\n", dataDir)

	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()

	srv := &http.Server{Addr: ":" + port}

	// 启动 HTTP 服务（goroutine）
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
			os.Exit(1)
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n正在关闭...")
	sch.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	fmt.Println("已关闭")
}

func exePath() string {
	p, _ := os.Executable()
	return filepath.Dir(p)
}

func initDataFiles(dir string) {
	files := map[string]string{
		"config.json":  "{}",
		"tasks.json":   "[]",
		"avatars.json": "[]",
		"voices.json":  "[]",
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			os.WriteFile(p, []byte(content), 0644)
		}
	}
}
