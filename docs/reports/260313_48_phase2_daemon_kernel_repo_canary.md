# 260313_48_phase2_daemon_kernel_repo_canary

## 背景

- 执行来源：Issue #48 `phase2: daemon 执行内核与 repo 级灰度切流`
- 前置阶段：
  - #46 已冻结 stream-json 事件契约
  - #47 已在 tmux 载体下切换 stream-json 并完成影子对账

本轮目标是引入 `executor.mode=tmux|daemon`，并支持 repo 级灰度覆盖。

## 本轮实现

### 1. 配置模型：全局 + repo 覆盖

- `src/internal/config/config.go`
  - 新增执行模式枚举：`tmux|daemon`
  - 新增全局配置：`[executor].mode`
  - 新增 target 覆盖：`[[targets]].executor_mode`
  - 新增解析/校验与解析接口：`ExecutorMode()`、`ExecutorModeForRepo(repo)`

- 样例配置同步更新：
  - `src/ops/config/config.example.toml`
  - `src/dist/ops/config/config.example.toml`
  - `src/dist/install.sh` 内置模板

### 2. 运行时按仓装配执行器

- `src/internal/app/runtime.go`
  - 新增 `newExecutorForMode(mode)` 与 `newExecutorForRepo(repo)`
  - `Patrol` 拆分为：
    - tmux 仓巡查（原能力）
    - daemon 仓健康检查/补偿（仅补偿 finalize retry）

- `src/internal/app/runtime_cycle.go`
  - ingest/dispatch/advance 改为按仓模式选择执行器
  - `RepoSlot` 增加 `executor_mode` 记录
  - daemon 仓不再依赖 tmux pane 状态推进，running/restarting 会直接进入收口路径

- `src/internal/adapters/storage/slot_store.go`
  - `RepoSlot` 新增 `executor_mode` 字段

### 3. daemon 执行内核：CommandContext + stream-json 逐行解码

- `src/internal/executor/executor.go`
  - 新增 `runSyncStreamJSON`：
    - 仍使用 `CommandContext`
    - 通过逐行扫描/解码直接消费 stream-json（daemon 路径）
    - 同步产出：
      - `*.stream.jsonl`
      - `*.event.json`
      - 兼容 `*.json` result
  - `runMetadata` 新增 `output_format`
  - 保留回滚开关：`CCCLAW_OUTPUT_FORMAT=json`

- `src/internal/executor/stream_artifact.go`
  - stream snapshot -> 兼容 result 的转换增强（保留错误 subtype）

- `src/internal/executor/stream_contract.go`
  - 错误 subtype 解析增强（用于兼容旧错误语义）

## 测试与灰度记录

### 新增/更新测试

- `src/internal/config/config_test.go`
  - 新增执行模式覆盖与非法模式校验
- `src/internal/app/runtime_test.go`
  - 新增按仓 mode 装配验证
  - 新增 daemon 仓 ingest 同轮 DONE（不依赖 tmux patrol）验证
- 既有 `executor` / `app` / `config` / 全量包回归通过

### 本地灰度记录

- 以 `target.executor_mode=daemon` 对单仓样本执行：
  - 任务可同轮 `RUNNING -> FINALIZING -> DONE`
  - 未引入 tmux 依赖
  - `FINALIZING` 失败重试与告警机制保持有效

## 回滚策略

- 可单仓回滚：将 `[[targets]].executor_mode` 改回 `tmux`
- 可全局回滚：将 `[executor].mode` 改回 `tmux`
- 不影响其他仓位

## 后续优化建议

1. 增加 daemon 仓对账统计持久化（按天输出 match/mismatch 与比率）。
2. 为 daemon 健康检查补充“长时间无事件进展”阈值告警。
3. 在 `status --json` 增加 repo-mode 分组与灰度态摘要。
