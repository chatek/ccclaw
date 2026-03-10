# RFC: claude-agent-sdk 调研与轻量集成方案

> 日期: 2026-03-10
> 关联 Issue: #9
> 状态: 待 sysNOTA 审阅

---

## 一、调研背景

当前 ccclaw 的执行层是"黑箱"模式：通过 `exec.Command` 调用 `claude -p` 传入提示词，只能获取退出码和文本输出。这导致：

1. **不可观测** — 无法知道 Claude Code 正在执行什么工具调用
2. **不可恢复** — 超时/崩溃后无法恢复 session，需重新加载全部上下文
3. **不可度量** — 不知道每次执行消耗了多少 token、花费多少美元
4. **不可巡查** — 无法检测僵死进程，只能等 30m timeout 到期

---

## 二、claude-agent-sdk 核心能力摘要

### 2.1 SDK 定位

`claude-agent-sdk`（PyPI 0.1.48，MIT）是 Anthropic 官方 Python SDK，提供对 Claude Code 代理循环的编程访问。核心：将 Claude Code CLI 打包在 wheel 中，通过 subprocess + JSON-lines 协议通信。TypeScript 版本同步存在。

### 2.2 关键抽象

| 抽象 | 说明 |
|------|------|
| `query()` | 异步单次调用，返回 `AsyncIterator[SDKMessage]` |
| `ClaudeSDKClient` | 双向客户端，支持多轮对话、自定义工具、hooks |
| `ClaudeAgentOptions` | 配置项：`system_prompt`, `max_turns`, `max_budget_usd`, `resume`, `hooks` 等 |
| `ResultMessage` | 终结消息：`total_cost_usd`, `usage`, `session_id`, `num_turns`, `duration_ms` |
| `HookMatcher` | 12 种事件钩子：PreToolUse, PostToolUse, Stop, SessionStart 等 |
| `AgentDefinition` | 子代理定义，具有隔离上下文窗口 |

### 2.3 与直接 API 调用的差异

| 维度 | 原始 Claude API | Agent SDK |
|------|----------------|-----------|
| 工具循环 | 自行实现 | SDK 自动运行 |
| 内置工具 | 无 | Read/Edit/Write/Bash/Glob/Grep 等 |
| 上下文管理 | 手动 | 自动压缩（compaction） |
| Session 持久化 | 无 | 磁盘 `.jsonl`，支持 resume/fork |
| 成本追踪 | 自行计算 | `total_cost_usd` 直接返回 |

### 2.4 非 Python 调用可行性

**官方仅支持 Python/TypeScript。** 无 REST/gRPC 接口。

但 SDK 本质上是对 Claude Code CLI 的 subprocess 封装，通信协议是 JSON-lines。这意味着任何语言都可以通过相同协议接入：

- **Go 社区 SDK**: `panbanda/claude-agent-sdk-go` 已实现完整 subprocess + JSON-lines 协议，覆盖全部 12 种 hook 事件
- **Shell 直接调用**: `claude -p --output-format json` 返回包含 `session_id`/`usage`/`total_cost_usd` 的结构化 JSON
- **流式调用**: `claude -p --output-format stream-json` 返回 NDJSON，逐条可见 tool call

---

## 三、Claude CLI JSON 协议详解

### 3.1 单次 JSON 输出 (`--output-format json`)

执行完成后返回单个 JSON 对象：

```json
{
  "type": "result",
  "subtype": "success",
  "session_id": "86eb732d-6d23-47a0-af75-2dd7f01a1891",
  "duration_ms": 45000,
  "duration_api_ms": 42000,
  "is_error": false,
  "num_turns": 5,
  "result": "任务完成文本...",
  "total_cost_usd": 0.0523,
  "usage": {
    "input_tokens": 15000,
    "output_tokens": 8000,
    "cache_creation_input_tokens": 200,
    "cache_read_input_tokens": 100
  }
}
```

错误时 `subtype` 为 `error_max_turns` / `error_during_execution` / `error_max_budget_usd`，但仍返回 `session_id` 可用于恢复。

### 3.2 流式 NDJSON (`--output-format stream-json`)

逐行输出，每行一个 JSON 对象。关键消息类型：

| type | 说明 |
|------|------|
| `system` (subtype `init`) | 首条消息，包含 `session_id`, model, tools 列表 |
| `assistant` | Claude 完整响应，含 `TextBlock` 和 `ToolUseBlock` |
| `user` | 工具执行结果 |
| `result` | 末条消息，含 cost/usage 汇总 |

