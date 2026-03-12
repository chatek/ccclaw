# 260311_30_claude_auth_field_forensics

## 背景

Issue #30 反馈：

- `upgrade` 后仍出现 Claude Code `authentication_error`
- 典型报错为：
  - `OAuth token has expired. Please obtain a new token or refresh your existing token.`
  - `Please run /login`

用户要求先做现场取证，不急于写代码，并明确限定：

- 只关注 `ccclaw` 自己启动的 tmux 托管 Claude 进程
- 忽略其它工具通过 tmux 管理的 Claude 进程
- 先拿到这台机器上的真实原因，再决定后续动作

## 取证边界

本轮只检查以下对象：

1. `ccclaw` 安装目录与运行配置
2. `ccclaw` 状态库、结果文件、日志目录
3. `ccclaw-` 前缀的 tmux 会话
4. `~/.claude/.credentials.json` 当前时间线
5. 本机 `~/.claude/projects/-opt-src-ccclaw/*.jsonl` 中的真实 Claude 认证错误记录

明确不把以下对象计入 `ccclaw` 根因：

- `agent-view`
- 其它 tmux socket / session
- 其它工具自行拉起的 Claude 进程

## 现场事实

### 1. 当前 tmux 现场没有任何 `ccclaw-` 会话

现场 `tmux list-sessions` 仅看到：

- `agentorch_check-claude-mmk9w7i2`
- `agentorch_fixed-codex-mmibgv4w`

未见任何 `ccclaw-...` 会话。

这说明当前存活的 tmux Claude 并发，不属于 `ccclaw` 自己的运行现场。

### 2. `ccclaw` 状态库中的失败原因不是 401

现场库：`/home/zoomq/.ccclaw/var/state.db`

`tasks` / `task_events` 直接显示：

- `41490/ccclaw#28#body` 最终状态为 `DEAD`
- 对应错误为：
  - `tmux 会话 ccclaw-41490_ccclaw_28_body-448a 已丢失，且未找到结果文件`
- 旧 task id `28#body` 也同样是：
  - `tmux 会话 ccclaw-28_body-0a65 已丢失，且未找到结果文件`

库内没有任何以下特征进入 `ccclaw` 任务事件链：

- `401`
- `authentication_error`
- `OAuth token has expired`
- `Please run /login`

### 3. `ccclaw` 结果文件为空，说明任务未拿到结构化结果即已丢失

现场结果文件：

- `/home/zoomq/.ccclaw/var/results/28_body.json`
- `/home/zoomq/.ccclaw/var/results/41490_ccclaw_28_body.json`

两者大小均为 `0`。

说明对应 tmux 托管任务在结果回写前已经丢失，和“拿到了 Claude 401 JSON 再被解析失败”不是同一条链。

### 4. `run` / `patrol` 定时任务存在同秒重叠，并已多次打到 SQLite 锁

现场 systemd timer 配置：

- `ccclaw-run.timer`: `OnCalendar=*:0/10`
- `ccclaw-patrol.timer`: `OnCalendar=*:0/2`

因此每逢整 10 分钟，二者会同秒触发。

`journalctl --user` 现场记录已多次出现：

- `2026-03-11 15:40:15 -0400`
  - `ccclaw-patrol.service` 报 `回填 issue_repo 失败: database is locked (5) (SQLITE_BUSY)`
- `2026-03-11 16:00:15 -0400`
  - `ccclaw-patrol.service` 同类 `SQLITE_BUSY`
- `2026-03-11 16:10:15 -0400`
  - `ccclaw-patrol.service` 同类 `SQLITE_BUSY`
- `2026-03-11 16:40:15 -0400`
  - `ccclaw-patrol.service` 同类 `SQLITE_BUSY`
- `2026-03-11 17:10:15 -0400`
  - `ccclaw-patrol.service` 同类 `SQLITE_BUSY`
- `2026-03-11 18:50:15 -0400`
  - `ccclaw-patrol.service` 同类 `SQLITE_BUSY`
- `2026-03-11 19:00:15 -0400`
  - `ccclaw-run.service` 报 `初始化 SQLite 表失败: database is locked (5) (SQLITE_BUSY)`

这条证据链已经足够说明：

- `ccclaw` 本机当前存在真实的调度并发碰锁
- 其直接后果之一，是任务/巡查状态无法稳定收口

### 5. 本机确实发生过真实 Claude OAuth 401

虽然 `ccclaw` 自己的任务链里没有 401，但在本机 Claude 会话历史里能直接找到：

