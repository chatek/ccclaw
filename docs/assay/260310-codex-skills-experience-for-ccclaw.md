# Codex Skills/Agent SDK 开发经验对 ccclaw 的借鉴价值调研

> Issue: #14
> 调研日期: 2026-03-10
> 调研模型: Claude Opus 4.6 (claude-opus-4-6)
> 状态: 待审核

---

## 1. 调研背景

OpenAI 于 2026 年初发布博文《Using Skills to Accelerate OSS Maintenance》，
披露其在 Agents SDK 的 Python 与 TypeScript 两个仓库中，利用 Codex Skills 系统
自动化日常维护工作流的实践经验。

**核心数据**：2025.12 — 2026.02 三个月间，两仓库合并 PR 从前一季度的 316 个
增长到 457 个（Python: 182→226, TypeScript: 134→231），归因于 Skills 驱动的
工作流自动化。

ccclaw 同样是一个以 AI 执行体为核心的长期任务系统，两者在架构理念上存在
可比性，值得深入分析 Codex 的实践经验中哪些可合理借鉴。

---

## 2. Codex 核心架构概述

### 2.1 Agent Loop（代理循环）

Codex CLI 的核心是一个请求-响应循环：

1. 用户输入 → 结构化 prompt
2. 向 Responses API 发送 HTTP 请求
3. 通过 SSE 流式接收模型响应
4. 模型返回最终回复或工具调用请求
5. 工具调用本地执行 → 结果追加到 prompt → 重新提交
6. 循环直到模型返回最终消息

**prompt 构建优先级**：system > developer (沙箱权限 + AGENTS.md) > user > assistant

### 2.2 Skills 系统架构

每个 Skill 是一个标准化目录：

```
my-skill/
├── SKILL.md          # 必需：元数据 + 指令
├── scripts/          # 可选：确定性 shell 操作
├── references/       # 可选：上下文文档
├── assets/           # 可选：模板等支持材料
└── agents/
    └── openai.yaml   # 可选：UI/策略/依赖配置
```

**SKILL.md 格式**：

```yaml
---
name: skill-name
description: 清晰描述何时触发、何时不触发
---

具体指令步骤...
```

**Skill 存储层级**：

| 范围 | 路径 | 用途 |
|------|------|------|
| 仓库目录 | `$CWD/.agents/skills` | 特定目录工作流 |
| 仓库根 | `$REPO_ROOT/.agents/skills` | 全仓库通用 |
| 用户级 | `$HOME/.agents/skills` | 跨项目个人技巧 |
| 系统级 | `/etc/codex/skills` | 系统管理员默认 |
| 内置级 | 捆绑 | OpenAI 出厂自带 |

**触发机制**：

- **显式调用**：用户通过 `/skills` 命令或 `$skill-name` 语法直接引用
- **隐式匹配**：Codex 根据 description 字段自主判断是否匹配当前任务

### 2.3 AGENTS.md 分层指令系统

三级发现与覆盖机制：

1. **全局级** `~/.codex/AGENTS.md` — 跨项目默认
2. **项目级** 从 git root 到当前目录逐层扫描
3. **就近覆盖** `AGENTS.override.md` 临时替代基础指令

合并策略：从根向下拼接，靠近当前目录的内容覆盖远端。
总大小上限默认 32 KiB（`project_doc_max_bytes`）。

### 2.4 渐进式上下文加载

三层递进，按需展开：

1. **第一层**：启动时仅加载所有 Skill 的 name + description（元数据）
2. **第二层**：任务匹配时加载完整 SKILL.md 指令体
3. **第三层**：执行时按需加载 scripts/、references/ 中的具体文件

**设计意图**：避免一次性加载所有知识导致上下文膨胀，保留 token 给真正需要的环节。

### 2.5 沙箱安全边界

平台原生实现：

- **Linux**：bubblewrap（实验性，默认关闭），user namespace 隔离
- **macOS**：Seatbelt 策略文件

三种沙箱模式：

| 模式 | 行为 |
|------|------|
| `read-only` | 只读，不允许编辑或执行命令 |
| `workspace-write` | 默认模式，workspace 内可编辑与运行命令 |
| `danger-full-access` | 移除所有文件系统与网络边界 |

搭配审批策略（`untrusted` / `on-request` / `never`）形成权限矩阵。

### 2.6 多 Agent 编排（实验性）

内置角色类型：

- **default**：通用回退
- **worker**：执行导向
- **explorer**：代码库探索（读取密集）
- **monitor**：长期运行监控（优化等待/轮询）

