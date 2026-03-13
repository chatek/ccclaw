# 260313_36_sevolver_task_classification

## 背景

Issue #36 的第 2 项待办要求：

- 让 `ingest` / `run` 对 `[sevolver]` 类 issue 提供更明确的任务分类
- 或提供稳定的模板识别入口
- 便于后续统计、筛选与回顾自进化任务

当前主干的任务模型只有：

- `Intent`
- `RiskLevel`
- `Labels`

但没有“任务分类/模板类型”这一层语义字段，导致：

- `ingest` 只能把 `[sevolver]` issue 当普通 research/fix 单处理
- `run` 无法按自进化任务模板生成更聚焦的 prompt
- 后续状态快照、统计与人工回顾只能重新扫标题前缀，缺少结构化字段

## 本轮实现

### 1. 新增任务分类字段 `TaskClass`

修改：

- `src/internal/core/task.go`
- `src/internal/core/task_test.go`

新增：

- `TaskClassGeneral`
- `TaskClassSevolver`
- `TaskClassSevolverDeepAnalysis`

并补齐：

- `InferTaskClass(title, body, labels)`

识别优先级：

1. 先读 Issue body 中显式 `task_class: ...`
2. 再回退到标题前缀 `[sevolver]`
3. 若命中 deep-analysis marker 或标题含“能力缺口深度分析”，归类为 `sevolver_deep_analysis`
4. 否则回退为 `general`

这样新老任务都能稳定识别：

- 新生成的 sevolver issue 走显式字段
- 历史 issue 仍可靠标题兼容识别

### 2. deep-analysis issue 生成链路补 `task_class` 与 `sevolver` 标签

修改：

- `src/internal/sevolver/deep_trigger.go`
- `src/internal/sevolver/deep_trigger_test.go`

现在 `renderDeepAnalysisIssueBody()` 会额外写入：

```md
task_class: sevolver_deep_analysis
```

并在创建 issue 时附加 `sevolver` label。

这解决了两个问题：

- `ingest` 不再只能靠标题猜测 deep-analysis 类型
- GitHub 侧也有显式标签，后续筛选自进化任务更直接

### 3. `ingest` 持久化分类，`run` 按分类切模板

修改：

- `src/internal/app/runtime.go`
- `src/internal/app/runtime_test.go`
- `src/internal/app/runtime_logging.go`

具体改动：

- `syncIssue()` 在入队时写入 `task.TaskClass`
- 入队事件详情与运行日志带出 `task_class`
- `buildPrompt()` 元数据区新增“任务分类”
- 对 `sevolver_deep_analysis` / `sevolver` 追加专项模板段

其中 `sevolver_deep_analysis` 模板会明确要求：

- 按 gap 样本与 fingerprint 收敛根因
- 不把样本逐条当普通 bug 机械修补
- 优先判断落点属于 `internal/sevolver`、`kb/skills/`、`kb/assay/` 还是 `ingest/run`
- 修改知识资产时同步考虑 gap 台账与 Skill frontmatter

这让 `run` 在面对 `[sevolver]` 任务时不再使用完全通用的 prompt。

### 4. 运行态快照显式展示任务分类

修改：

- `src/internal/app/runtime.go`
- `src/internal/app/runtime_test.go`
- `src/internal/adapters/storage/state_store_test.go`

状态快照现在会输出 `task_class`：

- JSON 里有 `tasks.items[].task_class`
- 人类可读表格里新增 `CLASS` 列
- `state.json` 快照持久化也会保留该字段

这为后续统计/筛选提供了稳定载体，而不是每次再从 `IssueTitle` 反推。

## 验证

执行：

```bash
cd src && go test ./internal/core ./internal/sevolver ./internal/app ./internal/adapters/storage
cd src && go test ./...
```

新增覆盖：

- `TaskClass` 推断
- sevolver deep-analysis issue 入队分类
- sevolver 专项 prompt 模板
- status JSON/human 输出分类字段
- `state.json` 持久化分类字段
- deep-analysis issue body 与 labels 同步补齐分类信息

## 当前结论

Issue #36 的第 2 项已落地：

- `ingest` 现在会结构化识别 `[sevolver]` 类 issue
- `run` 会按 `TaskClass` 切换专项模板，不再完全按通用任务执行
- `status/state` 已能显式展示并持久化自进化任务分类

后续若继续按顺序推进，第 3 项可以基于当前的 `TaskClass` 与 `task_events` 结构，评估如何把事件流也纳入缺口聚合。 
