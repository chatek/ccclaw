# 260310_10_phase1_json_stats_impl

## 背景

根据 Issue #10 `feat: 轻量集成 Claude CLI JSON 协议 + tmux 巡查器`，本轮先落地 Phase 1 最小可用闭环：

- executor 输出从 `text` 升级为 `json`
- 解析 Claude JSON 结果中的 `session_id`、`usage`、`total_cost_usd`
- SQLite 增加 `token_usage` 表与 `tasks.last_session_id`
- 新增 `ccclaw stats` 基础统计命令

本轮不进入 tmux / patrol / journal / resume，保持范围与 Issue 已拍板阶段一致。

## 实现内容

### 1. executor 改为 JSON 结果驱动

修改 `src/internal/executor/executor.go`：

- 调用参数从 `--output-format text` 改为 `--output-format json`
- stdout/stderr 分离采集，避免 stderr 污染 JSON 解析
- 新增结构化解析：
  - `session_id`
  - `usage.input_tokens`
  - `usage.output_tokens`
  - `usage.cache_creation_input_tokens`
  - `usage.cache_read_input_tokens`
  - `total_cost_usd`
- 即使 Claude 返回结构化错误结果，也尽量保留已解析出的 `session_id` 与 token/cost 指标，供后续 Phase 3 恢复能力复用

### 2. task / SQLite 持久化补齐会话与 token 维度

修改 `src/internal/core/task.go` 与 `src/internal/adapters/storage/sqlite.go`：

- `core.Task` 新增 `LastSessionID`
- 新增 `core.TokenUsage`
- `tasks` 表新增 `last_session_id`
- 启动时自动检查并补齐旧库缺失列，避免已有状态库因 schema 演进失效
- 新增 `token_usage` 表，记录：
  - `task_id`
  - `session_id`
  - input/output/cache token
  - `cost_usd`
  - `duration_ms`
  - `rtk_enabled` 预留字段

### 3. runtime 在成功与失败路径都记录执行指标

修改 `src/internal/app/runtime.go`：

- executor 返回结果后，优先把 `session_id` 回写到 task
- 只要拿到结构化结果，就写入 `token_usage`
- 因此即便任务失败，只要 Claude 返回了 JSON 结果，也不会丢失本次 session/token/cost 记录

这一步为后续 Phase 3 的 `--resume` 保留了基础数据。

### 4. 新增 `ccclaw stats`

修改 `src/cmd/ccclaw/main.go` 与 `src/internal/app/runtime.go`：

- 新增 `ccclaw stats`
- 当前输出聚焦基础历史聚合，避免越过 #12 尚未拍板的 `status/stats` 语义边界
- 输出包含：
  - 执行次数
  - 会话数
  - input/output/cache token 汇总
  - 累计成本
  - 首次/最近记录时间
  - 最近任务维度的 token 汇总表

## 测试与验证

新增测试：

- `src/internal/executor/executor_test.go`
  - 验证成功 JSON 解析
  - 验证结构化错误 JSON 仍保留 `session_id/cost`
- `src/internal/adapters/storage/sqlite_test.go`
  - 验证 `last_session_id` 持久化
  - 验证 `token_usage` 汇总与任务维度统计
- `src/internal/app/runtime_test.go`
  - 验证 `ccclaw stats` 输出摘要与任务表

执行验证：

```bash
/usr/local/go/bin/go test ./...
```

注意：

- 当前系统默认 `go` 仍是 `/usr/bin/go` -> `go1.19.8`
- 仓库 `src/go.mod` 为 `go 1.25.7`
- 因此验证继续使用仓库既有约束中的 `/usr/local/go/bin/go`

## 未纳入本轮的内容

以下内容仍保持在 Issue #10 后续阶段：

- tmux 会话管理
- `ccclaw patrol`
- systemd patrol timer/service
- `--resume` 恢复逻辑
- prompt 归档
- journal 生成
- rtk 前后对比统计

## 结论

本轮已把 Phase 1 的关键底座接通：

- `ccclaw` 不再把 Claude 当成纯文本黑盒
- 已能沉淀 session/token/cost 基础数据
- 已有最小可用的 `stats` 命令消费这些数据

后续进入 Phase 2 时，可以直接在此基础上叠加 tmux 巡查与运行态观测，而不需要回头重做数据层。