并行生成子 Agent → 收集结果 → 合并响应。
支持 CSV 批处理模式：每行数据一个 worker。

### 2.7 Prompt 缓存优化

**核心教训**：MCP 工具枚举顺序不一致导致缓存全部失效。

系统指令、工具定义、沙箱配置、环境上下文在请求间保持**完全一致的顺序**，
以维持长稳定前缀的缓存命中。任何变动（切换模型、修改沙箱权限、
MCP 服务器动态更新工具列表）都会使缓存失效。

### 2.8 验证门控与审查流程

**强制验证栈**（以 Agents SDK 为例）：

- Python: `make format` → `make lint` → `make typecheck` → `make tests`
- TypeScript: `pnpm i` → `pnpm build` → `pnpm -r build-check` → `pnpm lint` → `pnpm test`

**模型 vs 脚本职责分工**：

- **脚本负责**：确定性重复操作（格式化、lint、类型检查、测试执行）
- **模型负责**：解释、比较、报告、上下文相关判断

**发布审查流程**：获取上一版本 tag → diff 对比 → 检查兼容性/回归/遗漏 → 输出就绪判定。

---

## 3. ccclaw 现有架构对照

| 维度 | ccclaw 现状 | Codex 对应能力 |
|------|------------|---------------|
| **知识体系** | `kb/skills/` L1/L2 分层，关键词匹配 | `.agents/skills/` SKILL.md + 渐进加载 |
| **项目指令** | `CLAUDE.md` + `AGENTS.md` 多级 | `AGENTS.md` 三级覆盖 + override |
| **任务驱动** | GitHub Issue → systemd 调度 → Claude Code | 用户交互 → Agent Loop → 工具调用 |
| **执行沙箱** | tmux 会话隔离，无文件系统沙箱 | bubblewrap/Seatbelt 操作系统级隔离 |
| **多 Agent** | 单一 Claude Code 执行体 | worker/explorer/monitor 角色并行 |
| **成本优化** | RTK 前缀 + token 统计 | Prompt 缓存 + 渐进上下文加载 |
| **验证流程** | 手工或 Issue 驱动 | 强制验证栈 + 自动门控 |

---

## 4. 可借鉴经验分析

### 4.1 【高优先级】渐进式上下文加载

**Codex 做法**：Skill 元数据先行，完整指令按需加载，支持文件仅在使用时读取。

**ccclaw 现状**：`kb/` 通过关键词匹配整体加载文档内容推送给 Claude Code。

**借鉴建议**：

1. 为 `kb/skills/` 每个技巧增加 `summary.yaml` 元数据文件（名称 + 触发条件 + 关键词）
2. 执行时先加载所有 summary 索引（token 开销小），再按匹配度加载完整内容
3. 对 `references/` 类长文档采用按需加载而非全量推送

**预期收益**：降低每次执行的 token 消耗 30-50%，尤其在 kb 内容持续增长后。

### 4.2 【高优先级】Skill 标准化目录结构

**Codex 做法**：`SKILL.md` + `scripts/` + `references/` + `assets/` 标准结构。

**ccclaw 现状**：`kb/skills/L1/` 和 `L2/` 下为纯 Markdown 文件，缺乏标准结构。

**借鉴建议**：

1. 保留 L1/L2 分层（这是 ccclaw 独有的抽象层级，Codex 没有）
2. 将每个 Skill 从单文件升级为目录：

```
kb/skills/L1/git-conflict-resolve/
├── CLAUDE.md       # 元数据 + 指令（对应 SKILL.md）
├── scripts/        # 确定性脚本
└── references/     # 参考文档
```

3. `CLAUDE.md` 头部增加 YAML frontmatter：

```yaml
---
name: git-conflict-resolve
trigger: 当遇到 git 合并冲突时
keywords: [git, merge, conflict, resolve]
level: L1
---
```

**预期收益**：知识库结构化程度提升，支持更精准的匹配和渐进加载。

### 4.3 【高优先级】强制验证门控

**Codex 做法**：代码变更后强制执行格式化、lint、类型检查、测试的完整验证栈。

**ccclaw 现状**：依赖 Claude Code 自行判断是否验证，缺乏强制机制。

**借鉴建议**：

1. 在 `AGENTS.md` 中定义 ccclaw 项目的强制验证栈：

```markdown
# 强制验证（代码变更后必须执行）
- make fmt
- make test
- make build
```

2. 在任务执行结果回写时，检查验证栈是否通过
3. 验证失败的任务不标记为 DONE，而是标记为 FAILED 并记录原因

