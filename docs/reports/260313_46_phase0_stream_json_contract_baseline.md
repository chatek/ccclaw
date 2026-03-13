# 260313_46_phase0_stream_json_contract_baseline

## 背景

- 执行来源：Issue #46 `phase0: stream-json 事件契约冻结与解析基线`
- 关联决策：#45 已拍板 `Go stream-json daemon` 作为主执行方向，并按 `46 -> 47 -> 48 -> 49` 推进
- 本轮范围：仅冻结事件契约与解析基线，不改主执行器判定逻辑

## 门禁核验

- Issue 发起者 `ZoomQuiet` 在仓库权限为 `admin`（含 `maintain`），满足自动执行门禁
- 本轮未引入未拍板分歧项

## 本轮实现

### 1. 冻结最小事件集与契约结构

在 `src/internal/executor/stream_contract.go` 新增 Phase 0 契约模型：

- 事件集合：`started/progress/usage/result/error/restarted`
- 解析入口：`ParseStreamJSONL`
- 聚合入口：`AggregateStreamEvents`
- 映射规则：`MapStreamEvent`

新增契约快照结构 `StreamEventSnapshot`，固定 `schema_version=stream_event_contract/v1`，作为后续 Phase 1/2 的事实基线。

### 2. 固化落盘格式

在执行器产物路径中补充：

- `*.stream.jsonl`：原始事件流
- `*.event.json`：聚合快照

涉及文件：`src/internal/executor/executor.go`、`src/internal/executor/stream_artifact.go`。

### 3. 增加 runtime 映射钩子（非判定）

在 `src/internal/app/runtime_cycle.go` 增加 `observeStreamEventContract`：

- 读取 `*.event.json`（缺失则按 `*.stream.jsonl` 重建）
- 将快照映射为 `RepoSlot(phase/current_step)` 与 `Task(state/event)` 建议值
- 仅写 debug 日志，不改变现有主流程状态判定

满足“仅加映射钩子，不改主流程判定”。

### 4. Fixtures 与单测

新增 fixtures：

- `src/internal/executor/testdata/stream_contract/success.stream.jsonl`
- `src/internal/executor/testdata/stream_contract/restart_error.stream.jsonl`
- 对应聚合期望：`*.event.json`

新增测试：

- `src/internal/executor/stream_contract_test.go`
  - fixtures 重放一致性（同批样本重复聚合结果一致）
  - 原始流重建快照并回读一致
  - 未知事件拒绝校验
- `src/internal/app/runtime_cycle_stream_test.go`
  - runtime 映射转换与兜底行为

## 验证结果

已执行：

- `go test ./internal/executor ./internal/app`
- `go test ./...`

结果：全部通过。

## 风险与后续

- 当前主执行器仍使用 `--output-format json`，本轮仅建立契约与解析基线
- Phase 1 可在 tmux 载体下切到 `stream-json` 后直接复用本轮 `*.stream.jsonl -> *.event.json` 链路做影子对账
