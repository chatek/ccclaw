# 260313_36_deep_analysis_resolution_closure

## 背景

Issue #36 的第 4 项待办要求：

- 在 deep-analysis issue 关闭后，自动回写 gap 台账为“已收敛 / 已处理”
- 让 `sevolver` 真正形成长期闭环，而不是只会“发现 gap -> 开 issue -> 永远挂在 backlog 里”

前 3 项完成后，仓库已经具备：

- `gap-signals.md` 中的升级元数据
- Skill frontmatter 中的 `gap_escalations`
- deep-analysis issue 的 fingerprint / issue number / issue URL 回填

但主流程仍存在一个根因问题：

- 已关闭的 deep-analysis issue 不会自动回写本地状态
- `LoadGapSignals()` 读出的 backlog 会继续把这些旧 gap 当作未解决项参与阈值判断
- 结果是“远端 issue 已结束，本地 backlog 仍未收敛”，闭环不完整

## 本轮实现

### 1. 在 `sevolver.Run()` 里先做关闭态收敛，再算 backlog

修改：

- `src/internal/sevolver/sevolver.go`
- `src/internal/sevolver/escalation_backfill.go`

新增编排：

1. `LoadGapSignals()` 读出最近 30 天 gap 台账
2. `ResolveClosedDeepAnalysisEscalations()` 扫描所有 `escalated` 状态且带 `issue_number` 的 gap
3. 通过 GitHub client 读取对应 issue 状态
4. 若 issue 已关闭，则先回写本地状态
5. 再用过滤后的 active backlog 决定是否继续触发新的 deep-analysis

这样处理顺序才正确：

- 先收敛已关闭事项
- 再计算“当前未解决缺口总数”

避免已关闭 issue 残留在 backlog 中继续触发后续升级逻辑。

### 2. gap 台账回写 `resolved`

对命中的 gap 条目回写：

- `escalation_status: resolved`
- `escalation_updated_at: YYYY-MM-DD`

保留：

- `escalation_fingerprint`
- `escalation_issue_number`
- `escalation_issue_url`

这样 gap 台账既能保留历史链路，又能明确区分：

- `escalated`: 已升级处理中
- `resolved`: 关联 deep-analysis issue 已关闭，本地缺口台账已完成收敛回写

### 3. Skill frontmatter 回写 `converged`

修改：

- `src/internal/sevolver/skill_updater.go`

新增 `ApplySkillGapEscalationResolution()`：

- 通过 `fingerprint` 或 `issue_number` 匹配已有 `gap_escalations`
- 将 `gap_escalations[].status` 从 `escalated` 更新为 `converged`
- 同步刷新 `updated_at`
- 保留并补齐 `gap_ids`

这里把状态语义拆开：

- gap 台账侧用 `resolved`
- skill 生命周期侧用 `converged`

原因是两者关注点不同：

- gap 台账强调“该缺口条目已经处理完成”
- skill 元数据强调“围绕这组缺口触发的一轮自进化分析已经收敛”

### 4. 日报与 CLI 输出补充收敛结果

修改：

- `src/internal/sevolver/reporter.go`
- `src/internal/sevolver/sevolver.go`

新增输出：

- 收敛关闭的 issue 数量
- 收敛回写的 gap 数量
- 收敛回写的 skill 数量

这样 nightly `sevolver` 跑完后，可以直接从日志和日报看出：

- 本轮有没有新升级
- 也有没有旧升级完成收敛

### 5. 文档补齐新状态语义

修改：

- `src/dist/kb/skills/CLAUDE.md`
- `src/dist/kb/skills/L1/CLAUDE.md`
- `src/dist/kb/skills/L2/CLAUDE.md`

补充说明：

- `gap_escalations[].status: converged`

避免后续升级或人工排查时，只知道 `escalated`，却不知道闭环结束后的标准状态名是什么。

## 验证

执行：

```bash
cd src && go test ./internal/sevolver/...
cd src && go test ./...
```

新增覆盖点：

- closed issue 会把 `gap-signals.md` 中对应条目标记为 `resolved`
- 对应 Skill frontmatter 会把 `gap_escalations[].status` 更新为 `converged`
- `sevolver.Run()` 会先做收敛回写，再决定是否触发新的 deep-analysis
- 已 `resolved` 的 gap 不再进入 active backlog

## 当前结论

Issue #36 的第 4 项已落地：

- deep-analysis issue 关闭后，本地 gap 台账会自动收敛
- Skill frontmatter 会同步记录 `converged` 状态
- `sevolver` 后续阈值判断不再把已关闭 issue 对应的 gap 当成未解决 backlog

至此，#36 列出的 4 项闭环增强均已具备代码实现与测试覆盖。
