# 自主编程代理编排系统全景对比报告

> 调研日期: 2026-03-09
> 调研目的: 为 ccclaw 项目(以 GitHub Issue 为异步入口、Claude Code 为执行体、systemd 为调度层、kb 为长期记忆)寻找竞品和参考架构
> 调研人: CCoder

---

## 一、总览矩阵

| 项目 | GitHub URL | Stars | 许可证 | 异步任务 | Issue 驱动 | 多模型 | 记忆持久化 | 守护进程/后台 | 维护状态 |
|------|-----------|-------|--------|---------|-----------|--------|-----------|-------------|---------|
| **OpenClaw** | [openclaw/openclaw](https://github.com/openclaw/openclaw) | ~247k | MIT | 部分 | 否(消息驱动) | 是 | 是(Gateway 记忆) | 是(Gateway 常驻) | 活跃(已移交基金会) |
| **OpenAI Symphony** | [openai/symphony](https://github.com/openai/symphony) | 新发布 | Apache-2.0 | **是** | **是(Linear)** | 是 | **是(PostgreSQL)** | **是(Elixir 守护进程)** | 活跃(2026-03 刚发布) |
| **GitHub Copilot Coding Agent** | 闭源(GitHub 平台) | N/A | 商业 | **是** | **是(Issue/Jira)** | 是(Claude/Codex/Copilot) | 否 | **是(GitHub Actions)** | 活跃(GA) |
| **SWE-agent** | [SWE-agent/SWE-agent](https://github.com/SWE-agent/SWE-agent) | ~20k+ | MIT | 批量模式 | **是(GitHub Issue)** | 是 | 否 | 否(CLI 一次性) | 活跃(NeurIPS 2024) |
| **OpenHands** | [OpenHands/OpenHands](https://github.com/OpenHands/OpenHands) | ~65k | MIT | 部分 | 部分 | 是 | 有限(事件日志) | Docker 容器化 | 活跃 |
| **Devin** | 闭源(cognition.ai) | N/A | 商业 | **是** | **是(PR 驱动)** | 是(内部多模型) | **是** | **是(云端 VM)** | 活跃(Devin 2.0) |
| **Cursor Agent** | 闭源(cursor.com) | N/A | 商业 | **是(Cloud Agent)** | **是(Linear/GitHub)** | 是 | 部分 | **是(Cloud VM)** | 活跃 |
| **Aider** | [Aider-AI/aider](https://github.com/Aider-AI/aider) | ~39k | Apache-2.0 | 否 | 否 | 是 | 否(依赖 git) | 否(交互式 CLI) | 活跃 |
| **Cline** | [cline/cline](https://github.com/cline/cline) | ~59k | Apache-2.0 | 有限(-y 标志) | 否 | 是 | 否 | 否(VS Code 插件) | 活跃 |
| **Goose** | [block/goose](https://github.com/block/goose) | ~15k+ | Apache-2.0 | 否 | 否 | 是(MCP) | 否 | 有(goosed 服务) | 活跃 |
| **AutoCodeRover** | [AutoCodeRoverSG/auto-code-rover](https://github.com/AutoCodeRoverSG/auto-code-rover) | ~5k+ | GPL-3.0 | 批量模式 | **是(GitHub Issue)** | 是 | 否 | 否 | 学术项目 |
| **Open SWE** | [langchain-ai/open-swe](https://github.com/langchain-ai/open-swe) | 新项目 | 开源 | **是** | **是(自动创建)** | 是 | 是(LangGraph) | **是(云端)** | 活跃 |
| **Claude Agent SDK** | [anthropics/claude-agent-sdk-python](https://github.com/anthropics/claude-agent-sdk-python) | 新发布 | Anthropic | 是(可编程) | 可扩展 | Claude 系列 | 可扩展(Hooks) | 可编程 | 活跃(2026-03) |
| **Taskmaster (claude-task-master)** | [eyaltoledano/claude-task-master](https://github.com/eyaltoledano/claude-task-master) | ~21k | MIT + Commons Clause | 部分(loop 命令) | 否(PRD 驱动) | 是 | 是(JSON 文件) | 否(MCP 服务) | 活跃 |
| **GPT-Engineer** | [AntonOsika/gpt-engineer](https://github.com/AntonOsika/gpt-engineer) | ~55k | MIT | 否 | 否 | 部分 | 否 | 否 | 低活跃(转向 Lovable) |
| **Mentat** | [AbanteAI/mentat](https://github.com/AbanteAI/mentat) | ~3k+ | Apache-2.0 | 否 | 否 | 有限(GPT-4) | 否 | 否 | 低活跃 |
| **Smol Developer** | [smol-ai/developer](https://github.com/smol-ai/developer) | ~12k | MIT | 否 | 否 | 部分 | 否 | 否 | 低活跃 |
| **mini-claude-code** | [shareAI-lab/mini-claude-code](https://github.com/shareAI-lab/mini-claude-code) | 教学项目 | 开源 | 否 | 否 | 否 | 否 | 否 | 活跃(教学) |
| **Sweep** | [sweepai/sweep](https://github.com/sweepai/sweep) | ~8k+ | 开源 | **是** | **是(GitHub Issue)** | 部分 | 否 | **是(GitHub Bot)** | 低活跃 |
| **Windsurf (Codeium)** | 闭源(windsurf.com) | N/A | 商业($15/月) | 部分 | 否 | 是 | 部分 | 否(IDE) | 活跃 |

---

## 二、逐项目深度分析

### 1. OpenClaw (原 Clawdbot / Moltbot)

- **GitHub**: https://github.com/openclaw/openclaw
- **Stars**: ~247,000 (截至 2026-03, GitHub 历史增长最快之一)
- **许可证**: MIT
- **创建者**: Peter Steinberger (奥地利开发者, 2026-02 加入 OpenAI, 项目移交开源基金会)

**核心架构**:
- **Gateway**: 中央控制平面, 所有会话/路由/通道经此分发, WebSocket 协议 (ws://127.0.0.1:18789)
- **Skills 系统**: 插件化能力扩展, 分内置/ClawHub 托管/本地三类, 每个 Skill 由 SKILL.md 定义
- **ClawHub**: 公共 Skills 注册表, 截至 2026-02 已有 13,729+ 社区 Skills
- **记忆系统**: 跨会话持续性, Gateway 统一管理上下文

**关键差异化**:
- 面向"个人 AI 助手"定位, 支持 20+ 通道(WhatsApp/Telegram/Slack/Discord/iMessage/IRC 等)
- Skills 生态系统极其丰富
- 不以编码为唯一用途, 是通用 AI 代理平台

**与 ccclaw 对比**:
- OpenClaw 是消息通道驱动, ccclaw 是 GitHub Issue 驱动 -- 切入点不同
- OpenClaw 没有 systemd 调度, 用自己的 Gateway 守护进程
- OpenClaw 的 Skills 概念可借鉴为 ccclaw 的能力扩展机制
- OpenClaw 不针对软件工程任务优化

---

### 2. OpenAI Symphony (最高相关度)

- **GitHub**: https://github.com/openai/symphony
- **Stars**: 新发布(2026-03-04)
- **许可证**: Apache-2.0
- **状态**: 项目预览阶段

**核心架构**:
- **守护进程**: Elixir/BEAM 运行时, 天然支持容错和并发
- **Issue Tracker 轮询**: 持续轮询 Linear 中的任务(计划支持更多 Tracker)
- **隔离工作区**: 每个 Issue 创建独立目录, 代理操作被约束在其中
- **状态持久化**: PostgreSQL (通过 Ecto)
- **工作流版本化**: WORKFLOW.md 存放在仓库中, 代理行为随分支同步

**工作流程**: Todo -> In Progress -> Human Review -> Merging -> Done

**关键差异化**:
- **这是 ccclaw 最直接的竞品/参考** -- 同样是 daemon + issue tracker + 隔离执行
- 用 Linear 而非 GitHub Issues, 但核心理念几乎一致
- 用 Elixir 而非 systemd, 但都是长驻进程模式
- 提供"证明材料": CI 状态、PR 审查反馈、复杂度分析、演示视频

**与 ccclaw 对比**:
- Symphony 用 Linear, ccclaw 用 GitHub Issues -- ccclaw 对开源项目更友好
- Symphony 用 Elixir 自建 daemon, ccclaw 用 systemd -- ccclaw 更贴近 Linux 基础设施
- Symphony 用 PostgreSQL 持久化, ccclaw 用 kb 文件系统 -- 各有优劣
- Symphony 刚发布, 生态未成型; ccclaw 可以快速吸收其规范

---

### 3. GitHub Copilot Coding Agent

- **URL**: https://github.com/features/copilot (闭源平台)
- **许可证**: 商业(Copilot Pro $10/月, Business $19/月, Enterprise $39/月)
- **状态**: GA(2026-02 全面上线)

**核心架构**:
- GitHub Actions 驱动的云端隔离环境
- 异步后台运行, 推送 commits 到 draft PR
- 支持 Issue 分配、Jira Issue 分配(2026-03 公开预览)
- 内置安全扫描、自我审查、依赖检查

**关键差异化**:
- 原生 GitHub 集成, 零配置
- 支持 Claude / Codex / Copilot 多模型选择
- 自动 code review 后再开 PR

**与 ccclaw 对比**:
- Copilot 是闭源商业服务, ccclaw 是自托管开源
- Copilot 深度绑定 GitHub 生态, ccclaw 更灵活
- ccclaw 可作为 Copilot Coding Agent 的自托管替代方案

---

### 4. SWE-agent (Princeton/Stanford)

- **GitHub**: https://github.com/SWE-agent/SWE-agent
- **Stars**: ~20k+
- **许可证**: MIT
- **状态**: 活跃, NeurIPS 2024 论文项目

**核心架构**:
- 输入 GitHub Issue, 自动尝试修复
- 自定义 Agent-Computer Interface (ACI) 优化 LLM 与代码库交互
- SWE-ReX: 沙盒化执行, 支持本地或云端大规模并行
- mini-swe-agent: 100 行 Python, SWE-bench verified >74%

**关键差异化**:
- 学术血统, SWE-bench 标杆评测
- 专注 GitHub Issue -> 修复 这一条路径
- 支持多模态(可处理 Issue 中的图片)

**与 ccclaw 对比**:
- SWE-agent 是一次性 CLI 执行, 不是守护进程
- 没有长期记忆/知识库
- 但其 "Issue -> 修复 -> PR" 的管线可以被 ccclaw 复用或参考

---

### 5. OpenHands (原 OpenDevin, All-Hands-AI)

- **GitHub**: https://github.com/OpenHands/OpenHands
- **Stars**: ~65k
- **许可证**: MIT (enterprise/ 目录除外)
- **状态**: 活跃(v1.4+, 2026-02)

**核心架构**:
- 沙盒化执行环境(Docker)
- 事件日志记录所有命令/编辑/结果
- 可修改代码、运行命令、浏览网页、调用 API
- SWE-bench 解决率 >50%

**关键差异化**:
- 最大的开源 Devin 替代品
- Docker 隔离, 安全性好
- 但跨会话不保持记忆 -- 每次对话从头开始

**与 ccclaw 对比**:
- OpenHands 缺乏跨会话记忆, ccclaw 的 kb 是核心优势
- OpenHands 不是守护进程模式
- OpenHands 的 Docker 沙盒方案可参考

---

### 6. Devin (Cognition)

- **URL**: https://devin.ai / https://cognition.ai
- **许可证**: 商业(闭源)
- **状态**: 活跃(Devin 2.0)

**核心架构**:
- 复合 AI 系统: 不是单一模型, 而是专业化模型集群编排
- Planner(高推理模型) + 执行模型分层
- 云端隔离 VM, 支持多实例并行
- 自动开 PR、写描述、响应 code review 评论

**关键差异化**:
- 可以摄入大规模遗留代码库(COBOL/Fortran/Objective-C)并重构为现代语言
- DeepWiki: 自动生成并持续更新的代码库文档
- 多模态: 处理 UI mockup / Figma / 录屏

**与 ccclaw 对比**:
- Devin 是商业闭源云服务, ccclaw 是自托管
- Devin 的"Planner + Executor"模式值得参考
- ccclaw 用 Claude Code 做执行体, 但可以增加 Planner 层

---

### 7. Cursor Agent / Cloud Agents

- **URL**: https://cursor.com
- **许可证**: 商业($20/月)
- **状态**: 活跃(2026-03 Automations 发布)

**核心架构**:
- Background Agents: 异步远程 VM 执行代码编辑/测试
- Cloud Agents(2026-02-24): 完全自主 AI 编码, VM 隔离
- Automations: 基于事件触发(GitHub PR/Linear Issue/Slack/PagerDuty)

**关键差异化**:
- 30% 的 Cursor 自身合并 PR 由 Cloud Agent 生成
- 数百个代理可协作处理同一代码库
- IDE 原生体验

**与 ccclaw 对比**:
- Cursor 的 Automations 事件触发模型类似 ccclaw 的 Issue 驱动
- ccclaw 更轻量(systemd + CLI vs. 完整 IDE)
- Cursor 闭源, ccclaw 完全可控

---

### 8. Aider

- **GitHub**: https://github.com/Aider-AI/aider
- **Stars**: ~39k
- **许可证**: Apache-2.0
- **状态**: 活跃(最老牌的开源 AI 编码 CLI)

**核心架构**:
- 终端交互式对话编程
- 自动 git commit, 感知完整仓库上下文
- 支持 100+ 编程语言
- 多文件并发编辑

**关键差异化**:
- 最成熟的终端 AI pair programming 工具
- git 原生, diff 友好
- 社区和模型支持最广

**与 ccclaw 对比**:
- Aider 是交互式工具, 不支持无人值守长时间运行
- 没有 Issue 驱动, 没有守护进程, 没有知识库
- 但其 "git-native" 理念和多模型支持可参考

---

### 9. Cline

- **GitHub**: https://github.com/cline/cline
- **Stars**: ~59k
- **许可证**: Apache-2.0
- **状态**: 活跃(Samsung 正在试点部署)

**核心架构**:
- VS Code 扩展, 具有完整终端/文件/浏览器访问能力
- Human-in-the-loop: 每步操作需用户批准(可用 -y 跳过)
- 支持 Computer Use(浏览器操作/截图)

**关键差异化**:
- VS Code 内最强大的自主编码代理
- 支持浏览器操作(视觉调试、端到端测试)
- 企业级定制能力

**与 ccclaw 对比**:
- Cline 是 IDE 插件, 不适合无头后台运行
- 没有 Issue 驱动, 没有守护进程
- Cline CLI 版本开始出现, 但仍以交互为主

---

### 10. Goose (Block/Square)

- **GitHub**: https://github.com/block/goose
- **Stars**: ~15k+
- **许可证**: Apache-2.0
- **状态**: 活跃

**核心架构**:
- Rust 编写, CLI + Electron 桌面界面
- MCP (Model Context Protocol) 原生支持
- goosed 后端服务进程
- 插件架构, 可连接任意 MCP server

**关键差异化**:
- Block 官方开源, 企业级背景
- MCP 原生 -- 标准化工具连接协议
- dev-daemon 配套项目: Block 的后台任务执行守护进程

**与 ccclaw 对比**:
- Goose 的 MCP 集成思路值得参考
- goosed 服务进程 + dev-daemon 的组合类似 ccclaw 的 systemd 模式
- 但 Goose 没有 Issue 驱动, 没有长期知识库

---

### 11. AutoCodeRover (NUS 新加坡国立大学)

- **GitHub**: https://github.com/AutoCodeRoverSG/auto-code-rover
- **Stars**: ~5k+
- **许可证**: GPL-3.0
- **状态**: 学术项目, 更新较慢

**核心架构**:
- GitHub Issue 输入, 输出补丁
- 利用程序结构(类/方法)增强 LLM 理解
- 迭代搜索定位 bug 根因

**关键差异化**:
- SWE-bench verified 46.2%, 每个任务成本 <$0.7
- 项目结构感知(AST 级别)
- 67 个 GitHub issue 平均 12 分钟解决(人类 2.77 天)

**与 ccclaw 对比**:
- AutoCodeRover 只做 Issue -> Patch, 不做编排/记忆
- GPL-3.0 许可证限制商业使用
- 其 AST 感知方法论可参考

---

### 12. Open SWE (LangChain)

- **GitHub**: https://github.com/langchain-ai/open-swe
- **Stars**: 新项目
- **许可证**: 开源
- **状态**: 活跃

**核心架构**:
- LangGraph 构建, 多代理架构(Planner + Reviewer)
- 云端异步执行
- 自动创建 GitHub Issue + PR
- 默认使用 Claude Opus 4.5

**关键差异化**:
- Planner 先研究代码库制定策略, Reviewer 执行后检查
- 与 GitHub 深度集成
- LangChain 生态支持

**与 ccclaw 对比**:
- Open SWE 的 Planner-Reviewer 模式可直接参考
- 都强调异步执行
- Open SWE 依赖 LangChain/LangGraph, ccclaw 直接用 Claude Code

---

### 13. Claude Agent SDK (Anthropic 官方)

- **GitHub**: [TypeScript](https://github.com/anthropics/claude-agent-sdk-typescript) / [Python](https://github.com/anthropics/claude-agent-sdk-python)
- **Stars**: 新发布
- **许可证**: Anthropic 许可
- **状态**: 活跃(2026-03 更新)

**核心架构**:
- 封装 Claude Code CLI 为可编程库
- 提供: agent loop、内置工具、上下文管理
- In-process MCP servers(无需子进程)
- Hooks 系统: 拦截和控制代理行为

**关键差异化**:
- Claude Code 的基础设施, 以库的形式暴露
- Anthropic 官方支持
- 可构建自定义代理系统

**与 ccclaw 对比**:
- **这是 ccclaw 的核心基础层** -- ccclaw 的 Claude Code 执行体可以基于此 SDK 构建
- Hooks 系统可用于 ccclaw 的任务管控
- MCP server 支持可扩展 ccclaw 的工具能力

---

### 14. Taskmaster (claude-task-master)

- **GitHub**: https://github.com/eyaltoledano/claude-task-master
- **Stars**: ~21k
- **许可证**: MIT + Commons Clause
- **状态**: 活跃

**核心架构**:
- 解析 PRD 文档为可执行任务列表
- 三种操作模式: Core(7工具), Standard(15工具), All(36工具)
- MCP 集成, 支持 13+ IDE
- Claude Code 专用: task-orchestrator + task-executor 代理

**关键差异化**:
- 专注于"任务管理层"而非执行层
- 可与多种 AI IDE 配合使用
- 支持 metadata 字段存储外部 ID 等信息

**与 ccclaw 对比**:
- Taskmaster 面向 IDE 内交互, ccclaw 面向后台自主执行
- Taskmaster 的任务分解逻辑可参考
- ccclaw 用 GitHub Issue 作为任务入口, 不需要 PRD 解析

---

### 15. GPT-Engineer

- **GitHub**: https://github.com/AntonOsika/gpt-engineer
- **Stars**: ~55k
- **许可证**: MIT
- **状态**: 低活跃(核心团队转向商业产品 Lovable, CLI 社区维护)

**核心架构**:
- 自然语言 -> 完整代码库生成
- 交互式需求澄清

**与 ccclaw 对比**: 定位不同, GPT-Engineer 是一次性项目生成, 不是持续执行系统

---

### 16. Smol Developer

- **GitHub**: https://github.com/smol-ai/developer
- **Stars**: ~12k
- **许可证**: MIT
- **状态**: 低活跃

**核心架构**: 轻量级代码生成, 生成完整小项目

**与 ccclaw 对比**: 定位不同, 一次性生成工具

---

### 17. Mentat (AbanteAI)

- **GitHub**: https://github.com/AbanteAI/mentat
- **Stars**: ~3k+
- **许可证**: Apache-2.0
- **状态**: 低活跃

**核心架构**: 终端 AI 编码助手, 多文件编辑, git 感知

**与 ccclaw 对比**: 交互式工具, 缺乏自主运行能力

---

### 18. mini-claude-code / learn-claude-code (shareAI-lab)

- **GitHub**: [mini-claude-code](https://github.com/shareAI-lab/mini-claude-code) / [learn-claude-code](https://github.com/shareAI-lab/learn-claude-code)
- **Stars**: 教学项目
- **许可证**: 开源
- **状态**: 活跃(2026-03 更新)

**核心架构**:
- 12 个渐进式课程, 从简单循环到隔离自主执行
- mini-claude-code: 约 1100 行代码, 5 个版本
- 核心理念: "模型调用工具直到完成, 其他都是优化"

**与 ccclaw 对比**: 教学参考价值, 可帮助理解 Claude Code agent loop 内部机制

---

### 19. Sweep

- **GitHub**: https://github.com/sweepai/sweep
- **Stars**: ~8k+
- **许可证**: 开源
- **状态**: 低活跃(Sweep 团队已转型 JetBrains 插件方向)

**核心架构**:
- GitHub Bot, 监听 Issue 自动生成 PR
- 可并行处理多个 ticket

**与 ccclaw 对比**:
- Sweep 是 ccclaw 的前辈概念(Issue -> PR 自动化)
- 但 Sweep 不用 Claude Code, 也缺乏知识库
- 项目已式微

---

### 20. Claude Code 编排工具生态

以下项目围绕 Claude Code 进行多代理编排:

| 项目 | GitHub | 说明 |
|------|--------|------|
| ruflo | [ruvnet/ruflo](https://github.com/ruvnet/ruflo) | Claude 多代理 swarm 编排平台 |
| claude-orchestration | [mbruhler/claude-orchestration](https://github.com/mbruhler/claude-orchestration) | Claude Code 多代理工作流插件 |
| myclaude | [cexll/myclaude](https://github.com/cexll/myclaude) | 多代理编排(Claude/Codex/Gemini) |
| claude-octopus | [nyldn/claude-octopus](https://github.com/nyldn/claude-octopus) | 多触手编排器, Codex/Gemini CLI 并行 |
| claude-code-orchestrator | [mohsen1/claude-code-orchestrator](https://github.com/mohsen1/claude-code-orchestrator) | 基于 Claude Agent SDK 的长周期任务并行 |

这些项目大多是社区实验, 缺乏 ccclaw 的 Issue 驱动 + 知识库 + systemd 整合。

---

## 三、ccclaw 关键竞争定位分析

### ccclaw 的独特组合

ccclaw 的架构组合在当前生态中是**独特的**:

1. **GitHub Issue 作为异步入口** -- 只有 Symphony(Linear)、Copilot Coding Agent、SWE-agent 和 Sweep 有类似的 Issue 驱动能力
2. **Claude Code 作为执行体** -- Claude Agent SDK 刚发布, 直接对接
3. **systemd 作为调度层** -- 所有竞品要么用自建 daemon(Symphony 用 Elixir, OpenClaw 用 Gateway), 要么用云服务(Copilot 用 GitHub Actions, Cursor 用 Cloud VM)。**systemd 是最轻量、最 Linux 原生的方案**
4. **kb 作为长期记忆** -- 大多数竞品缺乏跨会话记忆。OpenClaw 有 Gateway 记忆, Devin 有内部记忆, 但开源方案中极少

### 最需要关注的竞品

按与 ccclaw 的相关性排序:

1. **OpenAI Symphony** -- 架构理念最接近, daemon + issue tracker + 隔离执行, 但用 Elixir/Linear
2. **GitHub Copilot Coding Agent** -- 商业标杆, Issue 驱动 + 异步 + 多模型, 但闭源
3. **Claude Agent SDK** -- ccclaw 的底层基础, 应优先集成
4. **Open SWE (LangChain)** -- Planner-Reviewer 模式可参考, 异步 + GitHub 集成
5. **OpenClaw** -- Skills 生态和记忆系统可借鉴, 但定位不同
6. **Cursor Automations** -- 事件触发 + 后台代理模式可参考

### ccclaw 的差异化优势

| 维度 | ccclaw 优势 |
|------|-----------|
| 部署模式 | systemd 原生, 无需 Elixir/Docker/K8s, 任何 Linux 服务器即可运行 |
| 任务入口 | GitHub Issue -- 全球开发者最熟悉的工作流 |
| 执行体 | Claude Code -- 当前最强 agentic 编码能力 |
| 记忆 | kb 文件系统 -- 简单、可审计、可版本控制 |
| 成本 | 自托管, 仅需 API 费用 |
| 透明度 | 完全开源, 完全可审计 |

### 建议关注的架构模式

1. **Symphony 的 WORKFLOW.md 版本化** -- 将代理行为规范放入仓库, 随分支同步
2. **Copilot 的自我审查** -- 代理提交前先自查安全/质量
3. **Open SWE 的 Planner-Reviewer 分层** -- 分析->计划->执行->审查
4. **OpenClaw 的 Skills 系统** -- 可扩展能力机制
5. **Claude Agent SDK 的 Hooks** -- 在代理行为关键点插入管控逻辑

---

## 四、附录: 关于"Elvis Sun"和"Zoe 编排"

调研中未找到与 OpenClaw/ClawDBot 关联的 "Elvis Sun" 人物。OpenClaw 的创建者是 Peter Steinberger(奥地利开发者)。"Zoe" 确实出现在 OpenClaw 生态中, 指的是一个编排器角色, 负责生成子代理、选择模型、监控进度。如果 sysNOTA 有更具体的来源信息, CCoder 可以进一步追踪。

---

## 五、信息来源

- [OpenClaw GitHub](https://github.com/openclaw/openclaw)
- [OpenClaw Wikipedia](https://en.wikipedia.org/wiki/OpenClaw)
- [OpenClaw Architecture (Medium)](https://bibek-poudel.medium.com/how-openclaw-works-understanding-ai-agents-through-a-real-architecture-5d59cc7a4764)
- [OpenAI Symphony GitHub](https://github.com/openai/symphony)
- [OpenAI Symphony SPEC](https://github.com/openai/symphony/blob/main/SPEC.md)
- [Symphony MarkTechPost](https://www.marktechpost.com/2026/03/05/openai-releases-symphony-an-open-source-agentic-framework-for-orchestrating-autonomous-ai-agents-through-structured-scalable-implementation-runs/)
- [GitHub Copilot Coding Agent Blog](https://github.blog/news-insights/product-news/github-copilot-meet-the-new-coding-agent/)
- [GitHub Copilot Jira Integration](https://github.blog/changelog/2026-03-05-github-copilot-coding-agent-for-jira-is-now-in-public-preview/)
- [SWE-agent GitHub](https://github.com/SWE-agent/SWE-agent)
- [OpenHands GitHub](https://github.com/OpenHands/OpenHands)
- [OpenHands Platform](https://openhands.dev/)
- [Devin 2.0 Blog](https://cognition.ai/blog/devin-2)
- [Devin AI Guide 2026](https://aitoolsdevpro.com/ai-tools/devin-guide/)
- [Cursor Background Agents](https://docs.cursor.com/en/background-agent)
- [Cursor Cloud Agents (NxCode)](https://www.nxcode.io/resources/news/cursor-cloud-agents-virtual-machines-autonomous-coding-guide-2026)
- [Aider GitHub](https://github.com/Aider-AI/aider)
- [Aider Website](https://aider.chat/)
- [Cline GitHub](https://github.com/cline/cline)
- [Cline Website](https://cline.bot/)
- [Goose GitHub](https://github.com/block/goose)
- [dev-daemon GitHub](https://github.com/block/dev-daemon)
- [AutoCodeRover GitHub](https://github.com/AutoCodeRoverSG/auto-code-rover)
- [Open SWE GitHub](https://github.com/langchain-ai/open-swe)
- [Open SWE Blog](https://blog.langchain.com/introducing-open-swe-an-open-source-asynchronous-coding-agent/)
- [Claude Agent SDK TypeScript](https://github.com/anthropics/claude-agent-sdk-typescript)
- [Claude Agent SDK Python](https://github.com/anthropics/claude-agent-sdk-python)
- [Taskmaster GitHub](https://github.com/eyaltoledano/claude-task-master)
- [GPT-Engineer GitHub](https://github.com/AntonOsika/gpt-engineer)
- [Mentat GitHub](https://github.com/AbanteAI/mentat)
- [Smol Developer GitHub](https://github.com/smol-ai/developer)
- [mini-claude-code GitHub](https://github.com/shareAI-lab/mini-claude-code)
- [learn-claude-code GitHub](https://github.com/shareAI-lab/learn-claude-code)
- [Sweep GitHub](https://github.com/sweepai/sweep)
- [Best AI Coding Agents 2026 (Faros)](https://www.faros.ai/blog/best-ai-coding-agents-2026)
- [AI Coding Landscape 2026 (ToolShelf)](https://toolshelf.dev/blog/ai-coding-landscape-2026)
- [Best Background Agents 2026 (Builder.io)](https://www.builder.io/blog/best-ai-background-agents-for-developers-2026)
- [Windsurf vs Cursor (Builder.io)](https://www.builder.io/blog/windsurf-vs-cursor)
