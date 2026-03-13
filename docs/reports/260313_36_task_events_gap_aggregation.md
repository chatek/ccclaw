# 260313_36_task_events_gap_aggregation

## 背景

Issue #36 的第 3 项待办要求评估并推进：

- 把 `task_events` 纳入缺口聚合
- 不再只依赖 journal 文本关键词
- 降低纯关键词扫描带来的噪声与漏报

当前主干的 `sevolver` 缺口来源只有：

- journal 文件里的自然语言文本
- `signal-rules.md` / 默认关键词匹配

这会漏掉一类真实缺口：

- 任务运行时已经在 `task_events` 记录了 `FAILED / DEAD / BLOCKED / WARNING`
- 但 journal 并没有同步写出对应描述
- 结果是实际执行问题不会进入 `gap-signals.md`

## 本轮实现

### 1. storage 增加时间窗内全量事件读取

修改：

- `src/internal/adapters/storage/store.go`

新增：

- `ListAllTaskEventsBetween(start, end)`

原因：

- 现有 `ListTaskEventsBetween()` 带 limit 语义，默认只取最近 50 条
- `sevolver` 做缺口聚合时需要完整时间窗，而不是仅取分页视图

因此本轮没有复用带 limit 的接口，而是补了专门的全量读取入口。

### 2. 新增 `task_events` 缺口扫描器

新增：

- `src/internal/sevolver/task_event_scanner.go`
- `src/internal/sevolver/task_event_scanner_test.go`

实现策略：

- 只扫描语义上可能代表缺口的事件类型：
  - `BLOCKED`
  - `FAILED`
  - `DEAD`
  - `WARNING`
- 先用现有 gap 关键词匹配 `event.Detail`
- 若 detail 未命中关键词，则对强语义事件给保守兜底：
  - `FAILED / DEAD` => `失败`
  - `BLOCKED` => `阻塞`

这样比“所有 task_event 明文全文扫词”更稳：

- 噪声更小，因为先用事件类型做门控
- 漏报更少，因为失败/阻塞类事件即使 detail 没写关键词，也能进入 backlog

### 3. `sevolver.Run()` 合并 journal 与 task_events 两路缺口

修改：

- `src/internal/sevolver/sevolver.go`
- `src/cmd/ccclaw/sevolver.go`

变更点：

- `Config` 新增 `StateDBPath`
- `ccclaw sevolver` 运行时传入 `cfg.Paths.StateDB`
- `Run()` 先扫 journal gaps，再扫 `task_events` gaps
- 两路信号通过 `mergeGapSignals()` 合并去重

这样 `gap-signals.md` 的 backlog 已不再只是 journal 文本产物，而是：

- journal 显式记录的能力缺口
- 运行态事件流中的失败/阻塞/告警信号

### 4. 日报与 CLI 输出补 `task_events` 观测面

修改：

- `src/internal/sevolver/reporter.go`
- `src/internal/sevolver/sevolver.go`

新增输出：

- CLI：`task_event_gaps=<N>`
- 日报摘要：`task_events 缺口: N`
- 日报明细：新增 `## task_events 缺口`

这样可以区分：

- 当日新增 gap 有多少来自 journal
- 有多少来自运行态事件流

避免后续观察时只看到总数，却不知道来源结构。

## 验证

执行：

```bash
cd src && go test ./internal/sevolver ./internal/adapters/storage ./cmd/ccclaw
cd src && go test ./...
```

新增覆盖：

- `FAILED` 事件在无显式关键词时会兜底归类为 `失败`
- `BLOCKED` 事件会兜底归类为 `阻塞`
- `DONE` 等非缺口事件不会进入 gap 聚合
- `ccclaw sevolver` 会把 `StateDBPath` 传给运行时

## 当前结论

Issue #36 的第 3 项已推进到代码实现：

- `gap` 聚合已同时覆盖 journal 与 `task_events`
- 对失败/阻塞类事件的识别不再完全依赖 journal 明文关键词
- 后续第 4 项“issue 关闭后回写已处理”可以直接基于更完整的 gap backlog 继续做闭环
