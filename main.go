package main

import (
	"context"
	"embed"
	"flag"
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

//go:embed frontend/* skills/* "chanjing/person id/*.png"
var embedded embed.FS

func main() {
	printCreds := flag.Bool("print-creds", false, "打印解密后的蝉镜凭据并退出（调试用）")
	testChanjing := flag.Bool("test-chanjing", false, "使用 n8n 工作流同款硬编码参数直接调蝉镜 API（调试用）")
	flag.Parse()

	exeFile := executablePath()
	exeDir := filepath.Dir(exeFile)
	dataDir := filepath.Join(exeDir, "data")
	os.MkdirAll(dataDir, 0755)

	if *printCreds {
		cfg, err := store.LoadConfig(filepath.Join(dataDir, "config.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("APP_ID=%s\n", cfg.Chanjing.AppID)
		fmt.Printf("SECRET_KEY=%s\n", cfg.Chanjing.SecretKey)
		return
	}

	if *testChanjing {
		cfg, err := store.LoadConfig(filepath.Join(dataDir, "config.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
			os.Exit(1)
		}
		// 使用 n8n 工作流里硬编码的同款参数
		client := chanjing.NewClient(cfg.Chanjing.AppID, cfg.Chanjing.SecretKey)
		token, err := client.GetToken()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取 token 失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Token 获取成功: %s...\n", token[:20])

		taskID, err := client.CreateVideo(token, chanjing.CreateVideoRequest{
			Person: chanjing.PersonConfig{
				ID:         "C-6a951f1434884368bc4942f7f74885ff",
				X:          0,
				Y:          0,
				Width:      1080,
				Height:     1920,
				FigureType: "stand_body",
				DriveMode:  "random",
				Backway:    1,
			},
			Audio: chanjing.AudioConfig{
				TTS: chanjing.TTSConfig{
					Text:     []string{"测试测试，这是一条测试稿子"},
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
			SubtitleConfig:         chanjing.SubtitleConfig{Show: false},
			AddComplianceWatermark: false,
			Source:                 1,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建视频失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ 成功！任务 ID: %s\n", taskID)
		return
	}

	initDataFiles(dataDir)
	seedAvatars(dataDir)
	seedVoices(dataDir)

	cfg, err := store.LoadConfig(filepath.Join(dataDir, "config.json"))
	if err != nil {
		fi, statErr := os.Stat(filepath.Join(dataDir, "config.json"))
		if statErr == nil && fi.Size() > 0 {
			fmt.Fprintf(os.Stderr, "警告: 配置文件损坏，将使用默认配置 (%v)\n", err)
		}
	}
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

	frontendFS, err := fs.Sub(embedded, "frontend")
	if err != nil {
		panic(fmt.Sprintf("嵌入资源加载失败 frontend: %v", err))
	}
	http.Handle("/", http.FileServer(http.FS(frontendFS)))

	// 服务数字人形象图片
	personFS, err := fs.Sub(embedded, "chanjing/person id")
	if err != nil {
		panic(fmt.Sprintf("嵌入资源加载失败 chanjing/person id: %v", err))
	}
	http.Handle("/img/person/", http.StripPrefix("/img/person/", http.FileServer(http.FS(personFS))))

	port := "8899"
	url := fmt.Sprintf("http://localhost:%s", port)

	fmt.Printf("服务启动: %s\n", url)
	fmt.Printf("数据目录: %s\n", dataDir)

	// 首次运行在桌面创建启动脚本
	createDesktopShortcut(exeFile)

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

func executablePath() string {
	p, err := os.Executable()
	if err != nil {
		p, _ = filepath.Abs(os.Args[0])
	}
	if p == "" {
		p, _ = filepath.Abs(os.Args[0])
	}
	return p
}

func createDesktopShortcut(exeFile string) {
	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	if _, err := os.Stat(desktop); err != nil {
		return // 桌面路径不可用
	}

	shortcutPath := filepath.Join(desktop, "口播视频工具.bat")
	if _, err := os.Stat(shortcutPath); err == nil {
		return // 已存在
	}

	os.WriteFile(shortcutPath, []byte(desktopShortcutContent(exeFile)), 0644)
	fmt.Println("桌面快捷方式已创建")
}

func desktopShortcutContent(exeFile string) string {
	return fmt.Sprintf("@echo off\r\nchcp 65001 > nul\r\nstart \"\" \"%s\"\r\n", exeFile)
}

func initDataFiles(dir string) {
	files := map[string]string{
		"config.json":  "{}",
		"tasks.json":   "[]",
		"topics.json":  "[]",
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
