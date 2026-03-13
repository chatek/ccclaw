# 260313_36_deep_analysis_backfill

## 背景

Issue #36 的第一项待办要求把 deep-analysis issue 的触发结果显式回写到：

- `kb/assay/gap-signals.md`
- 相关 Skill 的 YAML frontmatter

当前主干只有“创建或复用 open issue”的幂等控制，没有把“哪个 gap 已经升级处理中”沉淀到本地台账与 Skill 元数据里，导致：

- `gap-signals.md` 只能表示“有缺口”，不能表示“缺口已升级处理中”
- Skill frontmatter 的 `gap_signals` 字段没有真正回填具体 gap ID
- 一旦 gap 过了当次扫描窗口，后续很难再推导它和哪个 Skill/哪个深度分析单有关

## 本轮实现

### 1. 扩展 GapSignal 元数据模型

修改：

- `src/internal/sevolver/scanner.go`
- `src/internal/sevolver/gap_recorder.go`

新增字段：

- `related_skills`
- `escalation_status`
- `escalation_fingerprint`
- `escalation_issue_number`
- `escalation_issue_url`
- `escalation_updated_at`

其中：

- gap 记录主行仍保持 `- [gap-id] 日期 | 关键词 | 来源 | 上下文`
- 附加元数据改为缩进行格式，便于人工查看且避免和 `context` 的 `|` 冲突

这样 `gap-signals.md` 可以显式表达“该缺口已经升级为某个 deep-analysis issue 正在处理中”。

### 2. 扫描期即补齐 gap 与 skill 的关联来源

修改：

- `src/internal/sevolver/scanner.go`

现有实现只会扫描：

- Skill 命中：`kb/skills/...`
- Gap 信号：失败/卡住/找不到等关键词

但两者之间没有稳定关联，导致后续回写 Skill frontmatter 时无从下手。

本轮改为：

- 扫描 gap 时，同时提取同一 journal 文件内出现的 Skill 路径
- 优先使用触发该行中直接出现的 Skill 路径
- 若该行没有 Skill 路径，则回退到同一文件内已出现的 Skill 路径集合

这不是完美语义理解，但在当前已拍板的“纯 Go + journal 路径命中”边界内，已经把 gap→skill 的关联链补成了可持久化状态。

### 3. 新增 deep-analysis 回写编排

新增：

- `src/internal/sevolver/escalation_backfill.go`
- `src/internal/sevolver/escalation_backfill_test.go`

`BackfillDeepAnalysisEscalation()` 会在 deep-analysis issue 创建或复用成功后执行：

1. 用 `DeepAnalysisDecision.RelevantGaps` 锁定本次升级的 gap 集合
2. 回写 `gap-signals.md`：
   - `escalation_status: escalated`
   - 指纹、Issue 编号、Issue URL、更新时间
3. 回写相关 Skill frontmatter：
   - `gap_signals` 追加具体 gap ID
   - `gap_escalations` 追加结构化升级记录

Skill 新字段示例：

```yaml
gap_escalations:
  - fingerprint: sg-demo
    status: escalated
    issue_number: 88
    issue_url: https://github.com/41490/ccclaw/issues/88
    updated_at: "2026-03-12"
    gap_ids:
      - gap-20260312-demo
```

这样第一项待办要求的“显式标记已升级处理中”就不再依赖 open issue 扫描的隐式推断。

### 4. 安装/升级保护链路同步纳入新字段

修改：

- `src/dist/install.sh`
- `src/tests/install_regression.sh`
- `src/dist/kb/skills/CLAUDE.md`
- `src/dist/kb/skills/L1/CLAUDE.md`
- `src/dist/kb/skills/L2/CLAUDE.md`

原因：

- `gap_escalations` 属于 `sevolver` 自动维护字段
- 如果只改运行时不改 upgrade 保护，下次升级会把回写状态覆盖掉

因此本轮把 `gap_escalations` 纳入：

- frontmatter 保留字段列表
- 安装回归测试
- Skill 模板文档说明

## 验证

执行：

```bash
cd src && go test ./internal/sevolver/...
cd src && go test ./...
bash src/tests/install_regression.sh
```

预期覆盖：

- gap 扫描时携带相关 Skill 路径
- `gap-signals.md` 新元数据写入与回读
- deep-analysis 回写同时更新 gap 台账与 Skill frontmatter
- install/upgrade 过程保留 `gap_escalations`

## 当前结论

Issue #36 的第一项已落地到代码层面：

- deep-analysis issue 不再只是“远端有单”，而是会回写本地 gap 台账
- 相关 Skill frontmatter 现在会持久化 gap ID 与升级处理中状态
- 后续第 4 项“issue 关闭后回写已收敛/已处理”已有明确状态载体可继续扩展
