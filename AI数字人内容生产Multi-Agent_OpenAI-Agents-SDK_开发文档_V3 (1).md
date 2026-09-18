# AI 数字人内容生产 Multi-Agent 系统｜OpenAI Agents SDK 开发文档 V2

> 本文档替代上一版 LangGraph 实现方案。
>
> **产品逻辑不变，编排框架改为 OpenAI Agents SDK。**
>
> 前台仍然是一个连续对话框。
>
> 后台采用：
>
> **Supervisor Agent + Topic Agent + Research Agent + Script Agent + Content Review Agent**
>
> 以及：
>
> **Agents-as-tools + Function Tools + Sessions + Runtime Context + Structured Outputs + Guardrails + Tracing + HITL**
>
> 数字人生成仍然属于 Tool，不单独设计为 Agent。
>
> 核心原则：
>
> **能力预先定义，路径不预先定义。**
>
> **用户只和 Supervisor 对话，Supervisor 自主调用专业 Agent 和 Tool 完成任务。**

---

# 1. 为什么本项目改用 OpenAI Agents SDK

这个项目的核心需求是：

```text
前台：
一个连续聊天框

后台：
Supervisor
├── Topic Agent
├── Research Agent
├── Script Agent
└── Content Review Agent

同时具备：
会话记忆
Tool 调用
Agent 协作
结构化输出
人工确认
Trace
```

相比复杂的 Graph 状态机，本项目更需要：

```text
轻量 Multi-Agent
统一对话入口
专业 Agent 调用
连续 Session
Tool 使用
清晰 Trace
```

因此第一版采用 OpenAI Agents SDK。

Agents SDK 本身提供：

```text
Agent
Runner
Agents as tools
Handoffs
Function Tools
Sessions
Guardrails
Human-in-the-loop
Tracing
Structured output
```

本项目优先使用：

```text
Manager pattern
+
Agents as tools
```

而不是 Handoff。

原因：

```text
用户始终只和一个“内容生产助手”对话。
Supervisor 始终持有会话控制权。
专业 Agent 只负责完成专项任务并把结果交回 Supervisor。
```

---

# 2. 项目定位

现有网页工具已经具备：

```text
输入关键词
↓
自动搜索选题
↓
人工选择
↓
DeepSeek 生成口播稿
↓
人工修改
↓
数字人生成
```

本期不重新开发这些基础能力。

本期改造目标：

```text
按钮驱动工具
↓
升级为
↓
对话驱动 Multi-Agent 内容生产系统
```

用户以后不需要知道：

```text
选题按钮在哪里
搜索按钮在哪里
写稿按钮在哪里
数字人按钮在哪里
```

用户只需要表达：

```text
今天做啥好呢
这个方向可以，但是换几个还在招聘的
第三个写吧
第二段短一点
开头换成问句
这个版本可以
生成吧
```

系统负责理解当前状态并推进任务。

---

# 3. 最终用户体验

## 3.1 开放式选题

用户：

```text
今天做啥好呢？
```

系统内部：

```text
Supervisor
↓
Topic Agent
↓
Topic Agent 根据需要调用搜索 Tool
↓
必要时 Supervisor 调 Research Agent
↓
Topic Agent / Supervisor 汇总
↓
返回 3～5 个候选选题
```

用户看到：

```text
我看了一下近期热点和仍有时效的信息，
这几个方向比较值得做：

1. ...
2. ...
3. ...

每个方向包含：
内容切口
目标人群
为什么现在值得做
关键信息
原始链接
```

---

## 3.2 用户修改筛选条件

用户：

```text
这个方向可以，但是帮我找其他还在招聘的。
```

系统理解：

```text
保留：
当前内容方向

修改：
企业 / 招聘对象候选

增加条件：
仍然在招聘
```

系统内部可能：

```text
Supervisor
↓
Research Agent
↓
search_recruitment
↓
Research Result
↓
Topic Agent
↓
重新组织候选
↓
Supervisor 回复用户
```

---

## 3.3 写稿

用户：

```text
第三个可以，写吧。
```

系统知道：

```text
“第三个” = 当前 topic_candidates[2]
“写吧” = 基于该选题生成口播稿
```

Supervisor：

```text
调用 Script Agent
```

Script Agent读取：

```text
confirmed_topic
verified research
IP Principles
Script Principles
```

返回稿件。

---

## 3.4 改稿

用户：

```text
第二段太长了，压一下。
```

系统知道：

```text
当前对象 = current_draft
修改范围 = 第二段
修改目标 = 更短
```

Supervisor 调 Script Agent。

Script Agent：

```text
只修改相关部分
保留其他已确认内容
生成新版本
```

---

## 3.5 审核

用户可以直接说：

```text
看看有没有风险。
```

Supervisor 调：

```text
Content Review Agent
```

也可以在准备生成前，由 Supervisor 判断是否需要审核。

---

## 3.6 数字人生成

用户：

```text
生成吧。
```

Supervisor 理解：

```text
用户已授权使用当前稿件版本生成数字人
```

然后调用：

```text
generate_digital_human Tool
```

返回：

```text
生成中
完成
视频结果
```

---

# 4. 系统整体架构

```text
Browser Frontend
        │
        ▼
Conversation API
        │
        ▼
Runner.run()
        │
        ▼
Supervisor Agent
        │
        ├── Topic Agent.as_tool()
        │
        ├── Research Agent.as_tool()
        │
        ├── Script Agent.as_tool()
        │
        ├── Content Review Agent.as_tool()
        │
        ├── Function Tools
        │
        └── Digital Human Tool
        │
        ▼
Final Response
```

同时存在两个状态系统：

```text
Session
+
Application State Store
+
Long-term Content Memory
```

分别负责：

```text
Session
= 对话历史

Application State
= 当前内容任务的结构化状态
```

---

# 5. 为什么采用 Agents-as-tools