`AssistantMessage` 中可见每个 tool call 的名称和参数：

```json
{
  "type": "assistant",
  "session_id": "...",
  "message": {
    "content": [
      {"type": "text", "text": "让我读取配置文件..."},
      {"type": "tool_use", "name": "Read", "input": {"file_path": "/opt/ccclaw/config.toml"}}
    ],
    "usage": {"input_tokens": 100, "output_tokens": 50}
  }
}
```

### 3.3 Session 恢复

```bash
# 从 JSON 输出获取 session_id
session_id=$(claude -p "任务" --output-format json | jq -r '.session_id')

# 恢复 session 继续执行
claude -p "继续上次的任务" --resume "$session_id" --output-format json

# 恢复当前目录最近的 session
claude -p "继续" --continue --output-format json
```

Session 文件存储在 `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl`。

---

## 四、方案选型分析

### 4.1 三种集成路径对比

| 维度 | A: 轻量集成 | B: Go SDK 集成 | C: Python SDK 集成 |
|------|------------|---------------|-------------------|
| 改动范围 | 仅 executor + 新增 patrol 脚本 | executor 重写 + 新增 tmux 包 | 新增 Python 执行层 |
| 依赖变化 | 无新依赖 | +`claude-agent-sdk-go` | +Python 3.10+, +`claude-agent-sdk` |
| token 统计 | 从 JSON 输出解析 | 原生支持 | 原生支持 |
| session 恢复 | CLI `--resume` | SDK `Resume` | SDK `resume` |
| 实时监控 | tmux capture-pane + 日志解析 | Go 原生流式读取 | Python 异步流 |
| hooks | 不支持（可用 tmux 模拟部分） | 12 种事件 | 12 种事件 |
| 维护成本 | 最低 | 中等（依赖社区 SDK 质量） | 高（双语言栈） |
| 风险 | 低 | 中（社区 SDK 可能滞后） | 高（架构分裂） |

### 4.2 选定方案：A（轻量集成）+ tmux 后台管理

**核心思路**：保持 Go 主体不变，升级 executor 的输出格式从 `text` 到 `json`，从 JSON 中提取 token 统计和 session_id；用 tmux 替代裸 `exec.Command` 管理长时运行进程，实现可观测、可巡查、可恢复。

---

## 五、轻量集成架构设计

### 5.1 改造后拓扑

```text
 GitHub Issue (control_repo)
       |
   [systemd timer: 5min]
       |
   ccclaw ingest ──► SQLite (tasks + task_events + token_usage)
       |
   [systemd timer: 10min]
       |
   ccclaw run ──► tmux new-session ──► ccclaude -p --output-format json
       |               |                        |
       |          tmux pipe-pane ──► ~/.ccclaw/log/{task}.log
       |               |                        |
   ccclaw patrol ◄─ tmux capture-pane      JSON stdout
       |               |                        |
       |          pane_dead? ──► 解析 JSON ──► session_id + usage
       |               |                        |
   [systemd timer: 2min]                   SQLite token_usage
       |
   ccclaw journal ──► /opt/ccclaw/kb/journal/
```

### 5.2 executor 改造

当前 `executor.go` 第 64-68 行：

```go
args = append(args,
    "-p", prompt,
    "--dangerously-skip-permissions",
    "--output-format", "text",  // ← 改为 "json"
)
```

改造要点：

1. **输出格式**：`text` → `json`
2. **执行载体**：`exec.Command` → `tmux new-session -d -s {taskID}`
3. **输出解析**：从纯文本 → JSON 解析 `Result` 结构体
4. **结果扩展**：`Result` 新增 `SessionID`, `TokenUsage`, `CostUSD` 字段

改造后 `Result` 结构体：

```go
type Result struct {
    Output    string
    ExitCode  int
    Duration  time.Duration
    LogFile   string
    // 新增字段
    SessionID string        // Claude session UUID，用于恢复
    CostUSD   float64       // 本次执行 USD 成本
    Usage     TokenUsage    // token 明细
}

type TokenUsage struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}
```

### 5.3 tmux 会话管理层

新增 `internal/tmux/manager.go`：

