# Phase B 工程报告（Issue #2）

## 背景
- 父 Issue：#1
- 子 Issue：#2
- 目标：补齐 Phase B 中尚未落地的两项能力：
  - TodoWrite 风格计划模板
  - Task 风格子任务编排（explore / plan / code）

## 实施内容
1. 改造 `cmd/run/main.go`
- `buildPrompt()` 增加 Phase B 协议注入：
  - TodoWrite 五步计划模板（Explore / Plan / Code / Verify / Report）
  - 执行中状态更新要求（`[ ]` 到 `[x]`）
  - 子任务三阶段协议（EXPLORE / PLAN / CODE）及每阶段产物定义
- 新增 `buildPhaseBPrompt()`，统一维护 Phase B 文本模板。

2. 新增测试 `cmd/run/main_test.go`
- 覆盖点：
  - prompt 中必须包含 TodoWrite 模板关键段落
  - prompt 中必须包含 Task 子任务协议关键段落
  - docs 与 skills 匹配结果可正确注入 prompt

## 验证
执行：

```bash
go test ./...
```

结果：
- `cmd/run` 测试通过
- `internal/task` 测试通过
- 全仓无失败测试

## 结论
Phase B 的“计划化与子任务化”已补齐，当前 `ccclaw-run` 在执行任务时可强制产出可追溯计划，并按 explore/plan/code 阶段组织执行结果。

## 后续优化建议
1. 为 `buildPhaseBPrompt()` 增加更细粒度单元测试（逐段断言）。
2. 在 reporter 中增加“计划完成率”摘要字段，便于 Issue 侧快速审阅。
3. 后续可将计划状态写回本地任务状态文件，形成可机读的执行轨迹。
