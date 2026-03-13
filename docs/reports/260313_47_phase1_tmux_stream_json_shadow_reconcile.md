# 260313_47_phase1_tmux_stream_json_shadow_reconcile

## 背景

- 执行来源：Issue #47 `phase1: tmux 下切 stream-json 影子对账`
- 前置依赖：#46 已完成事件契约冻结与解析基线
- 本轮目标：不更换执行载体（仍为 tmux），仅切输出协议为 `stream-json`，并保持 finalize 语义不变

## 本轮实现

### 1. 执行输出协议切换与可回滚开关

在 `src/internal/executor/executor.go`：

- 默认输出协议由 `json` 切换为 `stream-json`
- 新增回滚开关：`CCCLAW_OUTPUT_FORMAT=json`
  - 未设置时默认 `stream-json`
  - 需要紧急回退时可一键切回 `json`
- 元数据新增 `output_format`，写入 `*.meta.json`

### 2. stream-json 与兼容 result 的双产物

仍保持现有 `result/diag/log/meta` 兼容，同时新增 stream 产物链路：

- `*.stream.jsonl`：原始流
- `*.event.json`：聚合快照
- `*.json`：兼容结构化结果（由 stream 快照转换生成）

在 sync/tmux 两条路径均支持：

- sync：执行后立即聚合 stream 并写兼容 result
- tmux finalize：从 `stdout` 暂存 materialize 为 stream/event/compat-result

### 3. finalize 影子对账指标

在 `src/internal/app/runtime_cycle.go`：

- finalize 时读取 stream 快照并进行影子对账
- 对账维度：`expected(done|failed)` vs `actual(done|failed)`
- 对账结果记录为 runtime 日志；若偏差则追加 `WARNING` 事件
- 不改变现有 `DONE/FAILED/DEAD` 主判定流程

### 4. 测试补齐

- `src/internal/executor/executor_test.go`
  - 断言 tmux 命令默认携带 `--output-format stream-json`
  - 断言 `CCCLAW_OUTPUT_FORMAT=json` 可回滚
  - 新增 stream stdout materialize 成功/失败用例
- `src/internal/app/runtime_cycle_stream_test.go`
  - 新增影子对账结果推导测试
- 全量回归：`go test ./...` 通过

## 对账结论

基于当前单测与 fixtures 回放：

- stream 输出路径能够稳定生成兼容 result
- finalize 主判定语义保持不变
- 对账异常可被日志与事件捕获

本轮未发现额外“卡 RUNNING 无进展”路径引入。

## 回滚方案

- 将 `.env` 中 `CCCLAW_OUTPUT_FORMAT` 设为 `json` 即可回切
- 不涉及执行载体切换，tmux 主链路保持不变

## 后续优化建议

1. 将影子对账统计落盘为日维度指标（match/mismatch 数量与比例），便于验证 >=99% 目标。
2. 在 status JSON 中增加只读对账摘要，便于外部观测。
3. 针对线上样本补充回放命令，形成固定对账基准集（本地 + CI）。