- `/home/zoomq/.claude/projects/-opt-src-ccclaw/471ae1e7-8eac-49a1-b6a1-c7e12727f0c5.jsonl`
  - `2026-03-09T08:37:43Z`
  - `OAuth token has expired ... Please run /login`
- `/home/zoomq/.claude/projects/-opt-src-ccclaw/05409543-c9db-49c6-819b-a18950508477.jsonl`
  - `2026-03-10T22:42:10Z`
  - `OAuth token has expired ... Please run /login`
- `/home/zoomq/.claude/projects/-opt-src-ccclaw/68df3ac1-a03b-449a-a505-6527997f8e96.jsonl`
  - `2026-03-11T23:00:04Z`
  - `OAuth token has expired ... Please run /login`
- 另有一条：
  - `2026-03-11T08:13:28Z`
  - `Invalid authentication credentials ... Please run /login`

因此，本机上的 Claude 会话层面，认证失效现象是真实存在的。

但这些证据不在 `ccclaw` 自身的状态库/结果文件/任务事件链里。

### 6. 当前凭据已经恢复可用

现场凭据：`/home/zoomq/.claude/.credentials.json`

取证时可见：

- 最近改写时间：`2026-03-11 19:00:44 -0400`
- 当前 `expiresAt`: `2026-03-12 03:00:44 -0400`
- `subscriptionType = max`
- `rateLimitTier = default_claude_max_5x`

同时现场执行 `ccclaw doctor`，结果为：

- `Claude 运行态: installed_setup_token_ready`

说明本机当时已恢复到可用登录态，不是一个持续失效中的现场。

## 归因判断

基于以上证据，最稳妥的归因是：

### A. 成立的判断

1. 这台机器上，Claude 自身确实发生过真实 OAuth 401
2. `upgrade` 不是这次 401 的直接根因
3. `ccclaw` 当前默认认证链路没有能力自动无感续登
4. `ccclaw` 本机最近一次已坐实的直接失败根因是：
   - `run` / `patrol` 并发触发
   - SQLite 锁冲突
   - tmux 会话丢失
   - 结果文件为空

### B. 不成立的判断

以下结论当前都没有证据支持：

1. `ccclaw upgrade` 覆盖坏了 `~/.claude/.credentials.json`
2. `ccclaw` 自己最近那几次 `DEAD` 任务是由 `OAuth token expired` 直接触发
3. 其它工具通过 tmux 管理的 Claude 并发，应归到 `ccclaw` 身上

## 根因拆分

Issue #30 表面上只有一个报错，但现场实际拆成了两条不同问题链：

### 问题链 1：Claude 上游认证失效

- 主体：Claude 自身会话
- 现象：`OAuth token has expired` / `Please run /login`
- 归属：上游 Claude Code 认证续期问题
- `ccclaw` 责任边界：只能预警与指引，不能承诺自动恢复

### 问题链 2：`ccclaw` 本地调度并发碰锁

- 主体：`ccclaw` 自己的 `run` / `patrol`
- 现象：`SQLITE_BUSY`、会话丢失、结果文件为空、任务误入 `DEAD`
- 归属：`ccclaw` 本地实现问题
- 优先级：应单独立 Issue 处理

## 建议动作

### 对 Issue #30

建议按“调研结论已完成”关闭，关闭语义为：

- 已确认本机确有 OAuth 401
- 但已排除它是 `ccclaw` 最近任务失败的直接根因
- 当前 Issue 的机制级结论已经足够闭环

### 对 `ccclaw` 后续实现

建议另开新 Issue，专门处理本机已坐实的问题：

- `bug: run/patrol 定时重叠导致 SQLite 锁冲突与 tmux 会话丢失`

建议该新 Issue 聚焦以下方向：

1. 严格避免 `run` 与 `patrol` 的共享库并发写入
2. 保证单线性调度，不让巡查与执行相互踩踏
3. 当 tmux 会话丢失时，优先拿现场尾日志，不要直接粗暴判 `DEAD`
4. 将认证失效与本地执行失败分开建模，避免误判

## 最终结论

Issue #30 对应的“真实原因”不是单一答案，而是两层：

1. 用户看到的 `OAuth token expired` 是真实上游问题
2. 但 `ccclaw` 自己在本机已坐实的直接故障，实际是调度并发碰锁，不是 401

因此，若继续把 #30 当成“修复 `ccclaw` 自己导致的认证失败”来做，会跑偏。