Agents SDK 主要有两种多 Agent 协作方式：

```text
Handoffs
Agents as tools
```

本项目选择：

```text
Agents as tools
```

原因：

```text
Supervisor 始终保持控制权。
用户始终看到同一个 Assistant。
专业 Agent 完成任务后结果自动回到 Supervisor。
```

例如：

```python
topic_agent.as_tool(
    tool_name="topic_expert",
    tool_description="负责选题判断、候选筛选和内容方向设计"
)
```

Supervisor 可以根据当前用户请求自主决定：

```text
是否调用
什么时候调用
调用哪个
是否连续调用多个专业 Agent
```

这比预先写：

```text
Topic → Research → Script → Review
```

更符合本项目的自主性要求。

---

# 6. 为什么第一版不以 Handoff 为主

Handoff 的语义是：

```text
把当前对话控制权交给另一个 Agent。
```

适合：

```text
客服转人工专家
技术支持转账单 Agent
语言 Agent 接管对话
```

本项目不希望：

```text
用户正在和 Supervisor 聊
↓
突然变成 Research Agent 直接接管整个会话
```

因此第一版采用 Manager pattern。

未来如果某个功能确实需要：

```text
让用户直接进入专项 Agent 会话
```

再使用 Handoff。

---

# 7. 五个 Agent

---

# 7.1 Supervisor Agent

Supervisor 是前台唯一对话主体。

名称建议：

```text
Content Production Supervisor
```

它负责：

```text
理解用户最新输入
理解当前对话历史
理解当前任务 State
理解“这个”“第三个”“写吧”“生成吧”等上下文指代

决定：
是否需要 Topic Agent
是否需要 Research Agent
是否需要 Script Agent
是否需要 Content Review Agent
是否需要直接调用 Tool
是否应该回复用户等待确认
是否应该继续内部调查
```

Supervisor 不负责：

```text
自己深入搜索
自己完成完整选题分析
自己完整写稿
自己做专业审核
```

它负责：

```text
统筹
```

---

# 7.2 Topic Agent

Topic Agent 是选题专家。

名称建议：

```text
Topic Strategy Agent
```

职责：

```text
理解账号定位
理解当前内容目标
判断哪些选题值得做
筛选候选方向
设计内容切入角度
评估时效性
避免重复内容
```

它可以拥有：

```text
search_platform_content Tool
search_recruitment Tool
get_content_history Tool
get_ip_profile Tool
```

它读取：

```text
Topic Principles
```

Topic Agent回答：

```text
现在最值得做什么？
```

---

# 7.3 Research Agent

名称建议：

```text
Research & Verification Agent
```

职责：

```text
搜索事实
核实信息
判断来源质量
确认招聘状态
确认时间
确认目标人群
确认企业
确认岗位
找到原始来源
补充 Script 所需资料
```

它可以拥有：

```text
web_search
search_recruitment
fetch_source
get_content_history
```

Research Agent回答：

```text
这个方向有哪些当前、可靠、可以使用的信息？
```

---

# 7.4 Script Agent

名称建议：

```text
Short-form Script Agent
```

职责：

```text
根据 confirmed_topic 写口播稿
结合 Research Result
遵守 IP 人设
遵守 Script Principles
处理用户多轮修改
维护内容结构
控制 Hook
控制节奏
控制广告感
```

Script Agent回答：

```text
这条内容应该怎么讲？
```

---

# 7.5 Content Review Agent

名称建议：

```text
Content Review Agent
```

职责：

```text
事实检查
内容质量检查
IP 一致性检查
平台风险检查
Hook 检查
结构检查
目标人群检查
夸张承诺检查
CTA 检查
```

它回答：

```text
这版内容是否可以进入生成阶段？
```

---

# 8. Supervisor 如何调用专业 Agent

示意：

```python
supervisor = Agent(
    name="Content Production Supervisor",
    instructions=...,
    tools=[
        topic_agent.as_tool(
            tool_name="topic_expert",
            tool_description="当需要寻找、比较、调整内容选题时调用。"
        ),
        research_agent.as_tool(
            tool_name="research_expert",
            tool_description="当需要搜索、核实、补充最新事实或招聘信息时调用。"
        ),
        script_agent.as_tool(
            tool_name="script_expert",
            tool_description="当用户需要生成或修改当前口播稿时调用。"
        ),
        review_agent.as_tool(
            tool_name="content_reviewer",
            tool_description="当需要检查稿件质量、事实、人设或合规风险时调用。"
        ),
    ]
)
```

注意：

这些 Tool Description 是：

```text
能力描述
```

不是：

```text
固定业务路由规则
```

Supervisor 仍然自己判断是否调用。

---

# 9. 专业 Agent 也可以使用普通 Tool

Multi-Agent 不代表：

```text
所有能力都必须做成 Agent。
```

应该区分：

```text
需要复杂专业判断
→ Agent

只负责查询 / 保存 / 调 API
→ Function Tool
```

例如：

```text
搜索
→ Tool

招聘查询
→ Tool

读取稿件
→ Tool

保存稿件版本
→ Tool

数字人 API
→ Tool
```

---

# 10. Function Tools

Agents SDK 可以把 Python 函数直接暴露给 Agent。

建议封装现有能力：

```text
search_platform_content
search_recruitment
fetch_source
get_content_history
get_ip_profile
get_current_task_state
save_topic_selection
save_draft_version
get_current_draft
generate_digital_human
get_generation_status

# Long-term Content Memory
search_content_history
get_recent_topics
get_high_performing_content
get_similar_scripts
get_revision_history
save_content_item
save_revision_record
update_content_metrics
```

---

# 11. Tool 设计原则

Tool 负责：

```text
查
读
写
执行 API
```

Agent 负责：

```text
判断
分析
决策
生成
```

例如：

错误：

```text
search_recruitment 返回：
“推荐做这个选题。”
```

正确：