```go
// 核心接口
type Manager interface {
    // 创建后台 session 执行命令
    Launch(name, command, workDir string) error
    // 检查 session 是否存在
    Exists(name string) bool
    // 获取 session 状态（运行中/已结束/退出码）
    Status(name string) (*SessionStatus, error)
    // 捕获 pane 输出（最近 N 行）
    CaptureOutput(name string, lines int) (string, error)
    // 终止 session（SIGTERM → 等待 → SIGKILL）
    Kill(name string) error
    // 列出所有 ccclaw session
    List() ([]SessionStatus, error)
}
```

命名规范：`ccclaw-{issueNum}-{shortHash}`（不含 `.` 和 `:`）

执行流程：

```text
1. tmux new-session -d -s "ccclaw-42-a1b2" -c /opt/src/target -x 200 -y 50
2. tmux set-option -t "ccclaw-42-a1b2" remain-on-exit on
3. tmux set-option -t "ccclaw-42-a1b2" history-limit 50000
4. tmux pipe-pane -t "ccclaw-42-a1b2" -o "cat >> ~/.ccclaw/log/ccclaw-42-a1b2.log"
5. tmux send-keys -t "ccclaw-42-a1b2" \
     "ccclaude -p '{prompt}' --dangerously-skip-permissions --output-format json \
      > ~/.ccclaw/var/result-42-a1b2.json 2>> ~/.ccclaw/log/ccclaw-42-a1b2.log; \
      tmux wait-for -S done-42-a1b2" Enter
```

### 5.4 巡查器 (`ccclaw patrol`)

新增 CLI 子命令，由 systemd timer 每 2 分钟触发：

```text
ccclaw patrol 流程：

1. tmux list-sessions -F '#{session_name}\t#{session_created}\t#{pane_dead}\t#{pane_dead_status}'
   ↓ 过滤 ccclaw-* 前缀
2. 对每个 session:
   a. 已结束 (pane_dead=1):
      - 读取 result JSON 文件
      - 解析 session_id, usage, cost
      - 写入 SQLite token_usage 表
      - 更新 task 状态 (DONE/FAILED)
      - 清理 tmux session
   b. 运行中 (pane_dead=0):
      - 计算已运行时长 = now - session_created
      - 若超时 (> executor.timeout):
        · capture-pane 保存最后输出
        · 获取 pane_pid → pgrep -P 找子进程
        · SIGTERM → 等 10s → SIGKILL
        · 记录 DEAD 状态 + 超时原因
      - 若接近超时 (> 80%):
        · 记录 WARNING 到 task_events
   c. 僵死检测:
      - 获取 pane_pid 的子进程
      - 检查子进程 CPU/状态 (ps -p PID -o %cpu,stat)
      - 若 CPU=0 且 stat=S 持续 > 5min → 标记为疑似僵死
```

### 5.5 token 统计与记录

新增 SQLite 表 `token_usage`：

```sql
CREATE TABLE token_usage (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id         TEXT NOT NULL,
    session_id      TEXT,
    input_tokens    INTEGER DEFAULT 0,
    output_tokens   INTEGER DEFAULT 0,
    cache_create    INTEGER DEFAULT 0,
    cache_read      INTEGER DEFAULT 0,
    cost_usd        REAL DEFAULT 0,
    duration_ms     INTEGER DEFAULT 0,
    rtk_enabled     BOOLEAN DEFAULT FALSE,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(task_id)
);
```

统计查询：

```bash
# 按天统计
ccclaw stats --daily
# 按任务统计
ccclaw stats --by-task
# rtk 前后对比
ccclaw stats --rtk-comparison
# 指定日期范围
ccclaw stats --from 2026-03-01 --to 2026-03-10
```

### 5.6 Session 恢复机制

当 executor 因超时/失败需要重试时：

```text
1. 从 SQLite 取出 task 的 last_session_id
2. 若 session_id 非空且 retry_count < max_retry:
   a. 构建恢复 prompt: "上次任务因{reason}中断，请从中断处继续"
   b. 调用 claude -p "{恢复prompt}" --resume {session_id} --output-format json
   c. Claude 自动恢复上下文，避免重复加载
3. 若 session_id 为空或 resume 失败:
   a. 降级为全新执行（当前行为）
```

task 表新增字段：

```sql
ALTER TABLE tasks ADD COLUMN last_session_id TEXT;
```

### 5.7 提示词记录与 journal

**提示词归档**：executor 在发送前将完整 prompt 写入 `~/.ccclaw/var/prompts/{taskID}_{timestamp}.md`，并在 `token_usage` 表中关联 prompt 文件路径。

