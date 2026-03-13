# 260313 Issue #26 `scheduler doctor --json` 收口报告

## 背景

Issue #26 的上一轮建议中，最后一条已明确优先补齐 `scheduler doctor --json`，让调度观测链路形成完整的结构化出口：

- `status --json`
- `scheduler status --json`
- `scheduler doctor --json`
- `scheduler timers --json`

在本次修改前，`scheduler doctor` 仍只有面向人工的文本输出，巡检脚本无法稳定读取失败数、逐项诊断结果和修复建议。

## 本轮实现

1. 为 `scheduler doctor` 新增 `--json`
- 命令入口位于 `src/cmd/ccclaw/main.go`
- 默认人工输出保持不变
- `--json` 时输出固定结构：
  - `summary.total`
  - `summary.passed`
  - `summary.failed`
  - `checks[].name`
  - `checks[].ok`
  - `checks[].detail`
  - `checks[].repair`
  - `checks[].error`

2. 保持失败语义不变
- 即使使用 `--json`，只要存在失败检查项，命令仍返回非零错误
- 这样外部脚本既能拿到结构化载荷，也不会丢掉原有退出码语义

3. 补齐回归
- 命令级回归：`scheduler doctor --json`
- 内部回归：失败场景下仍输出可解析 JSON，并保留失败统计

4. 文档同步
- 根 README
- `src/dist/README.md`
- 程序目录样板 `src/ops/examples/app-readme.md`
- release 样板 `src/dist/ops/examples/app-readme.md`

## 验证

已执行：

```bash
cd src && gofmt -w cmd/ccclaw/main.go cmd/ccclaw/main_test.go internal/scheduler/doctor.go internal/scheduler/doctor_test.go
cd src && go test ./cmd/ccclaw ./internal/scheduler
cd src && go test ./...
```

## 结果

Issue #26 建议中的第 1 点已完成，`doctor` 不再是观测链路里唯一缺失结构化出口的节点。