```text
search_recruitment 返回：
企业
招聘对象
截止时间
状态
来源
URL
```

“值不值得做”由 Topic Agent 判断。

---

# 12. Conversation Session

Agents SDK 提供 Sessions。

本项目必须使用 Session。

因为用户会连续说：

```text
这个
第三个
写吧
再换几个
刚才那版
第二段
就这样
生成吧
```

如果每一轮都是全新请求：

```text
系统无法理解这些指代。
```

第一版本地建议：

```text
SQLiteSession
```

示意：

```python
session = SQLiteSession(session_id)

result = await Runner.run(
    supervisor,
    user_message,
    session=session,
    context=app_context,
)
```

同一个：

```text
session_id
```

持续用于整个当前对话。

---

# 13. Session 和 Content State 不是一回事

这是本项目非常重要的一点。

Session 保存：

```text
用户说过什么
Assistant 说过什么
模型历史上下文
```

但不能只靠自然语言聊天历史管理业务状态。

还需要：

```text
Content Task State
```

保存结构化业务对象。

例如：

```text
topic_candidates
confirmed_topic
current_draft
draft_versions
approved_version
generation_status
```

所以：

```text
Session
= 对话记忆

Task State
= 产品状态
```

---


# 13.5 Long-term Content Memory / Content Database

本项目除了 Session 与当前任务状态，还必须存在第三类长期状态：

```text
Long-term Content Memory
=
持续积累的内容资产、历史表现、用户修改习惯与账号知识
```

三者职责必须严格区分：

```text
Session Memory
= 当前对话历史

Task State
= 当前这条内容任务的结构化状态

Long-term Content Memory
= 跨会话长期积累、持续更新的内容资产与经验
```

---

## 13.5.1 为什么需要 Long-term Content Memory

如果系统只有 Session 和 Task State，它只能做到：

```text
记得这次对话聊了什么
知道这条内容做到哪一步
```

但它无法真正理解：

```text
这个账号过去做过什么
哪些选题已经重复
哪些稿件历史表现好
哪些 Hook 长期有效
哪些表达用户经常删掉
哪些结构更符合当前 IP
哪些内容最近已经连续做过
哪些稿件最终真正被采用
```

因此，Content Agent 系统必须持续积累长期内容数据。

---

## 13.5.2 Long-term Content Memory 的核心数据

第一版建议至少保存以下几类数据。

### A. 历史内容

```text
content_items
```

建议字段：

```text
content_id
account_id
topic
title
script
content_type
platform
publish_date
status
tags
source_urls
hook
cta
created_at
updated_at
```

---

### B. 发布后表现数据

```text
content_metrics
```

建议字段：

```text
content_id
platform
impressions
views
likes
comments
shares
favorites
clicks
leads
valid_leads
ctr
conversion_rate
lead_rate
collected_at
```

具体字段以现有业务能拿到的数据为准。

---

### C. 稿件版本

```text
script_versions
```

建议字段：

```text
draft_id
content_id
version
script
change_type
user_feedback
is_final
created_at
```

---

### D. 选题历史

```text
topic_history
```

建议字段：

```text
topic_id
topic
content_angle
source
selected
used
publish_date
performance_summary
tags
created_at
```

---

### E. 用户修改记录

```text
revision_history
```

建议字段：

```text
draft_id
version_before
version_after
user_instruction
changed_sections
change_summary
accepted
created_at
```

这部分非常重要。

因为它可以逐渐沉淀：

```text
用户经常要求什么
哪些表达经常被删
什么 Hook 用户偏好
什么结构经常返工
```

---

### F. 内容经验沉淀

```text
content_learnings
```

建议字段：

```text
learning_id
learning_type
description
source_content_ids
confidence
scope
created_at
updated_at
```

例如：

```text
“海归招聘类视频中，问句 Hook 的平均表现优于直接陈述式开头。”
```

第一版不要求系统自动生成复杂经验。

可以先：

```text
人工写入
+
后续从历史数据中逐步沉淀
```

---

## 13.5.3 哪些 Agent 读取 Long-term Content Memory

### Topic Agent

可以查询：

```text
最近做过哪些选题
哪些主题已经重复
历史高表现选题
相似内容表现
近期账号内容分布
```

用于：

```text
避免重复
寻找高潜方向
保持账号内容节奏
```

---

### Research Agent

可以查询：

```text
过去同类招聘信息
历史来源
过去已核实企业
已有资料
```

用于：

```text
减少重复调查
复用可靠来源
发现旧信息是否已经失效
```

---

### Script Agent

可以查询：

```text
类似选题历史稿件
历史高表现脚本
常用 Hook
用户修改历史
最终采用版本
IP 长期表达习惯
```

用于：

```text
让新稿件越来越贴合账号
减少重复修改
复用有效结构
```

---

### Content Review Agent

可以查询：

```text
历史审核问题
用户常见修改点
账号长期表达风格
历史高风险表达
最终采用版本
```

用于：

```text
提高审核一致性
减少重复出现同类问题
```

---

### Supervisor

第一版不直接读取大量 Long-term Content Memory。

Supervisor 只需知道：

```text
当前是否存在可用长期记忆
专业 Agent 是否已经调用长期记忆 Tool
当前任务是否需要更多历史信息
```

长期内容数据应由专业 Agent 按需查询。

---

## 13.5.4 Long-term Content Memory Tools

建议增加以下 Function Tools：

```text
search_content_history
get_recent_topics
get_high_performing_content
get_similar_scripts
get_revision_history
get_ip_content_profile
save_content_item
save_revision_record
save_publish_metrics
update_content_metrics
save_content_learning
```

---

## 13.5.5 search_content_history

用途：

```text
按关键词 / 标签 / 时间 / 内容类型查询历史内容
```

典型调用：

```text
Topic Agent：
“过去 30 天做过哪些海归金融秋招内容？”
```

---

## 13.5.6 get_recent_topics

