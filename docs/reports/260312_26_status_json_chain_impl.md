# 260312_26_status_json_chain_impl

## 背景

Issue #26 第一轮收口后，`scheduler timers` 已具备 `--json`，但观测链路仍有两个断点：

- 根级 `ccclaw status` 只有面向人工的文本快照
- `ccclaw scheduler status` 只有单行 `key=value` 文本

这会导致自动巡检若想串起“整机运行态 -> 调度后端判定 -> timer 明细”，仍然需要混合解析多种非稳定文本格式。

## 本次决策

延续 Issue #26 已确认的下一步建议，只补结构化出口，不改默认人工视图：

- `ccclaw status --json`
- `ccclaw scheduler status --json`

同时把调度状态判定收敛到 `internal/scheduler/status.go` 一处，避免根级 `status` 和 `scheduler status` 各自维护一套重复逻辑，后续字段继续扩展时再次漂移。

## 实现摘要

1. 在 `src/internal/scheduler/status.go` 新增统一 `StatusSnapshot`
- 统一输出 `requested/effective/reason/repair/context`
- 追加 `matches_request` 与 systemd/cron 原始探测字段
- 提供 `CollectStatusFromProbe()`，便于逻辑级回归测试复用

2. 在 `src/internal/app/runtime.go` 重构运行态快照采集
- 新增 `StatusWithOptions()` 支持 `JSON` 视图
- 根级 `status` 默认文本输出保持不变
- JSON 视图统一输出 `scheduler/executor/sessions/tasks/tokens/alerts`

3. 在 `src/cmd/ccclaw/main.go` 接入 CLI 标志
- `ccclaw status --json`
- `ccclaw scheduler status --json`

4. 文档同步
- `README.md`
- `src/dist/README.md`
- `src/ops/examples/app-readme.md`

## 验证

已执行：

```bash
cd src && go test ./...
```

通过。

## 后续收益

- 调度观测链路现在具备完整结构化出口：`status --json` / `scheduler status --json` / `scheduler timers --json`
- 后续若继续推进 `scheduler doctor --json`，可以沿用同一套字段命名与渲染分层
- 根级 `status` 不再复制调度诊断逻辑，后续维护成本更低
