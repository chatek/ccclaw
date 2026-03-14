# 260313_38_deep_analysis_close_reason_taxonomy

## 背景

- 执行来源：Issue #38 `followup: deep-analysis 关闭结果细分与闭环状态语义扩展`
- 现状问题：deep-analysis 关闭后仅有 `closed -> resolved/converged`，无法区分关闭语义。

## 本轮实现

### 1. 关闭结果分类枚举（可机器判定）

在 `internal/sevolver/escalation_backfill.go` 增加关闭原因分类：

- `fixed`（已修复）
- `duplicate`（重复问题）
- `external_resolved`（外部依赖已解除）
- `archived`（归档不做）
- `cannot_reproduce`（无法复现）
- `closed`（未识别，兜底）

### 2. 关闭原因提取链路（标签/正文/评论）

新增关闭原因解析逻辑 `inferIssueCloseReason(...)`，按以下优先级提取：

1. Issue Body 中显式字段（`close_reason:` / `state_reason:`）
2. Issue Labels（如 `fixed` / `duplicate` / `not planned` / `cannot_reproduce`）
3. Done Comment 与最新评论文本
4. 无法识别时回落为 `closed`

### 3. 本地状态映射扩展（保持 resolved/converged 兼容）

- `gap-signals.md`：
  - 为 gap 元数据补充 `escalation_close_reason`
  - 解析/渲染/合并链路全量支持
- Skill frontmatter `gap_escalations`：
  - 收敛时写入 `close_reason`
  - 保持原有 `status: converged` 不变（兼容旧闭环）

### 4. 观测输出补齐

- `internal/sevolver/sevolver.go`：
  - 结果结构新增：
    - `ResolvedIssueReasons`
    - `ResolvedReasonCounters`
  - CLI 输出 `deep_analysis_resolved` 增加关闭分类汇总
- `internal/sevolver/reporter.go`：
  - 日报增加“收敛关闭分类”总览
  - 每个已关闭 issue 展示分类标签（中文）

## 验证

- `cd src && go test -count=1 ./internal/sevolver`
- `cd src && go test ./...`
- 结果：全部通过

## 兼容性说明

- 未改变 `resolved` / `converged` 的主状态语义，仅新增子分类字段。
- 历史无 `close_reason` 数据的条目可继续读取，按 `closed` 兜底。

## 后续优化建议

1. 在 `ccclaw status --json` 增加 sevolver 关闭分类聚合字段，便于外部看板直接消费。
2. 将 `close_reason` 规范沉淀到 deep-analysis issue 模板，减少自由文本歧义。
3. 为 `close_reason` 增加仓库级口径统计（日/周/月），用于评估“真实修复 vs 归档类关闭”的长期趋势。