用途：

```text
查看近期已经生产或发布的选题
```

用于：

```text
避免重复
判断内容密度
保持选题多样性
```

---

## 13.5.7 get_high_performing_content

用途：

```text
查询历史表现较好的内容
```

可以按照：

```text
平台
内容类型
主题
时间范围
指标
```

过滤。

第一版不要求 Tool 自己定义“什么叫好内容”。

Tool 只返回：

```text
历史数据
```

“是否值得借鉴”由 Agent 根据 Principles 判断。

---

## 13.5.8 get_similar_scripts

用途：

```text
根据当前选题查询相似历史稿件
```

返回：

```text
历史稿件
最终版本
表现数据
标签
```

---

## 13.5.9 get_revision_history

用途：

```text
查询用户过去如何修改类似稿件
```

例如：

```text
开头过长
广告感太强
信息密度不够
表达太正式
CTA 太硬
```

Script Agent 可以借此减少重复犯错。

---

## 13.5.10 save_content_item

当一条内容真正被确认使用时：

```text
保存最终内容资产
```

不要在每次草稿生成时都直接写入长期内容库。

长期库应优先保存：

```text
被选中的选题
确认稿
正式生成内容
最终发布内容
```

草稿版本单独进入 script_versions。

---

## 13.5.11 save_revision_record

每次用户对稿件作出明确修改时：

```text
记录修改前
修改后
用户要求
是否接受
```

这部分是长期个性化的重要来源。

---

## 13.5.12 update_content_metrics

发布后，如果可以获取内容数据：

```text
持续更新表现
```

例如：

```text
发布后 1 天
发布后 3 天
发布后 7 天
```

具体更新频率由现有数据能力决定。

第一版可以人工触发或定时更新。

---

## 13.5.13 长期 Memory 的写入原则

不是所有 Agent 输出都写进长期 Memory。

长期 Memory 只保存：

```text
真正具有复用价值的稳定信息
```

优先写入：

```text
用户最终确认选题
最终采用稿件
真实发布内容
真实表现数据
明确用户修改反馈
经过验证的长期内容经验
```

不优先写入：

```text
临时候选
未采用稿件
模型猜测
未验证分析
一次性中间推理
```

---

## 13.5.14 Long-term Memory 与 Principles 的区别

二者不能混在一起。

```text
Principles
= 业务判断方法

Long-term Content Memory
= 历史事实和经验数据
```

例如：

```text
Principle：
“选题不能只看热度，还要考虑账号人群和时效。”

Memory：
“过去 30 天已经做过 4 条金融秋招内容。”
```

Agent 结合两者做判断。

---

## 13.5.15 第一版技术实现

第一版推荐继续使用：

```text
SQLite
```

可以将：

```text
Session
Task State
Long-term Content Memory
```

都放在同一个 SQLite 数据库中，但使用不同表。

例如：

```text
sessions
content_tasks
topic_history
content_items
script_versions
revision_history
content_metrics
content_learnings
```

逻辑上严格分层。

---

## 13.5.16 第一版不需要向量数据库

第一版检索需求主要是：

```text
时间
关键词
标签
主题
账号
内容类型
历史表现
```

SQLite + 普通结构化查询即可。

如果后续需要：

```text
“找语义上和这条稿子最像的历史内容”
```

再考虑：

```text
embedding / vector search
```

第一版不作为必要依赖。

---

## 13.5.17 Memory 的更新闭环

完整闭环：

```text
Agent 读取长期内容库
↓
生成 / 修改新内容
↓
用户确认
↓
数字人生成
↓
内容发布
↓
回收表现数据
↓
写回长期内容库
↓
下一次 Agent 再读取
```

这使系统从：

```text
“每次重新写一条内容”
```

逐渐变成：

```text
“越来越了解这个账号的内容生产 Agent”
```

---

## 13.5.18 Memory MVP 验收

第一版至少做到：

```text
Topic Agent 可以查最近选题
Script Agent 可以查历史相似稿件
Script Agent 可以查用户修改历史
最终稿可以写入长期内容库
发布表现可以后续更新
```

不用第一版就实现复杂自动学习。


# 14. Application Context

Agents SDK 的 Runtime Context 用于给：

```text
Agent runtime
Function Tool
Hooks
Callbacks
```

共享应用依赖和状态访问能力。

建议定义：

```python
@dataclass
class ContentAppContext:
    session_id: str
    user_id: str | None
    task_store: ContentTaskStore
    search_service: SearchService
    digital_human_service: DigitalHumanService
```

Context 不直接等于：

```text
LLM 看见的 Prompt。
```

它主要供程序和 Tools 使用。

---

# 15. ContentTaskState

建议：

```python
class ContentTaskState(BaseModel):
    session_id: str

    current_goal: str | None = None
    current_stage: str | None = None

    topic_candidates: list[TopicCandidate] = []
    selected_topic_id: str | None = None
    confirmed_topic: TopicCandidate | None = None

    research_results: list[ResearchItem] = []
    verified_sources: list[SourceItem] = []

    current_draft: Draft | None = None
    draft_versions: list[Draft] = []
    approved_draft_version: str | None = None

    review_result: ReviewResult | None = None

    generation_task_id: str | None = None
    generation_status: str | None = None
    generated_video_url: str | None = None
```

---

# 16. Task Store

第一版建议：

```text
SQLite
```

理由：

```text
本地部署
实现简单
可持久化
便于作品集演示
重启后状态仍存在
```

不需要第一版上：

```text
PostgreSQL
Redis
向量数据库
```

---

# 17. Agent 如何读取业务 State

RunContext 本身不会自动变成 LLM 的可见上下文。

因此建议提供：

```text
get_current_task_state Tool
```

或者：

```text
在 Supervisor 每轮运行前，
由后端将当前关键 State 摘要放入输入。
```

推荐：

```text
Supervisor 输入
=
用户最新消息
+
简洁的 current task state summary
```

