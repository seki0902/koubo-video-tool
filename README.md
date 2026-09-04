
# Koubo Video Tool

留学生口播稿生成 + 蝉镜数字人视频生成工具。

## 功能

- 输入话题，调用大模型生成口播初稿
- 本地编辑口播稿
- 选择数字人形象和人声
- 提交蝉镜任务并轮询进度
- 保留任务记录，支持重试、删除、清空
- AI 选题搜索：联网检索、筛选并整理招聘会/校招/人才引进信息

## 目录说明

- `main.go`：程序入口
- `handlers/`：HTTP 接口
- `skill/`：口播 `skill` 加载逻辑
- `frontend/`：前端页面
- `data/`：运行配置和任务数据
- `skills/`：本地 skill 副本

## 运行方式

### 方式一：直接运行编译后的程序

双击当前最新版 `koubo-video-tool-2.0.exe`（也可运行 `koubo-video-tool.exe`）。

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
6. 联网搜索工具（默认本地联网搜索，无需搜索 API Key；Tavily/Brave 可选）
7. `skill` 源

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

## AI 选题搜索

主界面顶部的“AI 选题搜索”支持自然语言输入，例如“香港外企”“江浙沪招聘会”或“QS100人才引进”。这是一个小型招聘搜索 Agent：先由大模型理解需求并扩展查询词，再联网搜索候选页面，最后由大模型依据搜索证据抽取招聘事实、过滤过期或不相关信息并去重。前端只展示结构化事实和原文链接，不展示内部评分。

搜索服务默认使用本地联网搜索，不需要 Tavily、Brave 或其他搜索 API Key。对于 `https://api.deepseek.com`，程序会直接调用 DeepSeek Responses API 的内置 `web_search`；其他兼容 API 则由本地程序访问搜索引擎页面、抓取候选网页，再把搜索证据作为 tool message 回传给模型。

如果希望使用结构化搜索 API，也可以选择：

- `Tavily Search API`：推荐，返回结构化搜索结果，稳定性较好。
- `Brave Search API`：返回结构化网页搜索结果，也需要单独的 API Key。

使用 DeepSeek API 时，内置 `web_search` 和 Responses API 在同一条调用链中完成搜索与整理；使用其他兼容 API 时，搜索工具和大模型仍是两条独立链路。本地搜索或第三方搜索 API 都可以作为后者的证据来源。

如果 AI 抽取或核验失败，接口会返回失败状态，不会把网页标题直接冒充成 AI 结果。

搜索结果可通过“加入选题库”保存到运行目录的 `data/topics.json`，保留完整的 `raw_info`，后续可直接作为口播稿生成的结构化上下文。

### DeepSeek 内置 web_search 测试

独立探针位于 `experiments/deepseek-responses-web-search/test_web_search.py`。它会对每个测试 query 依次运行 `basic`、`strict`、`extreme` 三档 prompt，并保存完整 Responses API response、最终 JSON、耗时、token 用量、`web_search_call` 计数、输出契约校验和 URL 可访问性。

```powershell
$env:DEEPSEEK_API_KEY = "你的 API Key"
py -3 .\experiments\deepseek-responses-web-search\test_web_search.py
```

结果默认写入 `artifacts/deepseek-responses-web-search/`。单条 JSON 文件中的 `raw_response` 用于核对搜索调用和原始证据，`probe.final_structured_message` 用于核对最终 JSON；`summary.csv` 方便比较三档 prompt 的 JSON 合法性、契约通过率、结果数量和 URL 可访问数量。只想跑某一档时可使用 `--prompt-levels basic`、`--prompt-levels strict` 或 `--prompt-levels extreme`。

`contract_valid` 只校验可机器判断的结构和字段类型，不能单独证明标题、摘要或公司名确实来自搜索证据；来源忠实性仍需结合同一文件中的原始 `web_search_call` 内容人工核对。

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