**预期收益**：减少未验证代码合入主线的风险，提升自动化产出质量。

### 4.4 【中优先级】模型 vs 脚本职责分工

**Codex 做法**：确定性操作交给 scripts/，判断性操作留给模型。

**ccclaw 现状**：所有操作都通过 Claude Code 执行，未区分确定性与判断性。

**借鉴建议**：

1. 在 `kb/skills/` 中，将可脚本化的步骤提取到 `scripts/` 目录
2. 执行器（Executor）先运行确定性脚本收集证据，再将证据交给 Claude Code 做判断
3. 例如：`healthcheck.sh` 收集系统状态 → Claude Code 分析是否正常

**预期收益**：减少模型调用次数和 token 消耗，提升确定性操作的可靠性。

### 4.5 【中优先级】Prompt 缓存一致性

**Codex 教训**：MCP 工具枚举顺序不一致导致 prompt 缓存完全失效。

**ccclaw 现状**：通过 RTK 前缀做缓存优化，但未严格控制 prompt 构成顺序。

**借鉴建议**：

1. 确保 ccclaude 包装脚本构建 prompt 时，系统指令、kb 上下文、任务描述
   的拼接顺序**严格固定**
2. kb 文档排序采用确定性算法（如按路径字典序），避免不同执行间顺序漂移
3. 监控 `cache_read_input_tokens` 占比作为缓存命中率指标

**预期收益**：提升 Anthropic API 的 prompt 缓存命中率，降低重复 token 消耗。

### 4.6 【中优先级】Skill 触发路由优化

**Codex 做法**：description 字段作为路由智能，清晰描述何时触发、何时不触发。

**ccclaw 现状**：`kb/` 匹配依赖关键词数量评分，缺乏明确的触发条件语义。

**借鉴建议**：

1. 每个 Skill 的元数据中增加 `trigger` 字段，用自然语言描述触发条件
2. 匹配算法升级：关键词评分 + trigger 语义匹配的复合评分
3. 增加 `negative_trigger`（排除条件）减少误匹配

**预期收益**：提升知识库推荐的精准度，减少不相关文档占用上下文。

### 4.7 【低优先级】多 Agent 角色分工

**Codex 做法**：worker/explorer/monitor 角色并行执行。

**ccclaw 现状**：单一 Claude Code 执行体，通过 tmux 会话实现异步。

**借鉴建议**：

1. **短期**：利用 Claude Code 现有的 subagent 能力，在执行时按需分派子任务
2. **中期**：定义 ccclaw 特有的角色类型：
   - `researcher`：调研型任务，读取密集
   - `implementer`：实现型任务，代码修改
   - `reviewer`：审查型任务，比较与验证
3. **长期**：支持在 config.toml 的 `[[targets]]` 中配置角色偏好

**预期收益**：复杂任务可并行处理，提升吞吐量。当前单线程执行是 ccclaw 的瓶颈之一。

### 4.8 【低优先级】操作系统级沙箱

**Codex 做法**：bubblewrap (Linux) / Seatbelt (macOS) 实现文件系统和网络隔离。

**ccclaw 现状**：依赖 tmux 会话级隔离和 Linux 用户权限，无操作系统级沙箱。

**借鉴建议**：

1. **短期**：记录但不实施，ccclaw 当前场景（个人服务器、受信任仓库）风险可控
2. **中期**：如果扩展到多用户或不可信 Issue 来源，考虑引入 bubblewrap
3. 可参考 Codex 的 `workspace-write` 模式思路，限制 Claude Code 的写入范围

**预期收益**：安全性提升，但当前优先级较低。

---

## 5. 不建议借鉴的部分

### 5.1 AGENTS.md 标准替换 CLAUDE.md

Codex 使用 `AGENTS.md` 作为项目指令文件，这是一个多工具通用的开放标准
（Cursor、Aider 等也支持）。但 ccclaw 深度绑定 Claude Code 生态，
且 `CLAUDE.md` 已经提供了更丰富的配置能力（hooks、子 Agent 策略、
MCP 集成等）。ccclaw 已同时维护 `CLAUDE.md` 和 `AGENTS.md`，
无需改变现有策略。

### 5.2 CLI 交互式 Agent Loop

Codex CLI 的交互式循环设计面向开发者即时使用场景。
ccclaw 的 Issue 驱动 + systemd 调度的异步模式更适合长期无人值守任务，
无需模仿交互式循环。

### 5.3 GitHub Action 集成

