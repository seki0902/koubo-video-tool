
# Koubo Video Tool

留学生口播稿生成 + 蝉镜数字人视频生成工具。

## 功能

- 输入话题，调用大模型生成口播初稿
- 本地编辑口播稿
- 选择数字人形象和人声
- 提交蝉镜任务并轮询进度
- 保留任务记录，支持重试、删除、清空

## 目录说明

- `main.go`：程序入口
- `handlers/`：HTTP 接口
- `skill/`：口播 `skill` 加载逻辑
- `frontend/`：前端页面
- `data/`：运行配置和任务数据
- `skills/`：本地 skill 副本

## 运行方式

### 方式一：直接运行编译后的程序

双击 `koubo-tool.exe` 或 `koubo-video-tool.exe`。

首次启动会自动打开页面：

`http://localhost:8899`

### 方式二：本地编译

需要本机已安装 Go。

```bat
build.bat
```

## 首次配置

打开“首次设置”页，填这几项：

1. 蝉镜 `AppID`
2. 蝉镜 `SecretKey`
3. 大模型 `API 地址`
4. 大模型 `API Key`
5. 大模型 `模型名`
6. `skill` 源

## Skill 配置

当前工具支持两种来源：

- `local`：读取本地文件夹
- `github`：从 GitHub 仓库拉取

推荐本地方式。

### 本地 skill 地址

默认会优先读取：

`skills/Koubo-rewrite-skill-local`

也可以在设置页手动填绝对路径。

### GitHub 方式

填仓库的 Raw 根地址，工具会自动同步并缓存 24 小时。

## 口播稿生成流程

1. 输入主题
2. 点击 `AI 生成初稿`
3. 编辑并确认口播稿
4. 选择数字人、人声和参数
5. 点击 `生成视频`

## 注意事项

- 没有配置大模型时，不能生成口播稿
- 没有配置蝉镜凭证时，不能生成视频
- `skill` 只负责生成提示词，不会自动联网执行脚本
- 本地 skill 副本只用于当前工具，不会修改 GitHub 远端内容

## 常见问题

### 为什么生成结果不像正文？

先确认是否使用了最新程序版本，再确认本地 `skill` 目录是否完整。

### 为什么提示大模型未配置？

检查 `data/config.json` 里的 `llm.api_url`、`llm.api_key`、`llm.model`。

### 为什么找不到 skill？

确认本地目录存在：

`skills/Koubo-rewrite-skill-local`

并且目录下有 `SKILL.md`、`docs/`、`library/`。