例如：

```text
Current task state:
- 当前候选选题：5 个
- 用户上一轮选择：topic_03
- confirmed_topic：香港仍在招聘的金融企业
- current_draft：v2
- approved_draft_version：null
- generation_status：null
```

专业 Agent 则按需通过 Tool 读取详细状态。

---

# 18. Structured Outputs

专业 Agent 的输出尽量使用：

```text
output_type
+
Pydantic Schema
```

而不是让模型自由输出 JSON 再手工解析。

---

# 19. Topic Agent Schema

```python
class TopicCandidate(BaseModel):
    topic_id: str
    title: str
    why_now: str
    target_audience: str
    content_angle: str
    key_points: list[str]
    sources: list[str]
    freshness: str
    risks: list[str]

class TopicResult(BaseModel):
    summary: str
    topic_candidates: list[TopicCandidate]
    research_needed: list[str]
    uncertainties: list[str]
```

---

# 20. Research Agent Schema

```python
class SourceItem(BaseModel):
    title: str
    url: str
    source_type: str
    published_at: str | None
    verified_at: str | None

class ResearchResult(BaseModel):
    summary: str
    facts: list[str]
    sources: list[SourceItem]
    verified_items: list[str]
    not_verified: list[str]
    open_questions: list[str]
```

---

# 21. Script Agent Schema

```python
class Draft(BaseModel):
    draft_id: str
    version: str
    script: str
    hook: str
    body: str
    cta: str
    sources_used: list[str]
    uncertainties: list[str]
```

---

# 22. Content Review Schema

```python
class ReviewResult(BaseModel):
    status: Literal["pass", "revise", "need_research"]
    summary: str
    issues: list[str]
    must_fix: list[str]
    risk_items: list[str]
    revision_request: str | None
    research_request: str | None
    ready_for_generation: bool
```

---

# 23. Supervisor 是否需要结构化输出

如果 Supervisor 直接面向用户回复：

```text
不要求每次最终输出都是 Schema。
```

但需要保证：

```text
内部 specialist tools 返回结构化数据。
```

Supervisor 最终负责：

```text
把 Agent 结果整理成自然语言回复。
```

例如 Topic Agent 返回结构化 5 个选题。

Supervisor 再呈现为：

```text
1.
2.
3.
```

---

# 24. Skills / Principles

Agents SDK 本身不会替你定义业务 Skill。

本项目继续采用独立 Markdown Principles。

建议：

```text
principles/
├── topic.md
├── research.md
├── script.md
└── content_review.md
```

Agent instructions 动态加载对应 Principles。

---

# 25. Dynamic Instructions

每个 Agent 的 instructions 可以由程序动态构建。

例如：

```text
基础角色说明
+
Agent 边界
+
对应 Principles
+
当前必要上下文
```

不要：

```text
把所有 Agent 的所有 Skill 塞进一个巨型 Prompt。
```

---

# 26. Topic Principles

由项目负责人填写：

```text
什么是好选题
什么值得做
什么属于高时效
什么不值得做
什么算重复
爆款能否跟
账号人群
内容方向
什么情况下需要调研
```

---

# 27. Research Principles

由项目负责人填写：

```text
什么来源可信
官方来源优先级
怎样判断仍在招聘
不同来源冲突如何处理
哪些信息必须核实
哪些二手内容可以作为线索
什么情况下不能确认
```

---

# 28. Script Principles

由项目负责人填写：

```text
IP 人设
语气
Hook
前三秒
前 15 秒
信息密度
节奏
广告感
内容结构
CTA
常见禁忌
```

---

# 29. Content Review Principles

由项目负责人填写：

```text
通过标准
必须修改问题
事实风险
内容风险
平台风险
人设偏离
过度承诺
表达拖沓
什么情况退回 Script
什么情况退回 Research
```

---

# 30. “今天做啥好呢”的运行方式

用户：

```text
今天做啥好呢
```

Supervisor 根据 Session 知道：

```text
账号是谁
之前聊过什么
```

根据 Task State 知道：

```text
当前是否正在做一条内容
```

如果是新任务：

Supervisor 可以：

```text
调用 Topic Agent
```

Topic Agent：

```text
自己决定调用哪些 Tool
```

例如：

```text
search_platform_content
search_recruitment
get_content_history
```

如果信息不够，Topic Agent返回：

```text
research_needed
```

Supervisor可以再调用：

```text
Research Agent
```

然后再次调用：

```text
Topic Agent
```

整个路径不是代码预先固定。

---

# 31. “这个方向可以，但换几个还在招聘的”

Session 提供：

```text
上一轮候选
```

Task State提供：

```text
selected_topic
```

Supervisor理解：

```text
保留 topic direction
修改 entity candidates
要求当前仍在招聘
```

Supervisor可以直接：

```text
调用 Research Agent
```

Research Agent搜索并核实。

必要时：

```text
Supervisor再调用 Topic Agent
```

重组内容方向。

---

# 32. “写吧”

Supervisor必须先识别：

```text
当前是否有 confirmed_topic
```

如果有：

```text
调用 Script Agent
```

如果只是有候选但没有明确选中：

Supervisor应根据对话判断：

```text
“写吧”是否明确指向某个候选。
```

例如上一轮用户说：

```text
第三个不错
```

这一轮：

```text
写吧
```

可以自然推断：

```text
第三个为 confirmed_topic
```

不需要机械重复确认。

---

# 33. “略微改一下”

Script Agent需要：

```text
读取 current_draft
```

根据用户反馈：

```text
局部修改
```

保存：

```text
v2
v3
v4
```

每次修改后：

```text
save_draft_version
```

---

# 34. “生成吧”

这是本项目重要的 HITL 点。

用户明确说：

```text
生成吧
就这版
可以生成
```

视为：

```text
用户已经授权生成当前版本
```

然后调用：

