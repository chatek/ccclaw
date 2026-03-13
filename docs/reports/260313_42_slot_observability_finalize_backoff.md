# 260313_42_slot_observability_finalize_backoff

## 背景

Issue #42 在上一轮已经完成按仓分片调度与 `FINALIZING` 收尾状态改造，但运行态仍有两个缺口：

- `status --json` 无法直接看出 sidecar slot 当前卡在哪一步
- `FINALIZING` 失败后缺少分阶段退避，短时网络抖动会过快重试，且容易重复回帖

本轮按 Issue 追加拍板，继续补齐这两项。

## 本轮实现

### 1. slot 可观测字段补齐

给 `RepoSlot` 增加以下结构化字段：

- `current_step`
- `last_advance_at`
- `last_attempt_at`
- `next_retry_at`
- `finalize_retry_step`
- `finalize_retry_count`
- `last_reported_at`
- `last_reported_failure`

并在以下关键路径持续刷新：

- `ingest` 发射任务
- tmux 运行中巡查
- pane dead 后收口
- `FINALIZING` 各步骤推进
- patrol 原地重启 Claude

这样可以直接从 slot 看出：

- 当前仓位是在执行、重启、读结果、`sync_target`、`sync_home` 还是 `report_issue`
- 最近一次推进时间
- 最近一次收尾尝试时间
- 下次自动重试时间
- 当前重试计数与对应步骤

### 2. `status` 暴露 slot / FINALIZING 卡点

运行态快照新增 `slots` 段：

- 汇总各 phase 数量
- 输出每个 slot 的 `phase / current_step / finalize_retry / next_retry_at / last_error`

同时补齐任务计数中的 `FINALIZING`，避免之前只看 `NEW/RUNNING/BLOCKED/FAILED/DONE/DEAD` 时漏掉收尾中的任务。

### 3. `FINALIZING` 退避与降噪

新增两类失败策略：

- 临时抖动：如 timeout / connection reset / EOF / 5xx / rate limit
  - 走自动退避重试
  - 基础退避 4 分钟，指数增长，最大 1 小时
  - 在未耗尽重试前，不回 Issue，避免噪音
- 需人工介入：如冲突 / 分支保护 / 权限 / 配置错误
  - 直接进入慢速复查
  - 默认 1 小时后再探测一次
  - 相同失败键只回帖一次，避免重复刷屏

临时抖动重试超过阈值后，会自动转入“需人工介入”语义，并补一次 Issue 提示。

## 结果

- sidecar slot 现在可直接承载“卡点定位 + 重试计划”
- `status --json` 可直接被外部巡检脚本消费，不必再反推日志
- `FINALIZING` 对短时网络波动更稳，对人工介入场景更安静

## 验证

- `cd src && go test ./...`
- `cd src && bash tests/install_regression.sh`

## 涉及文件

- `src/internal/adapters/storage/slot_store.go`
- `src/internal/app/runtime.go`
- `src/internal/app/runtime_cycle.go`
- `src/internal/app/runtime_test.go`
