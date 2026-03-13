# 260313 #35 tmux stdout 暂存收口优化

## 背景

在 `99b4a81` 完成“结构化结果文件 + 诊断文件”分离后，最终态的 `result.json` 语义已经稳定，但 tmux 执行期间仍有一个中间态问题：

- tmux 运行时 stdout 会直接落到结构化结果文件
- 如果执行尚未结束，或最终 stdout 不是稳定 JSON，运行中的 `result.json` 仍可能短暂承载非最终语义内容
- 这与 Issue #35 想收紧的“结构化结果文件只承载最终机器可解析结果”仍有边界残留

因此本轮继续推进上一轮评论中建议的第 1 项后续优化：把 tmux 路径的 stdout 先收进内部暂存文件，等 patrol 收口时再决定是否提升为结构化结果。

## 本轮实现

### 1. 新增 tmux stdout 暂存文件

执行器内部新增：

- `var/results/<task>.stdout.txt`

它只作为 tmux 执行期的内部中间产物使用，不对外宣称为最终结果文件。

当前 tmux 路径改为：

- stdout -> `stdout.txt` + 汇总日志
- stderr -> `diag.txt` + 汇总日志
- `result.json` 在运行期间保持空文件

### 2. 收口阶段负责“提升或迁移”

`LoadResult()` 现在在读取最终结果前，会先处理 `stdout.txt`：

- 若 `stdout.txt` 是 machine-readable Claude JSON
  - 提升为 `result.json`
  - 删除 `stdout.txt`
- 若 `stdout.txt` 不是有效结构化 JSON
  - 合并进 `diag.txt`
  - 清空 `result.json`
  - 删除 `stdout.txt`

这样 tmux 路径不再把中间 stdout 直接当作最终结构化结果。

### 3. 同步路径语义顺带统一

本轮也顺带把同步执行路径的持久化规则对齐：

- 只要 stdout 是 machine-readable Claude JSON，即使语义上是 Claude error result，也仍保留在 `result.json`
- 只有 stdout 连结构化 JSON 都不是时，才清空 `result.json` 并把内容转入 `diag.txt`

这样“结构化结果文件只承载机器可解析 JSON”这一语义在同步与 tmux 两条路径上保持一致。

## 测试

新增/调整了以下回归测试：

- tmux 启动命令断言改为检查 `.stdout.txt` / `.diag.txt` 双暂存写入
- `LoadResult()` 可把 `stdout.txt` 中的成功 JSON 提升为 `result.json`
- `LoadResult()` 可把 `stdout.txt` 中的非 JSON 文本迁移到 `diag.txt`
- patrol 可从 `stdout.txt` 收口已退出 tmux 会话，并在收口后生成最终 `result.json`
- 同步执行遇到 Claude 结构化 error result 时，`result.json` 继续保留原始 JSON

## 验证

已执行：

```bash
go test ./internal/executor ./internal/app
```

随后再执行：

```bash
go test ./...
```

## 结果

- tmux 运行期间的 `result.json` 不再承载中间 stdout
- 结构化结果文件只在收口确认后才落最终语义
- 诊断文件继续保留独立排障落点
- 同步与 tmux 两条结果持久化路径的语义进一步统一

## 后续建议

1. 若后续继续做统计/审计，可把 `stdout.txt` 的“已提升/已迁移”状态写成 machine-readable 产物索引，而不只靠文件是否存在判断。
2. `status` / `doctor` 可补一个最近任务产物快照，直接显示 `result/diag/stdout-staging` 三者命中情况，方便现场观察收口状态。