```text
generate_digital_human
```

不需要再次问：

```text
“是否确定生成？”
```

除非当前状态有歧义。

---

# 35. Agents SDK HITL

Agents SDK 支持 Tool Approval / Run interruption。

本项目可以把数字人生成 Tool 设计为：

```text
可审批 Tool
```

但第一版建议：

```text
显式自然语言授权
+
应用状态
```

决定是否需要额外 approval。

例如：

```text
latest user intent = generate
current draft明确
```

则：

```text
无需重复审批
```

如果：

```text
用户只是说“差不多了”
```

但 Supervisor准备执行生成：

```text
要求用户确认
```

---

# 36. Digital Human Tool

示意：

```python
@function_tool
async def generate_digital_human(
    ctx: RunContextWrapper[ContentAppContext],
    draft_version: str,
) -> DigitalHumanTask:
    ...
```

Tool内部：

```text
读取对应稿件
调用现有数字人 API
保存 task_id
返回状态
```

---

# 37. 数字人状态查询

另一 Tool：

```text
get_generation_status
```

如果数字人 API 是异步：

```text
提交生成
↓
返回 task_id
↓
前端轮询
↓
完成
```

不要让 LLM 自己长时间等待。

---

# 38. Runner

每次用户发消息：

```text
后端收到 user_message
↓
加载 session
↓
加载 ContentTaskState
↓
构建 ContentAppContext
↓
Runner.run(supervisor, ...)
↓
保存状态
↓
返回 final_output
```

---

# 39. Streaming

前端建议使用：

```text
Runner.run_streamed()
```

这样用户可以看到：

```text
正在调研…
正在整理选题…
正在生成稿件…
```

而不必一直等待最终完整结果。

---

# 40. 前端 Agent 工作动画

前端“小牛马工作动画”不是 Agent 框架本身。

它由：

```text
SDK lifecycle events
Tool events
Agent tool calls
```

驱动。

前端可以展示：

```text
选题员正在工作
调研员正在搜索
写稿员正在写稿
审核员正在检查
数字人正在生成
```

---

# 41. 前端状态事件

后端建议统一发送：

```json
{
  "type": "agent_status",
  "agent": "research",
  "status": "working",
  "message": "正在核实招聘状态"
}
```

其他：

```text
agent_started
agent_finished
tool_started
tool_finished
draft_updated
waiting_user
generation_started
generation_completed
```

---

# 42. Hooks

Agents SDK 提供生命周期 Hooks。

本项目可以通过 Hooks记录：

```text
Agent start
Agent end
Tool start
Tool end
LLM start
LLM end
```

这些既可以：

```text
写日志
```

也可以：

```text
推给前端动画
```

---

# 43. Tracing

Agents SDK 内置 Tracing。

第一版直接使用内置 Trace。

可以看到：

```text
LLM generation
Tool call
Agent-as-tool
Guardrail
Run
```

作品集里非常适合展示：

```text
真实一次内容生产 Trace
```

---

# 44. 自定义业务 Trace

除了 SDK Trace，建议本地再保存：

```text
session_id
user_message
specialist agent calls
tool calls
topic selection
draft version changes
generation task
latency
error
```

因为作品集前端可能需要直接读取这些数据。

---

# 45. Guardrails

第一版 Guardrail 重点用于：

```text
输入安全
Tool 参数
输出基本边界
```

不要让 Guardrail 变成第二套业务判断 Agent。

例如：

```text
Content Review Agent
负责业务内容审核

Guardrail
负责技术安全边界
```

---

# 46. Agent 边界

## Supervisor

负责：

```text
用户交互
理解上下文
调度
整合结果
决定继续或返回用户
```

## Topic Agent

负责：

```text
选题
内容方向
候选比较
```

## Research Agent

负责：

```text
搜索
核实
资料
来源
```

## Script Agent

负责：

```text
写稿
改稿
版本
```

## Content Review Agent

负责：

```text
审核
风险
返工建议
```

---

# 47. 不需要再单独做 Graph State Machine

迁移到 Agents SDK 后：

```text
不再使用 LangGraph Node / Edge / StateGraph
```

编排主要由：

```text
Supervisor Agent
+
Agents-as-tools
+
Runner
+
Sessions
+
Application State
```

共同完成。

---

# 48. 自主性来自哪里

自主性不是：

```text
框架帮你写了一个自动流程
```

而是：

```text
Supervisor看到多个可用专业 Agent
↓
根据当前任务自己决定调用谁

专业 Agent看到多个 Tool
↓
根据当前问题自己决定调用哪个

Agent拿到结果
↓
自己判断是否还需要继续调查
```

程序只提供：

```text
能力边界
状态
调用安全
```

---

# 49. 模型选择

Agents SDK 默认最适合 OpenAI 模型。

但本项目可以保留模型层可配置。

建议第一版：

```text
Supervisor：
高质量 reasoning model

Topic / Research：
通用模型

Script：
可以继续使用你当前验证过的 DeepSeek 路线

Review：
通用模型
```

但第一版也可以为了简单：

```text
所有 Agent 使用同一个模型
```

跑通以后再拆。

---

# 50. 如果继续使用 DeepSeek

Agents SDK支持非 OpenAI 模型接入方式。

可选择：

```text
OpenAI-compatible client
ModelProvider
Agent.model
第三方 adapter
```

如果 DeepSeek 使用 OpenAI-compatible Chat Completions：

```text
可以尝试直接接入。
```

但必须实际验证：

```text
tool calling
structured outputs
usage
streaming
```

兼容情况。

如果某些高级能力不稳定：

```text
Supervisor / Tool-heavy Agent 使用 OpenAI 模型
Script Agent 保留 DeepSeek
```

即可。

---

# 51. 推荐的第一版模型策略

为了减少开发变量：

```text
先统一模型完成 Agent 架构
```

成功后再做：

```text
Script Agent → DeepSeek
其他 Agent → 原模型
```

