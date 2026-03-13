# 260312_37_claude_patrol_done_context_restart_impl

## 背景

- Issue: `#37`
- 目标:
  - `rtk proxy claude` 先退化一步, 默认直连 `claude`
  - 修复 tmux 管理 Claude 进程时的触发执行、过程回报、完成感知
  - 成功终态不自动 close Issue, 改为评论末行追加 `/ccclaw [DONE]`
  - 巡查时发现 Claude 触发压缩或 `usable context > 64%` 时, 立即停止当前 Claude 进程并 fresh restart

## 决策落地

- 执行入口:
  - `ccclaude` 只直连 `claude`
  - 运行态强制停用 `rtk proxy`
- 终态语义:
  - 成功评论末行追加 `/ccclaw [DONE]`
  - 仅当原 `DoneCommentID` 对应评论仍保留该 marker 时, 任务保持 `DONE`
  - 若该评论被人工删改 marker, 任务允许重新进入 `NEW/BLOCKED`
- 巡查语义:
  - 不做刷屏巡查评论
  - 只在状态跃迁时回帖
  - fresh restart 上限 `2` 次
  - 命中压缩时回 `RESTARTED`
- 上下文探测:
  - 参考 `ccstatusline` 的 `Context % (usable)` 思路
  - 分母采用 `context_window_size * 0.8`
  - 分子采用 transcript 最近主链消息的
    - `input_tokens`
    - `cache_creation_input_tokens`
    - `cache_read_input_tokens`
  - 阈值定为 `64%`

## 实现摘要

### 1. Claude 执行链退化

- `src/dist/ops/scripts/ccclaude`
  - 改为只解析并执行 `claude`
- `src/internal/executor/executor.go`
  - `RTKEnabled` 强制为 `false`
  - 运行环境不再注入 `CCCLAW_RTK_BIN`

### 2. Issue 终态与重开

- `src/internal/core/task.go`
  - 新增 `DoneCommentID`
  - 新增 `RestartCount`
- `src/internal/adapters/github/client.go`
  - 新增 `/ccclaw [DONE]` marker 识别
  - 新增 `FindDoneComment`
- `src/internal/adapters/reporter/reporter.go`
  - 成功时追加 `/ccclaw [DONE]`
  - 新增 `ReportStarted`
  - 新增 `ReportRestarted`
- `src/internal/app/runtime.go`
  - `syncIssue()` 读取评论并钉住 `DoneCommentID`
  - 若原 `[DONE]` 评论仍存在且 marker 未被删改, 保持 `DONE`
  - 若 marker 被人工删改, 允许 reopen

### 3. tmux 巡查与 Claude hook 状态面

- `src/internal/tmux/manager.go`
  - 先按 session 前缀过滤再解析
  - 活跃 pane 的空 `pane_dead_status` 不再误判错误
- `src/internal/claude/hooks.go`
  - 新增 hook payload/state 持久化
  - 自动维护目标仓库 `.claude/settings.local.json`
  - 本轮修正:
    - hooks 写入结构改为官方要求的嵌套层级:
      - 事件数组
      - `matcher`
      - `hooks`
      - 内层 `command`
    - 旧路径遗留的 `ccclaw claude-hook` managed entry 也会被替换, 避免重复触发
- `src/cmd/ccclaw/main.go`
  - 新增隐藏子命令:
    - `ccclaw claude-hook session-start`
    - `ccclaw claude-hook pre-compact`
    - `ccclaw claude-hook stop`

### 4. usable context 驱动 fresh restart

- `src/internal/claude/context.go`
  - 从 transcript JSONL 计算 usable context 百分比
- `src/internal/app/runtime.go`
  - 巡查时优先同步 hook 写回的 session/transcript 状态
  - 命中 `PreCompact:auto` 时触发 fresh restart
  - `usable context > 64%` 时触发 fresh restart
  - fresh restart 时:
    - 终止旧 tmux session
    - 清空 `LastSessionID`
    - 清理 hook state
    - 任务回到 `NEW`
  - 超过 `2` 次后转 `DEAD`

## 验证

- 已通过:
  - `cd /opt/src/ccclaw/src && go test ./...`
- 新增覆盖:
  - `src/internal/claude/hooks_test.go`
    - 校验 hooks 官方嵌套结构
    - 校验旧 managed hook 替换且保留自定义 hook
  - `src/internal/claude/context_test.go`
    - 校验 usable context 计算逻辑
  - `src/internal/app/runtime_test.go`
    - 校验 patrol 在 `usable context > 64%` 时会 kill 当前 session 并转回 `NEW`

## 余留风险

- 当前 `64%` 判定主要依赖 transcript 最近主链 usage
- 现场仍建议再做一次真 Claude CLI 集成验证:
  - 确认 `.claude/settings.local.json` 在本机 Claude 版本下可正常触发 hooks
  - 确认 `PreCompact:auto` 与 patrol 配合时的时序符合预期
