package skill

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"koubo-video-tool/store"
)

// BuildPrompt 读取 Skill 源，拼接所有 .md 文件为 system prompt
// GitHub 源同步成功后会自动更新 cfg.Skill.GithubCacheUntil
func BuildPrompt(cfg *store.Config) (string, error) {
	switch cfg.Skill.Source {
	case "local":
		if cfg.Skill.LocalPath == "" {
			return "", fmt.Errorf("本地 Skill 路径未设置")
		}
		return readLocalDir(cfg.Skill.LocalPath)

	case "github":
		if cfg.Skill.GithubURL == "" {
			return "", fmt.Errorf("GitHub Skill URL 未设置")
		}
		return readGitHub(cfg)

	default:
		return "", fmt.Errorf("未知 Skill 源类型: %s", cfg.Skill.Source)
	}
}

func readLocalDir(root string) (string, error) {
	var parts []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("读取 Skill 目录失败: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subDir := filepath.Join(root, entry.Name())
		files, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(subDir, f.Name()))
			if err != nil {
				continue
			}
			parts = append(parts, string(content))
		}
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("Skill 目录中未找到任何 .md 文件: %s", root)
	}
	return strings.Join(parts, "\n\n---\n\n"), nil
}

// ──────────────────── GitHub 源 ────────────────────

type ghContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"` // "dir" or "file"
	DownloadURL string `json:"download_url"`
}

func readGitHub(cfg *store.Config) (string, error) {
	cacheDir := filepath.Join(os.TempDir(), "koubo-skill-cache")
	now := time.Now().Unix()

	// 缓存有效则直接读本地
	if cfg.Skill.GithubCacheUntil > now {
		if _, err := os.Stat(cacheDir); err == nil {
			prompt, err := readLocalDir(cacheDir)
			if err == nil {
				return prompt, nil
			}
		}
	}

	// 从 raw URL 推导 API URL
	apiURL, branch := rawToAPI(cfg.Skill.GithubURL)
	if apiURL == "" {
		return "", fmt.Errorf("无法解析 GitHub Raw URL: %s", cfg.Skill.GithubURL)
	}

	client := &http.Client{Timeout: 20 * time.Second}

	if err := fetchGitHubTree(client, apiURL, branch, cacheDir); err != nil {
		return "", fmt.Errorf("GitHub 同步失败: %w", err)
	}

	// 更新缓存时间（指针回写）
	cfg.Skill.GithubCacheUntil = now + 24*3600

	prompt, err := readLocalDir(cacheDir)
	if err != nil {
		return "", fmt.Errorf("GitHub 缓存读取失败: %w", err)
	}
	return prompt, nil
}

// rawToAPI 将 raw.githubusercontent.com URL 转为 API URL
// https://raw.githubusercontent.com/owner/repo/branch/path → https://api.github.com/repos/owner/repo/contents/path, branch
func rawToAPI(rawURL string) (apiURL, branch string) {
	rawURL = strings.TrimRight(rawURL, "/")
	prefix := "https://raw.githubusercontent.com/"
	if !strings.HasPrefix(rawURL, prefix) {
		// 尝试直接作为 API URL 使用
		if strings.Contains(rawURL, "api.github.com") {
			return rawURL, ""
		}
		return "", ""
	}
	rest := strings.TrimPrefix(rawURL, prefix)
	parts := strings.SplitN(rest, "/", 3) // [owner, repo, branch/path...]
	if len(parts) < 3 {
		return "", ""
	}
	owner, repo := parts[0], parts[1]
	remainder := parts[2]
	idx := strings.Index(remainder, "/")
	if idx < 0 {
		branch = remainder
		return fmt.Sprintf("https://api.github.com/repos/%s/%s/contents", owner, repo), branch
	}
	branch = remainder[:idx]
	subPath := remainder[idx+1:]
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, subPath), branch
}

func fetchGitHubTree(client *http.Client, apiURL, branch, destDir string) error {
	url := apiURL
	if branch != "" {
		url += "?ref=" + branch
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "koubo-video-tool")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub API 返回状态 %d (URL: %s)", resp.StatusCode, url)
	}

	var items []ghContent
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return fmt.Errorf("解析 GitHub API 响应失败: %w", err)
	}

	os.RemoveAll(destDir)
	os.MkdirAll(destDir, 0755)

	for _, item := range items {
		if item.Type == "dir" {
			subURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", extractRepo(apiURL), item.Path)
			subDest := filepath.Join(destDir, item.Name)
			os.MkdirAll(subDest, 0755)
			if err := fetchGitHubTree(client, subURL, branch, subDest); err != nil {
				return err
			}
		}
		if item.Type == "file" && strings.HasSuffix(item.Name, ".md") {
			content, err := fetchFile(client, item.DownloadURL)
			if err != nil {
				continue
			}
			parentDir := filepath.Join(destDir, filepath.Base(filepath.Dir(item.Path)))
			os.MkdirAll(parentDir, 0755)
			os.WriteFile(filepath.Join(parentDir, item.Name), content, 0644)
		}
	}
	return nil
}

func fetchFile(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch file %s: status %d", url, resp.StatusCode)
	}
	buf := new(strings.Builder)
	// 简单的 read-all
	data := make([]byte, 4096)
	for {
		n, _ := resp.Body.Read(data)
		if n == 0 {
			break
		}
		buf.Write(data[:n])
	}
	return []byte(buf.String()), nil
}

func extractRepo(apiURL string) string {
	// https://api.github.com/repos/owner/repo/contents/path → owner/repo
	prefix := "https://api.github.com/repos/"
	if !strings.HasPrefix(apiURL, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(apiURL, prefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return rest
}
