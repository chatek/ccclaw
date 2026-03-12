# 260312_32_phase1_jsonl_state_store_foundation

## 背景

Issue #32 的 `Phase 1` 目标是用 JSONL + 周级哈希链替代 SQLite `state_db`，优先解决 #31 暴露出的 SQLite 锁冲突根因。

当前代码现实与设计草案之间有一个关键落差：

- 设计草案里的 `state.json` 只描述“当前状态缓存”
- 但现有运行时、统计、journal、doctor 与测试，实际都依赖 `Store` 提供的完整任务快照、事件流与 token 聚合能力

如果直接按草案只保留极瘦 `state.json`，会上层大面积失配。因此本轮先按“兼容现有 `Store` API、替换底层实现”的方式推进。

## 本轮实现

### 1. `storage.Store` 底层已不再依赖 SQLite

删除原 `src/internal/adapters/storage/sqlite.go`，改为：

- `src/internal/adapters/storage/store.go`
- `src/internal/adapters/storage/state_store.go`
- `src/internal/adapters/storage/jsonl.go`
- `src/internal/adapters/storage/hashchain.go`

新的 `Store` 仍保留原方法集：

- `GetByIdempotency`
- `UpsertTask`
- `AppendEvent`
- `RecordTokenUsage`
- `ListTasks`
- `ListRunnable`
- `ListRunning`
- `TokenStats*`
- `Journal*`
- `ListTaskEvents*`
- `RTKComparison*`

因此 `internal/app` 与 `cmd/ccclaw` 调用面无需改写。

### 2. 任务快照改由 `state.json` 原子持久化

实现方式：

- `state.json` 落在 `var/` 目录
- 结构为 `map[task_id]*core.Task`
- 每次 `UpsertTask`：
  - 加独立锁文件
  - 重新读取当前 `state.json`
  - 合并指定 task 快照
  - 临时文件写入
  - `os.Rename` 原子替换

这一步并未引入 SQLite，也避免了旧 WAL 锁冲突。

说明：

- 为保证与现有运行时兼容，当前 `state.json` 保存的是**完整任务快照**，不只是 `state/updated_at/seq`
- 这是对设计草案的兼容性补强，而不是功能扩张

### 3. 事件流与 token 流改为按周 JSONL

实现方式：

- `events-YYYY-WNN.jsonl`
- `token-YYYY-WNN.jsonl`

每条记录都包含：

- `seq`
- `prev_hash`
- `hash`

事件记录额外包含：

- `task_id`
- `event_type`
- `detail`
- `ts`

token 记录额外包含：

- `session_id`
- `input/output/cache_*`
- `cost_usd`
- `duration_ms`
- `rtk_enabled`
- `prompt_file`
- `ts`

### 4. 周级哈希链已落地

实现：

- `GenesisHash(year, week)`
- `ComputeHash(...)`
- `ComputeTokenHash(...)`
- `VerifyChain(path)`

当前策略：

- 每周独立链
- 首条记录 `prev_hash = GENESIS-WYYYY-WW`
- `VerifyChain()` 会验证：
  - `seq` 递增
  - `prev_hash` 串接正确
  - 当前记录 `hash` 与 canonical 字段重新计算结果一致

### 5. 查询聚合先用 Go 扫描，未在本轮强接 DuckDB CLI

为了先稳定替换写入底座、保证现有行为不回退，本轮统计与 journal 查询先直接扫描 JSONL 并在 Go 内聚合：

- token 汇总
- task token 汇总
- daily token 汇总
- RTK 对比
- journal 日汇总
- journal task 汇总
- 事件列表

这一步的意义是：

- 先把 SQLite 从运行路径移除
- 先让 app/runtime 与现有测试全部在新底座上跑通
- 再在后续切片中补 `DuckDB CLI + archive`，而不把两类风险叠在同一轮

## 运行时同步

同步把 `doctor` 中的文案从：

- `sqlite3 CLI`
- `SQLite 读写`

改成：

- `duckdb CLI`
- `状态存储读写`

这样诊断语义已与新底座一致。

## 测试

新增：

- `src/internal/adapters/storage/hashchain_test.go`
- `src/internal/adapters/storage/jsonl_test.go`
- `src/internal/adapters/storage/state_store_test.go`

并保留原有 `storage` / `runtime` 相关测试直接跑新实现。

执行：

```bash
cd src
go test ./internal/adapters/storage/...
go test ./internal/app/...
go test ./internal/config/...
go test ./...
```

结果：

- 全部通过

其中 `go test ./...` 期间还顺手修复了一条既有测试样本：

- `internal/config/config_test.go`
- 原因：`scheduler.logs.retention_days` 新校验已存在，但旧 fixture 未补默认字段

## 当前结论

Issue #32 `Phase 1` 已完成其最关键的基础替换：

- SQLite 已退出 `storage.Store` 运行路径
- 任务快照改为 `state.json`
- 事件与 token 写入改为周级 JSONL
- 哈希链校验已具备
- 现有 app/runtime 与全量测试均已跑通

## 后续待续切片

本轮**尚未**完成的 `Phase 1` 后续项：

1. `DuckDB CLI` 查询适配器
2. `ccclaw archive` 子命令
3. 每周 archive systemd timer/service
4. JSONL → Parquet 的非当前周归档流程

这些项建议作为 `Phase 1` 的后半段继续推进，但不影响“SQLite 运行时退出主路径”这一主目标已经达成。