Codex 提供 GitHub Action 用于 CI 自动化。ccclaw 采用 systemd 本地调度 +
gh CLI 轮询方案，避免了对 GitHub Actions 的依赖和额外成本，
更适合个人/小团队的部署模型。

---

## 6. 实施优先级建议

| 优先级 | 建议项 | 预计工作量 | 预期收益 |
|--------|--------|-----------|---------|
| P0 | 渐进式上下文加载 | 中 | token 消耗降低 30-50% |
| P0 | 强制验证门控 | 小 | 产出质量保障 |
| P1 | Skill 标准化目录结构 | 中 | 知识库可维护性提升 |
| P1 | 模型 vs 脚本职责分工 | 中 | token 节省 + 可靠性提升 |
| P1 | Prompt 缓存一致性 | 小 | 缓存命中率提升 |
| P2 | Skill 触发路由优化 | 中 | 匹配精准度提升 |
| P2 | 多 Agent 角色分工 | 大 | 并行吞吐量提升 |
| P3 | 操作系统级沙箱 | 大 | 安全性提升（当前非刚需） |

---

## 7. 总结

Codex Skills 系统的核心价值在于三点：

1. **渐进式上下文管理** — 元数据/指令/脚本三层按需加载，避免上下文膨胀
2. **确定性与判断性分离** — 脚本处理重复操作，模型聚焦高价值判断
3. **强制验证门控** — 不信任默认行为，用流程保障质量

这三点与 ccclaw 的设计哲学（"把高频低价值操作从高价模型中移出"）高度一致，
建议优先落地。

多 Agent 编排和操作系统级沙箱属于 Codex 的差异化优势，但在 ccclaw 当前阶段
（个人/小团队场景、可信 Issue 来源）优先级较低，可作为中长期演进方向。

---

## refer.

### 一手资料（OpenAI 官方）

- [Using Skills to Accelerate OSS Maintenance](https://developers.openai.com/blog/skills-agents-sdk) — 本次调研主文，披露 Agents SDK 仓库中 Skills 驱动维护的实践数据
- [Agent Skills 文档](https://developers.openai.com/codex/skills/) — Codex Skills 系统完整文档，含 SKILL.md 格式、存储层级、触发机制
- [Custom instructions with AGENTS.md](https://developers.openai.com/codex/guides/agents-md/) — AGENTS.md 三级覆盖机制详解
- [Sandboxing 文档](https://developers.openai.com/codex/concepts/sandboxing/) — Codex 沙箱模式与审批策略完整说明
- [Multi-agents 文档](https://developers.openai.com/codex/multi-agent/) — 多 Agent 编排（实验性）角色类型与管理
- [Unrolling the Codex agent loop](https://openai.com/index/unrolling-the-codex-agent-loop/) — Agent Loop 架构详解与 prompt 缓存教训
- [Codex SDK 文档](https://developers.openai.com/codex/sdk/) — Codex 编程接口
- [Use Codex with the Agents SDK](https://developers.openai.com/codex/guides/agents-sdk/) — Codex CLI 与 Agents SDK 协同使用指南
- [GitHub openai/skills](https://github.com/openai/skills) — Codex Skills 目录仓库，含 curated/experimental/system 分类

### 社区分析与比较

- [Codex vs Claude Code (Builder.io)](https://www.builder.io/blog/codex-vs-claude-code) — 两大 AI 编码 Agent 的功能与配置对比
- [Skills in OpenAI Codex (Jesse Vincent)](https://blog.fsck.com/2025/12/19/codex-skills/) — 第三方开发者对 Codex Skills 系统的深度分析
- [Porting Skills (and Superpowers) to OpenAI Codex](https://blog.fsck.com/2025/10/27/skills-for-openai-codex/) — Claude Code Superpowers 迁移到 Codex 的实践
- [OpenAI Codex Agent Loop Architecture (InfoQ)](https://www.infoq.com/news/2026/02/codex-agent-loop/) — Agent Loop 内部机制技术报道
- [MCP tools cache rate issue (#2610)](https://github.com/openai/codex/issues/2610) — MCP 工具枚举导致缓存命中率下降的实际 Issue

### 搜索验证

所有关键事实均通过 DuckDuckGo 网络搜索交叉验证，包括：
- PR 合并速率数据（316 → 457）
- bubblewrap/Seatbelt 沙箱实现细节
- SKILL.md 格式与目录结构规范
- prompt 缓存 MCP 工具排序 bug
- 多 Agent 角色类型（worker/explorer/monitor）
