# 260313_39_task_events_structured_gap_fields

## 背景

- 执行来源：Issue #39 `followup: task_events 缺口信号结构化字段与聚合精度提升`
- 现状问题：task_events 聚合主要依赖 `detail` 文本关键词，稳定性与可复用性不足。

## 本轮实现

### 1. task_events 结构化字段落盘

在 `internal/adapters/storage` 的事件记录模型中新增字段：

- `gap_keyword`
- `gap_scope`
- `gap_source`
- `root_cause_hint`
- `gap_aggregatable`
- `gap_escalatable`

并新增结构化写入接口：

- `AppendEventStructured(...)`
- `AppendEventAtStructured(...)`

### 2. 写入策略（兼容旧调用）

为不改动现有 runtime 调用面，`AppendEvent/AppendEventAt` 默认自动推断结构化字段：

- 基于 `event_type + detail` 推断 `gap_keyword`
- 生成标准 `gap_source` 与 `gap_scope`
- 生成 `root_cause_hint`
- 赋值 `gap_aggregatable/gap_escalatable`

同时支持显式结构化覆盖（通过 `AppendEvent*Structured`）。

### 3. sevolver 聚合逻辑改为“结构化优先，文本兜底”

`internal/sevolver/task_event_scanner.go` 调整为：

1. 若存在 `gap_keyword`，优先使用；
2. 若 `gap_aggregatable=false`，直接跳过；
3. 无结构化字段时，回退到原有 `detail` 关键词扫描与 eventType fallback；
4. `source/context` 优先使用结构化字段，保证链路可追踪。

### 4. 测试补齐

- `internal/adapters/storage/jsonl_test.go`
  - 校验事件结构化字段自动写入行为
- `internal/sevolver/task_event_scanner_test.go`
  - 校验结构化优先策略
  - 校验 `gap_aggregatable=false` 被正确过滤

## 验证

- `cd src && go test -count=1 ./internal/adapters/storage ./internal/sevolver`
- `cd src && go test ./...`
- 结果：全部通过

## 兼容性说明

- 对历史 JSONL 事件完全兼容：旧记录无结构化字段时自动走文本兜底。
- 不改变既有 task_events 主字段与查询路径，保持当前闭环流程稳定。

## 后续优化建议

1. 在 runtime 的关键事件写入点补“显式业务语义”字段，逐步替换纯推断。
2. 为 `gap_source/gap_scope` 建立稳定枚举，减少后续统计口径漂移。
3. 在 sevolver 报告中追加“结构化命中率”指标，持续观测聚合精度提升效果。
