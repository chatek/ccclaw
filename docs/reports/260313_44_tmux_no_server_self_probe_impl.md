# 260313_44 tmux no-server 归一化与发射后即时自检

## 背景

Issue [#44](https://github.com/41490/ccclaw/issues/44) 已确认当前异步链路在“审批 -> 发射 -> RUNNING 回帖”可用，但收口阶段被 `tmux no server running` 阻断：

- `patrol` 与 `ingest` 在查询 `tmux` 状态时直接失败
- repo slot 可能长期停留在 `running`
- 发射后瞬时退出场景缺少最早期诊断证据

本次实现按 Issue 中已拍板方向推进两项改造：

1. 统一把 `no server running` 视为“会话不存在”
2. 发射后同轮即时自检，若会话已消失则立即收口并保留诊断

## 变更摘要

### 1) tmux 错误归一化

文件：`src/internal/tmux/manager.go`

- 新增 `isSessionNotFoundError(err)`，将以下两类错误统一映射为会话不存在：
  - `can't find session`
  - `no server running`
- `Status()`、`CaptureOutput()`、`Kill()` 改为统一使用该判定，返回 `ErrSessionNotFound`（或在 `Kill` 中视作幂等成功）。

效果：`tmux server` 级消失不再把上层轮次直接打崩，可进入既有 `pane_dead`/收口路径。

### 2) 发射后即时自检 + 最早期诊断

文件：`src/internal/app/runtime_cycle.go`

- `dispatchTask()` 在 `tmux` 发射成功并回帖 `RUNNING` 后，不再直接返回；新增调用 `probeDispatchedTMuxSession()`。
- 自检逻辑：
  - 若 `Status()` 返回 `ErrSessionNotFound`，或状态为 `PaneDead`：
    - 立即把 slot 推进到 `pane_dead/load_result`
    - 写入 `EventWarning`
    - 从 `diag/log` 读取尾部诊断并合并到 `slot.LastError`
    - 直接在同轮调用 `finalizeRepoSlot()` 进入收口
- 新增 `formatLaunchProbeFailure()` 统一拼接“即时自检结论 + 诊断摘要”。

效果：避免任务卡在 `RUNNING` 等待下一轮 patrol，且失败时能保留最早期证据。

## 测试

### 新增测试

1. `src/internal/tmux/manager_test.go`
- `TestExecManagerStatusTreatsNoServerAsSessionNotFound`
  - 验证 `list-panes` 返回 `no server running` 时，`Status()` 会返回 `ErrSessionNotFound`。

2. `src/internal/app/runtime_test.go`
- `TestIngestDispatchFinalizesImmediatelyWhenSessionMissingRightAfterLaunch`
  - 构造 `new-session` 成功、随后 `list-panes` 立即 `no server running` 的场景
  - 验证同轮完成：
    - 任务进入 `FAILED`（不再停在 `RUNNING`）
    - 错误信息包含“发射后立即丢失”与诊断内容
    - repo slot 已收口删除
    - 存在即时 `WARNING` 事件

### 回归验证

执行：`cd src && go test ./...`

结果：全量通过。

## 结果与风险

### 已解决

- `tmux server` 消失导致 ingest/patrol 轮次失败的问题已归一化处理
- 发射后瞬时退出可同轮收口并保留诊断，不再形成“只有 RUNNING 回帖”的黑洞

### 仍需后续关注

- 本次未改动 `remain-on-exit` 设置时序；是否与极早退出存在因果仍需独立最小复现实验
- `doctor` 尚未直接输出 slot 卡点与建议动作（Issue #44 的后续建议项）