不要在系统还没跑通时同时处理：

```text
Multi-Agent
+
多模型兼容
+
前端
+
数字人 API
```

四个变量。

---

# 52. 前端

前端值得保留。

作品集需要看到：

```text
这是一个产品
```

而不是：

```text
只有 Python terminal。
```

---

# 53. 前端布局

建议双栏。

左侧：

```text
聊天区
```

右侧：

```text
当前任务
当前选题
当前稿件
审核状态
生成状态
Agent 工作状态
```

---

# 54. 聊天区

支持：

```text
文字消息
选题卡片
来源链接
稿件
视频结果
```

---

# 55. Agent 工作区

可以做成：

```text
Topic
Research
Script
Review
```

四个小角色。

状态：

```text
idle
working
done
waiting
error
```

---

# 56. “小牛马工作动画”

可以纯前端实现。

例如：

```text
Research Agent start
↓
小人走到电脑前
↓
显示“正在搜索”

Tool call
↓
出现搜索动画

Research Agent end
↓
显示“已找到 8 条信息”
```

不影响后端 Agent 架构。

---

# 57. 现有网页如何迁移

不用推倒。

原有：

```text
选题功能
稿件功能
数字人功能
```

全部保留底层函数。

前端逐步增加：

```text
/chat
/session
/task-state
/events
```

---

# 58. API 建议

```text
POST /api/chat
GET  /api/sessions/{id}
GET  /api/tasks/{session_id}
GET  /api/events/{session_id}
GET  /api/generation/{task_id}
```

如果使用 WebSocket / SSE：

```text
/api/chat/stream
```

---

# 59. 目录结构建议

```text
src/
├── agent_system/
│   ├── agents/
│   │   ├── supervisor.py
│   │   ├── topic.py
│   │   ├── research.py
│   │   ├── script.py
│   │   └── review.py
│   │
│   ├── schemas/
│   │   ├── topic.py
│   │   ├── research.py
│   │   ├── draft.py
│   │   └── review.py
│   │
│   ├── tools/
│   │   ├── search.py
│   │   ├── recruitment.py
│   │   ├── content_history.py
│   │   ├── drafts.py
│   │   └── digital_human.py
│   │
│   ├── principles/
│   │   ├── topic.md
│   │   ├── research.md
│   │   ├── script.md
│   │   └── content_review.md
│   │
│   ├── services/
│   │   ├── task_store.py
│   │   ├── session_service.py
│   │   ├── model_factory.py
│   │   └── event_bus.py
│   │
│   ├── context.py
│   ├── runner.py
│   └── hooks.py
│
├── web/
└── existing_modules/
```

---

# 60. 当前已有能力复用

## 搜索

现有搜索：

```text
改造成 Function Tool。
```

## 写稿

现有 DeepSeek：

```text
可以作为 Script Agent 底层模型
或保留为现有服务供 Script Agent 使用。
```

## 数字人

现有数字人 API：

```text
改造成 Function Tool。
```

---

# 61. 开发顺序

---

## Step 0｜冻结现有能力

Codex先找：

```text
现有搜索入口
现有 DeepSeek 写稿入口
现有数字人 API
现有前端
现有数据结构
```

---

## Step 1｜安装 Agents SDK

加入：

```text
openai-agents
pydantic
```

如果使用 SQLite Session：

```text
使用 SDK 对应 Session 实现。
```

---

## Step 2｜定义 Pydantic Schemas

先完成：

```text
TopicResult
ResearchResult
Draft
ReviewResult
ContentTaskState
```

---

## Step 3｜定义 ContentAppContext

加入：

```text
session_id
task_store
search_service
digital_human_service
```

---

## Step 4｜封装 Function Tools

优先复用现有能力：

```text
search_platform_content
search_recruitment
fetch_source
get_content_history
get_current_task_state
save_topic_selection
save_draft_version
generate_digital_human
get_generation_status
```

---

## Step 5｜建立 Task Store + Long-term Content Memory

第一版统一使用：

```text
SQLite
```

Task Store 保存：

```text
当前选题
当前调研
当前稿件
当前版本
生成状态
```

Long-term Content Memory 保存：

```text
历史选题
最终稿件
稿件版本
用户修改记录
发布表现
内容经验
```

同时实现第一批 Memory Tools：

```text
search_content_history
get_recent_topics
get_similar_scripts
get_revision_history
save_content_item
save_revision_record
update_content_metrics
```

---

## Step 6｜Topic Agent

实现：

```text
Principles
Tools
output_type
```

先验证：

```text
今天做啥好呢
```

---

## Step 7｜Research Agent

实现：

```text
最新信息搜索
招聘核实
来源整理
```

---

## Step 8｜Script Agent

实现：

```text
生成
局部修改
版本
```

---

## Step 9｜Content Review Agent

实现：

```text
审核
返工
需要补资料
```

---

## Step 10｜Supervisor

把 4 个 Agent：

```text
as_tool()
```

挂给 Supervisor。

Supervisor同时拥有必要基础 Tools。

---

## Step 11｜Session

使用：

```text
SQLiteSession
```

验证：

```text
这个
第三个
写吧
改一下
生成吧
```

能保持连续上下文。

---

## Step 12｜Runner

封装统一入口：

```python
async def run_content_assistant(
    session_id: str,
    user_message: str,
):
    ...
```

---

## Step 13｜Streaming

支持：

```text
run_streamed
```

供前端显示实时状态。

---

## Step 14｜Hooks + Events

将：

```text
Agent start/end
Tool start/end
```

映射为：

```text
前端工作动画事件
```

---

## Step 15｜数字人 Tool

接现有 API。

---

## Step 16｜HITL

处理：

```text
选题确认
生成授权
```

---

## Step 17｜Tracing

启用 SDK Trace。

---

## Step 18｜前端聊天框

连接：

```text
Conversation API
```