**journal 自动生成** (`ccclaw journal`)：

```text
每日 23:50 触发（systemd timer），生成：

/opt/ccclaw/kb/journal/2026-03-10.md

内容：
- 今日执行任务数 / 成功 / 失败 / 僵死
- 总 token 消耗 (input/output/cache)
- 总 cost (USD)
- rtk 节省比例（有 rtk 的任务 vs 无 rtk 的任务平均成本）
- 各任务摘要（issue 号、标题、状态、耗时、成本）
- 巡查事件日志（超时、僵死检测、强制终止）
```

---

## 六、改造影响范围

### 6.1 需修改文件

| 文件 | 改动 |
|------|------|
| `src/internal/executor/executor.go` | `--output-format json` + tmux 调用 + Result 扩展 |
| `src/internal/adapters/storage/sqlite.go` | 新增 `token_usage` 表 + `last_session_id` 字段 |
| `src/internal/app/runtime.go` | patrol/journal/stats 逻辑 + session 恢复 |
| `src/cmd/ccclaw/main.go` | 新增 `patrol`, `journal`, `stats` 子命令 |

### 6.2 需新增文件

| 文件 | 说明 |
|------|------|
| `src/internal/tmux/manager.go` | tmux 会话管理封装 |
| `src/ops/systemd/ccclaw-patrol.timer` | 巡查 timer（2min） |
| `src/ops/systemd/ccclaw-patrol.service` | 巡查 service |
| `src/ops/systemd/ccclaw-journal.timer` | journal timer（每日 23:50） |
| `src/ops/systemd/ccclaw-journal.service` | journal service |

### 6.3 新增依赖

**零新 Go 依赖**。tmux 通过 `exec.Command` 调用，JSON 解析使用标准库。

系统依赖新增：`tmux`（install.sh 的 preflight 检查中加入）。

---

## 七、与现有 RFC 的关系

`260309_architecture_evaluation.md` 中提到的四个结构性问题，本方案解决情况：

| 问题 | 本方案覆盖 |
|------|-----------|
| 问题 1: 执行层黑盒 | 部分解决 — JSON 输出可见 tool call 和 token，但不具备实时流式监控 |
| 问题 2: 多 target 路由 | 不涉及（已在 Issue #5 范围） |
| 问题 3: 记忆注入粗糙 | 不涉及（phase1 范围） |
| 问题 4: 缺少 Planner | 不涉及（phase1 范围） |

本方案聚焦于 **可观测性** 和 **可恢复性**，不扩大架构边界。

---

## 八、实施阶段建议

### Phase 1: executor 升级（最小可用）

- `--output-format text` → `json`
- 解析 JSON 提取 `session_id`, `usage`, `cost`
- `token_usage` 表 + `last_session_id` 字段
- `ccclaw stats` 基础统计

### Phase 2: tmux 集成

- `internal/tmux/manager.go`
- executor 从 `exec.Command` 切换到 tmux launch
- `ccclaw patrol` 子命令 + systemd timer
- 超时/僵死检测与处理

### Phase 3: session 恢复 + journal

- `--resume` 重试逻辑
- 提示词归档
- `ccclaw journal` 自动生成
- rtk 前后对比统计

---

## 九、风险与缓解

| 风险 | 缓解 |
|------|------|
| tmux 未安装 | install.sh preflight 检查；降级为当前 exec.Command 模式 |
| JSON 输出格式变化 | Claude CLI 版本锚定；JSON schema 验证 |
| tmux 服务器意外退出 | patrol 检测 tmux server 状态；systemd 可配置 restart |
| session 文件损坏无法 resume | 降级为全新执行；记录降级事件 |
| 日志文件膨胀 | logrotate 配置；pipe-pane 过滤 ANSI 转义 |

---

## 十、结论

**轻量集成方案（A）可以用最小改动实现 Issue #9 的四个核心目标**：

1. **可观测** — `--output-format json` 暴露 tool call 和 token 消耗
2. **可恢复** — `session_id` + `--resume` 避免重复加载上下文
3. **可巡查** — tmux + patrol timer 实现 2 分钟粒度的进程健康检查
4. **可度量** — `token_usage` 表 + `ccclaw stats` 实现全链路 token 统计

零新 Go 依赖，仅新增 tmux 系统依赖，保持架构简洁。若未来需要更深度的 hooks 和实时流式监控，可无损升级到方案 B（Go SDK）。
