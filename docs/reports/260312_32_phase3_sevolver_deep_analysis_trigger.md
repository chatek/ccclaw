# 260312_32_phase3_sevolver_deep_analysis_trigger

## 背景

Issue #32 的 `Phase 3` 已明确锁定为：

- `sevolver` 继续沿用纯 Go 的关键词提取
- 当缺口信号满足阈值时，自动创建 GitHub Issue
- 由新 Issue 进入既有 `ingest -> run` 链路，触发 Claude Code 深度分析闭环

在 `Phase 2` 完成后，当前系统已经能：

- 扫描 journal 中的 skill 使用与 gap signal
- 维护 skill 生命周期
- 持久化 `kb/assay/gap-signals.md`
- 生成 `sevolver` 日报

但还缺最后一环：

- 只能“记录缺口”，不能“把重复缺口升级成任务”
- gap signal 达到阈值后没有后续动作
- 缺少幂等控制，若直接每天创建 Issue，容易产生重复单

本轮目标是把这条链路补完整，同时处理重复触发的根因。

## 本轮实现

### 1. 新增 `deep_trigger.go`

新增：

- `src/internal/sevolver/deep_trigger.go`
- `src/internal/sevolver/deep_trigger_test.go`

核心能力：

- `ShouldTriggerDeepAnalysis()`：
  - 连续 3 个自然日出现同类 gap keyword 时触发
  - 未解决 gap backlog 累积达到 5 条时触发
- `MaybeTriggerDeepAnalysis()`：
  - 读取已聚合后的 backlog
  - 生成候选 deep-analysis plan
  - 检查控制仓库 open issues
  - 若已存在同指纹深度分析单，则直接复用
  - 若不存在，自动创建新 Issue

### 2. 引入“指纹 + open issue 复用”避免重复开单

如果只按“达到阈值就创建 Issue”实现，`ccclaw-sevolver.timer` 每晚执行时会对同一批未解决 gap 重复开单。

本轮补的幂等约束是：

- 连续型触发：指纹按 `consecutive + keyword` 生成
- 累积型触发：指纹按 `accumulated` 生成
- 新 Issue body 写入隐藏 marker：

```md
<!-- ccclaw:sevolver:deep-analysis:fingerprint=sg-xxxx -->
```

- 运行前先扫描 open issues
- 若命中同指纹，则：
  - 不再创建新单
  - 日报中记录为“复用已有 issue”

这样解决的是“重复 gap backlog 会导致 GitHub spam”的根因，而不是靠人工清理。

### 3. 深度分析 Issue 自动接入现有执行链路

`renderDeepAnalysisIssueBody()` 生成的 Issue body 现在包含：

- `target_repo: 41490/ccclaw`
- 触发日期
- 触发原因
- 当前 backlog 规模
- 触发样本 gap 列表
- Claude Code 应执行的分析要求

这样新建 Issue 会自动满足既有任务路由约束：

- 控制仓库仍然是 `41490/ccclaw`
- label 继续使用配置中的 `github.issue_label`
- `ingest` 拉取后可直接进入既有任务状态机

### 4. `sevolver.Run()` 改为基于 gap backlog 判定阈值

`Phase 2` 的 `result.Gaps` 只是“本轮扫描窗口内新发现的 gap”。

但 `Phase 3` 的阈值语义是“未解决缺口 backlog”，尤其是：

- 连续 3 天
- 累积 5 条未解决

因此本轮没有直接拿“当次新增 gap”做判断，而是新增：

- `LoadGapSignals()`

流程调整为：

1. `AppendGapSignals()` 先把新 gap 合并进 `gap-signals.md`
2. `LoadGapSignals()` 再读取受管区块，得到当前 backlog
3. `MaybeTriggerDeepAnalysis()` 基于 backlog 做阈值判定

这样阈值判断才和 Issue 规格一致。

### 5. 日报与 CLI 输出补充深度分析状态

修改：

- `src/internal/sevolver/sevolver.go`
- `src/internal/sevolver/reporter.go`

新增输出：

- 创建了新 Issue
- 复用了已有 Issue
- 未达到阈值
- 触发失败但不阻断本轮 sevolver

这保证了：

- skill 生命周期维护不会因为 GitHub 短时异常而整体失败
- 深度分析状态会稳定出现在 `kb/journal/sevolver/...` 日报中

### 6. GitHub 适配层新增创建 Issue 能力

修改：

- `src/internal/adapters/github/client.go`
- `src/internal/adapters/github/client_test.go`

新增：

- `Client.CreateIssue(title, body, labels)`

实现仍复用仓库现有 `gh` CLI 适配层，没有引入 GitHub SDK，也没有引入新依赖。

### 7. `ccclaw sevolver` 开始读取固定 `.env`

修改：

- `src/cmd/ccclaw/sevolver.go`
- `src/cmd/ccclaw/main.go`

变更点：

- `sevolver` 子命令现在会在 `.env` 存在时读取 secrets
- 然后把 secrets 传给 GitHub 适配层
- 若 `.env` 不存在，则保持兼容，不阻断 sevolver 本体执行

这样 Phase 3 的新行为仍符合项目约束：

- 敏感信息只从固定 `.env` 读取
- 不依赖调用者额外导出环境变量

## 验证

执行：

```bash
cd src && go test ./...
bash src/tests/install_regression.sh
```

结果：

- `go test ./...` 全量通过
- `install_regression.sh` 全量通过

本轮新增测试覆盖：

- 连续 3 天 gap 触发
- 累积 5 条 gap 触发
- 达到阈值后创建 deep-analysis issue
- 已存在 open deep-analysis issue 时复用而非重复创建
- `CreateIssue()` 参数与 label 透传
- `LoadGapSignals()` 读取受管 backlog

## 当前结论

Issue #32 的 `Phase 3` 已补齐到可部署状态：

- `sevolver` 已不再只是“记录问题”，而是能把重复 gap 升级成可执行任务
- deep-analysis 任务会自动进入既有 `ingest/run` 执行链路
- 通过 issue 指纹与 open issue 复用，避免了同一 backlog 的重复开单

至此，Issue #32 规划中的 `Phase 0 -> 1 -> 2 -> 3` 已全部落地完结。