---

## Step 19｜Agent 状态动画

接：

```text
stream / event bus
```

---

## Step 20｜真实 Case 测试

---

# 62. MVP Case

至少跑通：

---

## Case 1

```text
用户：
今天做啥好呢
```

系统：

```text
自主调用 Topic
搜索
必要时 Research
返回 3～5 个选题
```

---

## Case 2

```text
用户：
第三个方向可以，但换几个还在招聘的
```

系统：

```text
保留方向
Research
重新组织候选
```

---

## Case 3

```text
用户：
写吧
```

系统：

```text
理解当前 confirmed topic
调用 Script
```

---

## Case 4

```text
用户：
第二段短一点
```

系统：

```text
修改当前 Draft
保存新版
```

---

## Case 5

```text
用户：
这个稿子有没有风险
```

系统：

```text
调用 Review
```

---

## Case 6

```text
用户：
生成吧
```

系统：

```text
调用数字人 Tool
```

---

# 63. Evaluation

评估：

```text
对话理解
状态连续性
Agent 自主调用
Tool 使用
事实可靠性
稿件质量
Agent 边界
生成授权
```

---

# 64. 自主性验收

必须出现：

```text
不同输入
→ 不同 Specialist 调用
```

例如：

```text
“第三个现在还招吗？”
→ Research
```

而：

```text
“第三个写一版”
→ Script
```

而：

```text
“这个稿有没有问题？”
→ Review
```

而：

```text
“今天做啥？”
→ Topic / Research
```

不能所有输入都走同一固定链路。

---

# 65. Tool 自主性验收

Topic / Research 不应：

```text
每次机械调用全部 Tool。
```

应该：

```text
根据问题选择必要 Tool。
```

---

# 66. 状态验收

必须正确理解：

```text
这个
第三个
刚才那个
第二段
这版
生成吧
```

---


# 66.5 Long-term Memory 验收

必须验证：

```text
新会话开始后仍然可以查询历史选题
Script Agent 可以读取过去最终稿
Script Agent 可以读取用户历史修改记录
最终采用稿可以写入长期内容库
内容发布数据可以后续更新
不同 Session 之间长期内容数据仍然存在
```

同时检查：

```text
临时候选没有被错误写入长期 Memory
未验证模型推测没有被写入长期 Memory
```

---

# 67. 边界验收

Supervisor：

```text
不替专业 Agent 干全部工作
```

Topic：

```text
不编造事实
```

Research：

```text
不替用户决定最终内容
```

Script：

```text
不编造未验证招聘信息
```

Review：

```text
不擅自替用户生成数字人
```

---

# 68. Tracing 展示

作品集中展示一次真实 Trace：

```text
Supervisor
↓
topic_expert
↓
search
↓
research_expert
↓
topic_expert
↓
用户确认
↓
script_expert
↓
用户修改
↓
content_reviewer
↓
digital_human
```

---

# 69. 作品集前端展示

前台可以同时展示：

```text
聊天
当前内容
Agent 工作动画
实时 Trace
最终数字人
```

这会比只有聊天框更容易让面试官理解：

```text
这真的是 Multi-Agent，
不是一个大 Prompt。
```

---

# 70. 本期不做

```text
自动发布
自动账号运营
批量无人化生产
评论自动回复
长期复杂 Memory
向量数据库
多租户权限
高并发平台
复杂工作流引擎
```

---

# 71. 与投放诊断 Agent 的技术区别

投放诊断：

```text
LangGraph
```

适合：

```text
复杂 State
补证据循环
明确节点控制
复杂 workflow + agent 混合
```

内容生产：

```text
OpenAI Agents SDK
```

适合：

```text
Supervisor
Specialists
Tools
Sessions
连续聊天
轻量自主协作
```

---

# 72. 两个项目共同能力

共同：

```text
业务 Principles
专业 Agent
Tool
结构化输出
Trace
人机协作
```

区别：

```text
一个展示复杂 orchestration
一个展示对话式 Agent 产品
```

---

# 73. 一句话技术叙事

> 在已有 AI 选题、DeepSeek 口播稿生成和数字人生产能力基础上，使用 OpenAI Agents SDK 将原有按钮驱动工具升级为对话式 Multi-Agent 内容生产系统。采用 Manager 模式，由 Supervisor 始终保持用户会话控制权，并通过 Agents-as-tools 自主调用 Topic、Research、Script 和 Content Review 等专业 Agent；结合 Sessions 管理连续对话、结构化 Task State 管理选题与稿件版本、Function Tools 复用搜索和数字人 API，并通过 Tracing 展示多 Agent 实际协作过程。

---

# 74. 最终定义

本项目的目标不是：

```text
做一个更复杂的写稿工具。
```

而是：

```text
让用户只通过对话完成一整条内容任务。
```

用户只需要说：

```text
今天做啥
换几个
第三个
写吧
改一下
这个可以
生成吧
```

系统自己负责：

```text
理解
调度
调查
写作
审核
状态维护
长期内容记忆
工具调用
生成执行
```

这就是第一版 OpenAI Agents SDK 内容生产 Multi-Agent 的完整形态。

---

# 75. 官方技术参考

开发时优先以 OpenAI Agents SDK 官方文档为准：

- Agents SDK Overview:
  https://openai.github.io/openai-agents-python/

- Agents / Multi-agent patterns:
  https://openai.github.io/openai-agents-python/agents/

- Tools / Agents as tools:
  https://openai.github.io/openai-agents-python/tools/

- Sessions:
  https://openai.github.io/openai-agents-python/sessions/

- Context:
  https://openai.github.io/openai-agents-python/context/

- Human-in-the-loop:
  https://openai.github.io/openai-agents-python/human_in_the_loop/

- Tracing:
  https://openai.github.io/openai-agents-python/tracing/

- Models / Non-OpenAI providers:
  https://openai.github.io/openai-agents-python/models/
