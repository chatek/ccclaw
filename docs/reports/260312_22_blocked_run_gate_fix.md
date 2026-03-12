# 260312_22_blocked_run_gate_fix

## 背景

用户要求立即发布当前版本，并确保 [Issue #22](https://github.com/41490/ccclaw/issues/22) 这种普通成员发起、尚未获批的任务，不再被调度链路反复意外触发“尝试执行”。

现场核对结果：

- `Issue #22` 当前状态为 `BLOCKED`
- 本机任务事件中已出现两次阻塞记录
- 当前实现里，`run` 入口使用的 `ListRunnable()` 仍把 `BLOCKED` 任务纳入待执行集合

这意味着即使任务最终不会真正进入执行，`run` 周期仍会持续把已阻塞任务重新捞出来，再做一次 `GetIssue + syncIssue` 检查，行为上属于错误的“重复尝试”。

## 根因

根因位于存储层可执行任务筛选条件：

- 文件：`src/internal/adapters/storage/sqlite.go`
- 旧逻辑：`WHERE state IN ('NEW', 'FAILED', 'BLOCKED')`

`BLOCKED` 的职责应当是“等待外部条件变化后，由 ingest 重新同步并转回 `NEW`”，而不是被 `run` 轮询。

因此问题不是 patrol/tmux 本身，而是 run 阶段的任务选集定义错误。

## 变更

本轮仅做根因修复，不做表面绕过：

1. 将 `ListRunnable()` 的 SQL 条件收口为仅允许 `NEW` 与 `FAILED`
2. 新增回归测试，明确断言：
   - `BLOCKED` 任务不得进入 runnable 集合
   - `NEW/FAILED` 仍可正常进入 runnable 集合

这样收口后：

- 普通成员 Issue 在未获批前会稳定停留在 `BLOCKED`
- 只有后续 `ingest` 发现标签、权限、审批状态发生变化时，才会重新评估并解锁
- `run` 不再对已阻塞任务做周期性“再碰一次”

## 验证

执行命令：

```bash
cd /opt/src/ccclaw/src
go test ./internal/adapters/storage ./internal/app
go test ./...
```

结果：

- 存储层与运行层相关测试通过
- 全量 Go 测试通过

## 结论

Issue #22 对应的根因已经在代码层收口：

- `BLOCKED` 不再属于 `run` 的候选任务
- 后续发布并升级后，本机不会再对该类任务反复发起执行尝试

后续动作：

1. 提交并推送到 `main`
2. 基于新提交创建 GitHub Release
3. 补发 announcements
4. 用本地最新 release 升级当前安装并做现场复核
