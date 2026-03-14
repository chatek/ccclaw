# Issue #38 深度分析关闭结果细分 实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 deep-analysis issue 的关闭结果从单一 `resolved/converged` 细分为 `fixed / duplicate / external / archived / unreproducible`，通过 GitHub label + `state_reason` 自动识别，零人工约定。

**Architecture:** 新增 `EscalationCloseReason`（GapSignal）与 `CloseReason`（skillGapEscalation）字段，在 `ResolveClosedDeepAnalysisEscalations` 读取已关闭 issue 时同步提取原因，向下传播到 gap 台账与 skill frontmatter 并写入日报。现有 `resolved / converged` 状态语义不变，只是附加了子分类字段，向后完全兼容。

**Tech Stack:** Go 1.21+, gopkg.in/yaml.v3, GitHub REST API (`state_reason` + labels), `go test ./...`

---

## 文件变更映射

| 操作 | 文件 | 职责 |
|------|------|------|
| Modify | `src/internal/adapters/github/client.go` | `Issue` 结构加 `StateReason` 字段 |
| Modify | `src/internal/sevolver/scanner.go` | `GapSignal` 加 `EscalationCloseReason` |
| Modify | `src/internal/sevolver/escalation_backfill.go` | 提取关闭原因常量 + 函数；传播到 gap 更新 |
| Modify | `src/internal/sevolver/gap_recorder.go` | 渲染/解析 `escalation_close_reason` |
| Modify | `src/internal/sevolver/skill_updater.go` | `skillGapEscalation` 加 `CloseReason`；传入收敛时回写 |
| Modify | `src/internal/sevolver/reporter.go` | 日报 `## 收敛回写` 区块展示关闭原因分布 |
| Modify | `src/internal/sevolver/deep_trigger_test.go` | `fakeDeepAnalysisClient.GetIssue` 支持 Labels |
| Modify | `src/internal/sevolver/escalation_backfill_test.go` | 新增 close reason 测试 |

---

## Chunk 1: 数据结构与原因提取（无 IO）

### Task 1: `Issue.StateReason` + 关闭原因提取函数

**Files:**
- Modify: `src/internal/adapters/github/client.go`
- Modify: `src/internal/sevolver/escalation_backfill.go`
- Modify: `src/internal/sevolver/deep_trigger_test.go`
- Test: `src/internal/sevolver/escalation_backfill_test.go`

- [ ] **Step 1.1: 在 `Issue` 结构体加 `StateReason` 字段**

在 `src/internal/adapters/github/client.go` 的 `Issue` struct 中，`State` 字段后面加：

```go
StateReason string    `json:"state_reason,omitempty"`
```

完整 struct（只改这一行）：
```go
type Issue struct {
    Repo        string    `json:"-"`
    Number      int       `json:"number"`
    Title       string    `json:"title"`
    Body        string    `json:"body"`
    State       string    `json:"state"`
    StateReason string    `json:"state_reason,omitempty"`
    Labels      []Label   `json:"labels"`
    User        User      `json:"user"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
    PullReq     any       `json:"pull_request,omitempty"`
}
```

- [ ] **Step 1.2: 写测试：关闭原因提取函数**

在 `src/internal/sevolver/escalation_backfill_test.go` 末尾追加：

```go
func TestExtractCloseReasonFromLabelsAndStateReason(t *testing.T) {
    cases := []struct {
        name        string
        labels      []ghadapter.Label
        stateReason string
        want        string
    }{
        {"default fixed",        nil,                                   "",            "fixed"},
        {"fixed completed",      nil,                                   "completed",   "fixed"},
        {"not_planned archived", nil,                                   "not_planned", "archived"},
        {"label duplicate",      []ghadapter.Label{{Name: "duplicate"}}, "",           "duplicate"},
        {"label wontfix",        []ghadapter.Label{{Name: "wontfix"}},   "",           "archived"},
        {"label invalid",        []ghadapter.Label{{Name: "invalid"}},   "",           "unreproducible"},
        {"label external",       []ghadapter.Label{{Name: "external"}},  "",           "external"},
        // label takes priority over state_reason
        {"label beats not_planned", []ghadapter.Label{{Name: "invalid"}}, "not_planned", "unreproducible"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            issue := ghadapter.Issue{
                Labels:      tc.labels,
                StateReason: tc.stateReason,
            }
            got := extractCloseReason(issue)
            if got != tc.want {
                t.Fatalf("extractCloseReason() = %q, want %q", got, tc.want)
            }
        })
    }
}
```

- [ ] **Step 1.3: 运行测试确认失败**

```bash
cd /opt/src/ccclaw/src && go test ./internal/sevolver/ -run TestExtractCloseReason -v
```

期望：`FAIL — extractCloseReason undefined`

- [ ] **Step 1.4: 实现 `extractCloseReason` + 常量**

在 `src/internal/sevolver/escalation_backfill.go` 开头常量区加：

```go
const (
    gapEscalationStatusEscalated = "escalated"
    gapEscalationStatusResolved  = "resolved"
    gapEscalationStatusConverged = "converged"

    closeReasonFixed          = "fixed"
    closeReasonDuplicate      = "duplicate"
    closeReasonExternal       = "external"
    closeReasonArchived       = "archived"
    closeReasonUnreproducible = "unreproducible"
)
```

在 `escalation_backfill.go` 末尾加函数：

```go
// extractCloseReason 从已关闭 issue 的 label 与 state_reason 推断关闭原因。
// 优先级：label 关键词 > state_reason > 默认 fixed。
func extractCloseReason(issue ghadapter.Issue) string {
    labelPriority := map[string]string{
        "duplicate": closeReasonDuplicate,
        "wontfix":   closeReasonArchived,
        "invalid":   closeReasonUnreproducible,
        "external":  closeReasonExternal,
    }
    for _, label := range issue.Labels {
        name := strings.ToLower(strings.TrimSpace(label.Name))
        if reason, ok := labelPriority[name]; ok {
            return reason
        }
    }
    if strings.EqualFold(strings.TrimSpace(issue.StateReason), "not_planned") {
        return closeReasonArchived
    }
    return closeReasonFixed
}
```

- [ ] **Step 1.5: 运行测试确认通过**

```bash
cd /opt/src/ccclaw/src && go test ./internal/sevolver/ -run TestExtractCloseReason -v
```

期望：`PASS`

- [ ] **Step 1.6: 运行全量测试确认无回退**

```bash
cd /opt/src/ccclaw/src && go test ./... 2>&1
```

期望：全部 PASS。

- [ ] **Step 1.7: 提交**

```bash
cd /opt/src/ccclaw/src && git add internal/adapters/github/client.go internal/sevolver/escalation_backfill.go internal/sevolver/escalation_backfill_test.go
git commit -m "feat(#38): Issue 结构加 StateReason，新增 extractCloseReason 函数"
```

---

## Chunk 2: 数据模型扩展（GapSignal + skillGapEscalation）

### Task 2: 给 GapSignal 和 skillGapEscalation 加 CloseReason 字段

**Files:**
- Modify: `src/internal/sevolver/scanner.go`
- Modify: `src/internal/sevolver/skill_updater.go`
- Test: `src/internal/sevolver/gap_recorder_test.go`（如存在）或 `escalation_backfill_test.go`

- [ ] **Step 2.1: `GapSignal` 加 `EscalationCloseReason`**

在 `src/internal/sevolver/scanner.go` 的 `GapSignal` struct 中，`EscalationUpdatedAt` 后面加：

```go
type GapSignal struct {
    ID                    string
    Date                  time.Time
    Keyword               string
    Context               string
    Source                string
    RelatedSkills         []string
    EscalationStatus      string
    EscalationFingerprint string
    EscalationIssueNumber int
    EscalationIssueURL    string
    EscalationUpdatedAt   string
    EscalationCloseReason string   // 新增：关闭原因子分类
}
```

- [ ] **Step 2.2: `skillGapEscalation` 加 `CloseReason`**

在 `src/internal/sevolver/skill_updater.go` 的 `skillGapEscalation` struct 中加：

```go
type skillGapEscalation struct {
    Fingerprint string   `yaml:"fingerprint"`
    Status      string   `yaml:"status,omitempty"`
    CloseReason string   `yaml:"close_reason,omitempty"`   // 新增
    IssueNumber int      `yaml:"issue_number,omitempty"`
    IssueURL    string   `yaml:"issue_url,omitempty"`
    UpdatedAt   string   `yaml:"updated_at,omitempty"`
    GapIDs      []string `yaml:"gap_ids,omitempty"`
}
```

- [ ] **Step 2.3: 运行全量测试确认结构变更无破坏**

```bash
cd /opt/src/ccclaw/src && go test ./... 2>&1
```

期望：全部 PASS（字段为零值，不影响现有序列化）。

- [ ] **Step 2.4: 提交**

```bash
cd /opt/src/ccclaw/src && git add internal/sevolver/scanner.go internal/sevolver/skill_updater.go
git commit -m "feat(#38): GapSignal 加 EscalationCloseReason，skillGapEscalation 加 CloseReason"
```

---

## Chunk 3: 持久化（gap-signals.md 渲染/解析 + skill frontmatter）

### Task 3: gap-signals.md 渲染与解析支持 close reason

**Files:**
- Modify: `src/internal/sevolver/gap_recorder.go`
- Test: `src/internal/sevolver/gap_recorder_test.go`（如不存在就在 `escalation_backfill_test.go` 追加）

- [ ] **Step 3.1: 写测试：渲染 close reason**

在 `src/internal/sevolver/escalation_backfill_test.go` 末尾追加：

```go
func TestGapSignalRenderAndParseCloseReason(t *testing.T) {
    kbDir := t.TempDir()
    now := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)
    gaps := []GapSignal{{
        ID:                    "gap-20260313-dup",
        Date:                  now,
        Keyword:               "失败",
        Source:                "2026/03/2026.03.13.md",
        Context:               "重复出现",
        EscalationStatus:      gapEscalationStatusResolved,
        EscalationFingerprint: "sg-dup",
        EscalationIssueNumber: 99,
        EscalationCloseReason: closeReasonDuplicate,
        EscalationUpdatedAt:   "2026-03-13",
    }}

    if _, err := SaveGapSignals(kbDir, gaps, now); err != nil {
        t.Fatalf("SaveGapSignals failed: %v", err)
    }
    loaded, err := LoadGapSignals(kbDir, now)
    if err != nil {
        t.Fatalf("LoadGapSignals failed: %v", err)
    }
    if len(loaded) != 1 {
        t.Fatalf("expected 1 gap, got %d", len(loaded))
    }
    if loaded[0].EscalationCloseReason != closeReasonDuplicate {
        t.Fatalf("EscalationCloseReason: got %q, want %q", loaded[0].EscalationCloseReason, closeReasonDuplicate)
    }
}
```

- [ ] **Step 3.2: 运行确认失败**

```bash
cd /opt/src/ccclaw/src && go test ./internal/sevolver/ -run TestGapSignalRenderAndParseCloseReason -v
```

期望：FAIL（字段不持久化，加载后为空）。

- [ ] **Step 3.3: 更新 `renderGapSignalBody`**

在 `src/internal/sevolver/gap_recorder.go` 的 `renderGapSignalBody` 函数，在 `escalation_updated_at` 行后加：

```go
if strings.TrimSpace(item.EscalationUpdatedAt) != "" {
    lines = append(lines, "  escalation_updated_at: "+strings.TrimSpace(item.EscalationUpdatedAt))
}
// 新增：
if strings.TrimSpace(item.EscalationCloseReason) != "" {
    lines = append(lines, "  escalation_close_reason: "+strings.TrimSpace(item.EscalationCloseReason))
}
```

- [ ] **Step 3.4: 更新 `parseGapSignalMetadata`**

在 `src/internal/sevolver/gap_recorder.go` 的 `parseGapSignalMetadata` switch 中加：

```go
case "escalation_close_reason":
    item.EscalationCloseReason = value
```

- [ ] **Step 3.5: 更新 `mergeGapSignal`**

在 `src/internal/sevolver/gap_recorder.go` 的 `mergeGapSignal` 函数末尾（return 前）加：

```go
if strings.TrimSpace(current.EscalationCloseReason) == "" {
    current.EscalationCloseReason = incoming.EscalationCloseReason
}
```

- [ ] **Step 3.6: 运行测试确认通过**

```bash
cd /opt/src/ccclaw/src && go test ./internal/sevolver/ -run TestGapSignalRenderAndParseCloseReason -v
```

期望：PASS。

- [ ] **Step 3.7: 写测试：skill frontmatter 写入 close_reason**

在 `src/internal/sevolver/escalation_backfill_test.go` 末尾追加：

```go
func TestApplySkillGapEscalationResolutionWritesCloseReason(t *testing.T) {
    kbDir := t.TempDir()
    now := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)
    skillPath := filepath.Join(kbDir, "skills", "L1", "demo", "CLAUDE.md")
    if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(skillPath, []byte(`---
name: demo-skill
description: test
keywords: [demo]
status: active
gap_signals:
  - gap-20260313-dup
gap_escalations:
  - fingerprint: sg-dup
    status: escalated
    issue_number: 99
    updated_at: "2026-03-12"
    gap_ids:
      - gap-20260313-dup
---
content
`), 0o644); err != nil {
        t.Fatal(err)
    }

    target := skillGapEscalation{
        Fingerprint: "sg-dup",
        Status:      gapEscalationStatusConverged,
        CloseReason: closeReasonDuplicate,
        IssueNumber: 99,
        GapIDs:      []string{"gap-20260313-dup"},
        UpdatedAt:   now.Format("2006-01-02"),
    }
    changed, err := ApplySkillGapEscalationResolution(skillPath, []skillGapEscalation{target}, now)
    if err != nil {
        t.Fatalf("ApplySkillGapEscalationResolution failed: %v", err)
    }
    if !changed {
        t.Fatal("expected changed=true")
    }
    payload, err := os.ReadFile(skillPath)
    if err != nil {
        t.Fatal(err)
    }
    text := string(payload)
    for _, want := range []string{
        "status: converged",
        "close_reason: duplicate",
    } {
        if !strings.Contains(text, want) {
            t.Fatalf("expected %q in skill frontmatter:\n%s", want, text)
        }
    }
}
```

- [ ] **Step 3.8: 运行确认失败**

```bash
cd /opt/src/ccclaw/src && go test ./internal/sevolver/ -run TestApplySkillGapEscalationResolutionWritesCloseReason -v
```

期望：FAIL（`close_reason` 字段不出现在 frontmatter）。

- [ ] **Step 3.9: 更新 `ApplySkillGapEscalationResolution`**

在 `src/internal/sevolver/skill_updater.go` 的 `ApplySkillGapEscalationResolution` 函数循环内，`Status` 赋值块后面加：

```go
if meta.GapEscalations[idx].Status != gapEscalationStatusConverged {
    meta.GapEscalations[idx].Status = gapEscalationStatusConverged
    changed = true
}
// 新增：
if strings.TrimSpace(target.CloseReason) != "" &&
    meta.GapEscalations[idx].CloseReason != target.CloseReason {
    meta.GapEscalations[idx].CloseReason = target.CloseReason
    changed = true
}
```

- [ ] **Step 3.10: 运行测试确认通过**

```bash
cd /opt/src/ccclaw/src && go test ./internal/sevolver/ -run TestApplySkillGapEscalationResolutionWritesCloseReason -v
```

期望：PASS。

- [ ] **Step 3.11: 运行全量测试**

```bash
cd /opt/src/ccclaw/src && go test ./... 2>&1
```

期望：全部 PASS。

- [ ] **Step 3.12: 提交**

```bash
cd /opt/src/ccclaw/src && git add internal/sevolver/gap_recorder.go internal/sevolver/skill_updater.go internal/sevolver/escalation_backfill_test.go
git commit -m "feat(#38): 持久化 close reason 到 gap-signals.md 与 skill frontmatter"
```

---

## Chunk 4: 传播逻辑（ResolveClosedDeepAnalysisEscalations 携带 close reason）

### Task 4: 在收敛流程中提取并传播关闭原因

**Files:**
- Modify: `src/internal/sevolver/escalation_backfill.go`
- Modify: `src/internal/sevolver/deep_trigger_test.go` — `fakeDeepAnalysisClient` 支持 Labels/StateReason
- Test: `src/internal/sevolver/escalation_backfill_test.go`

- [ ] **Step 4.1: 更新 `fakeDeepAnalysisClient.GetIssue` 支持 Labels**

`fakeDeepAnalysisClient` 定义在 `src/internal/sevolver/deep_trigger_test.go`。`GetIssue` 目前只返回 `State`，需要原样返回 `Labels` 与 `StateReason`（它们已经在 `issuesByID` map 里，只需 `issue` struct 赋值即可，struct 已有字段，无需改 fake 逻辑——只需在测试用例里构造 `Issue` 时带上 Labels 即可）。确认 `GetIssue` 直接返回 map 里的 Issue 拷贝：

```go
func (f *fakeDeepAnalysisClient) GetIssue(number int) (*ghadapter.Issue, error) {
    if issue, ok := f.issuesByID[number]; ok {
        copy := issue
        return &copy, nil
    }
    return &ghadapter.Issue{Repo: "41490/ccclaw", Number: number, State: "open"}, nil
}
```

这个实现已经正确——Labels/StateReason 会随 struct 被拷贝。无需改动。

- [ ] **Step 4.2: 写测试：`ResolveClosedDeepAnalysisEscalations` 传播 close reason**

在 `src/internal/sevolver/escalation_backfill_test.go` 末尾追加：

```go
func TestResolveClosedDeepAnalysisEscalationsPopulatesCloseReason(t *testing.T) {
    cases := []struct {
        name        string
        labels      []ghadapter.Label
        stateReason string
        wantReason  string
    }{
        {"fixed",          nil,                                    "completed",   "fixed"},
        {"duplicate",      []ghadapter.Label{{Name: "duplicate"}}, "",           "duplicate"},
        {"archived wontfix", []ghadapter.Label{{Name: "wontfix"}}, "",           "archived"},
        {"unreproducible", []ghadapter.Label{{Name: "invalid"}},   "",           "unreproducible"},
        {"external",       []ghadapter.Label{{Name: "external"}},  "",           "external"},
        {"not_planned",    nil,                                    "not_planned", "archived"},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            kbDir := t.TempDir()
            now := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)

            backlog := []GapSignal{{
                ID:                    "gap-20260313-x",
                Date:                  now,
                Keyword:               "失败",
                Source:                "2026/03/2026.03.13.md",
                Context:               "测试",
                EscalationStatus:      gapEscalationStatusEscalated,
                EscalationFingerprint: "sg-x",
                EscalationIssueNumber: 55,
            }}
            if _, err := SaveGapSignals(kbDir, backlog, now); err != nil {
                t.Fatalf("SaveGapSignals failed: %v", err)
            }

            _, resolution, err := ResolveClosedDeepAnalysisEscalations(Config{
                KBDir:       kbDir,
                ControlRepo: "41490/ccclaw",
                IssueClient: &fakeDeepAnalysisClient{
                    issuesByID: map[int]ghadapter.Issue{
                        55: {
                            Repo:        "41490/ccclaw",
                            Number:      55,
                            State:       "closed",
                            Labels:      tc.labels,
                            StateReason: tc.stateReason,
                        },
                    },
                },
            }, backlog, now)
            if err != nil {
                t.Fatalf("ResolveClosedDeepAnalysisEscalations failed: %v", err)
            }
            if len(resolution.GapIDs) != 1 {
                t.Fatalf("expected 1 resolved gap, got %#v", resolution.GapIDs)
            }

            loaded, err := LoadGapSignals(kbDir, now)
            if err != nil {
                t.Fatalf("LoadGapSignals failed: %v", err)
            }
            if len(loaded) != 1 {
                t.Fatalf("expected 1 gap in ledger, got %d", len(loaded))
            }
            if loaded[0].EscalationCloseReason != tc.wantReason {
                t.Fatalf("EscalationCloseReason: got %q, want %q", loaded[0].EscalationCloseReason, tc.wantReason)
            }
        })
    }
}
```

- [ ] **Step 4.3: 运行确认失败**

```bash
cd /opt/src/ccclaw/src && go test ./internal/sevolver/ -run TestResolveClosedDeepAnalysisEscalationsPopulatesCloseReason -v
```

期望：FAIL（`EscalationCloseReason` 为空）。

- [ ] **Step 4.4: 在 `ResolveClosedDeepAnalysisEscalations` 中提取并存储 close reason**

在 `src/internal/sevolver/escalation_backfill.go` 的 `ResolveClosedDeepAnalysisEscalations` 函数里：

1. 建立 `issueCloseReasons map[int]string`，在读取 closed issue 时填充：

```go
closedIssues := map[int]struct{}{}
issueCloseReasons := map[int]string{}   // 新增
for _, issueNumber := range issueNumbers {
    issue, err := client.GetIssue(issueNumber)
    if err != nil {
        return updatedBacklog, result, fmt.Errorf("读取 deep-analysis issue #%d 失败: %w", issueNumber, err)
    }
    if issue == nil || !strings.EqualFold(strings.TrimSpace(issue.State), "closed") {
        continue
    }
    closedIssues[issueNumber] = struct{}{}
    issueCloseReasons[issueNumber] = extractCloseReason(*issue)   // 新增
    result.IssueNumbers = append(result.IssueNumbers, issueNumber)
}
```

2. 在填充 `gap.EscalationStatus = gapEscalationStatusResolved` 那一段，同时设置 close reason：

```go
gap.EscalationStatus = gapEscalationStatusResolved
gap.EscalationUpdatedAt = dateFloor(now).Format("2006-01-02")
gap.EscalationCloseReason = issueCloseReasons[gap.EscalationIssueNumber]   // 新增
result.GapIDs = append(result.GapIDs, gap.ID)
```

3. 在构建 `resolutionTargets` 时，把 close reason 带进去：

```go
key := resolutionKey(gap.EscalationFingerprint, gap.EscalationIssueNumber)
target := resolutionTargets[key]
target.Fingerprint = strings.TrimSpace(gap.EscalationFingerprint)
target.IssueNumber = gap.EscalationIssueNumber
target.CloseReason = issueCloseReasons[gap.EscalationIssueNumber]   // 新增
target.GapIDs = append(target.GapIDs, gap.ID)
resolutionTargets[key] = target
```

- [ ] **Step 4.5: 运行测试确认通过**

```bash
cd /opt/src/ccclaw/src && go test ./internal/sevolver/ -run TestResolveClosedDeepAnalysisEscalationsPopulatesCloseReason -v
```

期望：PASS（所有 6 个子用例）。

- [ ] **Step 4.6: 运行全量测试**

```bash
cd /opt/src/ccclaw/src && go test ./... 2>&1
```

期望：全部 PASS。

- [ ] **Step 4.7: 提交**

```bash
cd /opt/src/ccclaw/src && git add internal/sevolver/escalation_backfill.go internal/sevolver/escalation_backfill_test.go
git commit -m "feat(#38): ResolveClosedDeepAnalysisEscalations 传播 close reason 到 gap 台账与 skill"
```

---

## Chunk 5: 日报展示 + 工程报告

### Task 5: 日报 `## 收敛回写` 区块展示 close reason 分布

**Files:**
- Modify: `src/internal/sevolver/reporter.go`
- Modify: `src/internal/sevolver/sevolver.go` — `Result` 加 `ResolvedCloseReasons`
- Test: （集成在 `sevolver_test.go` 或 `reporter` 单元测试中）

- [ ] **Step 5.1: `Result` 加 `ResolvedCloseReasons` 字段**

在 `src/internal/sevolver/sevolver.go` 的 `Result` struct 加：

```go
ResolvedCloseReasons map[string]int   // 新增：close reason → 数量
```

- [ ] **Step 5.2: 在 `Run` 函数中填充 `ResolvedCloseReasons`**

在 `Run` 函数里，处理完 `ResolveClosedDeepAnalysisEscalations` 后：

```go
if resolution != nil {
    result.ResolvedGapIDs = append([]string(nil), resolution.GapIDs...)
    result.ResolvedSkills = append([]string(nil), resolution.SkillPaths...)
    result.ResolvedIssues = append([]int(nil), resolution.IssueNumbers...)
    // 新增：
    if len(resolvedBacklog) > 0 {
        result.ResolvedCloseReasons = collectCloseReasonCounts(resolvedBacklog, resolution.GapIDs)
    }
}
```

在 `escalation_backfill.go` 末尾加辅助函数：

```go
func collectCloseReasonCounts(backlog []GapSignal, resolvedIDs []string) map[string]int {
    if len(resolvedIDs) == 0 {
        return nil
    }
    idSet := map[string]struct{}{}
    for _, id := range resolvedIDs {
        idSet[id] = struct{}{}
    }
    counts := map[string]int{}
    for _, gap := range backlog {
        if _, ok := idSet[gap.ID]; !ok {
            continue
        }
        reason := strings.TrimSpace(gap.EscalationCloseReason)
        if reason == "" {
            reason = closeReasonFixed
        }
        counts[reason]++
    }
    if len(counts) == 0 {
        return nil
    }
    return counts
}
```

- [ ] **Step 5.3: 更新 `reporter.go` 展示 close reason 分布**

在 `src/internal/sevolver/reporter.go` 的 `## 收敛回写` 区块，在已关闭 Issue 行后加：

```go
if len(result.ResolvedCloseReasons) > 0 {
    // 排序输出，保证日报内容确定性
    reasons := make([]string, 0, len(result.ResolvedCloseReasons))
    for r := range result.ResolvedCloseReasons {
        reasons = append(reasons, r)
    }
    sort.Strings(reasons)
    parts := make([]string, 0, len(reasons))
    for _, r := range reasons {
        parts = append(parts, fmt.Sprintf("%s×%d", r, result.ResolvedCloseReasons[r]))
    }
    lines = append(lines, fmt.Sprintf("- 关闭原因分布: %s", strings.Join(parts, " / ")))
}
```

（需要在 `reporter.go` 顶部 import 加 `"sort"`，如果未包含。）

- [ ] **Step 5.4: 运行全量测试**

```bash
cd /opt/src/ccclaw/src && go test ./... 2>&1
```

期望：全部 PASS。

- [ ] **Step 5.5: 提交**

```bash
cd /opt/src/ccclaw/src && git add internal/sevolver/sevolver.go internal/sevolver/escalation_backfill.go internal/sevolver/reporter.go
git commit -m "feat(#38): 日报收敛区块展示 close reason 分布"
```

---

### Task 6: 工程报告

**Files:**
- Create: `docs/reports/260313_38_deep_analysis_close_reason.md`

- [ ] **Step 6.1: 写工程报告**

报告内容模板（创建文件后填入）：

```markdown
# deep-analysis 关闭结果细分工程报告

关联 Issue: #38
日期: 2026-03-13

## 背景

Issue #36 已实现 deep-analysis issue 关闭后自动回写 gap 台账为 `resolved`、skill frontmatter 为 `converged`，
但所有关闭都被压成同一类"已收敛"，缺少结果细分。

## 方案

### 关闭原因分类

| 原因值 | 触发条件 | 中文含义 |
|--------|---------|---------|
| `fixed` | 默认（completed 或无标记） | 已修复 |
| `duplicate` | label: `duplicate` | 重复问题 |
| `archived` | label: `wontfix` 或 `state_reason: not_planned` | 归档不做 |
| `unreproducible` | label: `invalid` | 无法复现 |
| `external` | label: `external` | 外部依赖已解除 |

### 触发来源

评估结论：优先使用 **GitHub label**（语义明确、无需注释约定），次选 **`state_reason`**
（GitHub REST API 原生字段，`not_planned` 对应"不计划修复"）。
Close comment 方式（需解析文本）被排除：增加维护成本且不如 label 可靠。
Issue body frontmatter 方式被排除：与现有 ccclaw marker 格式冲突风险高。

### 向后兼容

- `EscalationStatus == "resolved"` 语义不变，`filterActiveGapSignals` 逻辑不变
- 已有 `resolved` 条目无 `EscalationCloseReason` 字段，读取时默认视为 `fixed`
- `skillGapEscalation.Status == "converged"` 语义不变，`CloseReason` 为可选附加字段

## 落地文件

- `src/internal/adapters/github/client.go` — `Issue.StateReason`
- `src/internal/sevolver/scanner.go` — `GapSignal.EscalationCloseReason`
- `src/internal/sevolver/skill_updater.go` — `skillGapEscalation.CloseReason`
- `src/internal/sevolver/escalation_backfill.go` — `extractCloseReason` + 传播逻辑
- `src/internal/sevolver/gap_recorder.go` — 渲染/解析 `escalation_close_reason`
- `src/internal/sevolver/reporter.go` — 日报关闭原因分布

## 测试覆盖

- `TestExtractCloseReasonFromLabelsAndStateReason` — 8 个 case 覆盖标签与 state_reason 优先级
- `TestGapSignalRenderAndParseCloseReason` — 持久化往返验证
- `TestApplySkillGapEscalationResolutionWritesCloseReason` — skill frontmatter 写入验证
- `TestResolveClosedDeepAnalysisEscalationsPopulatesCloseReason` — 端对端 6 个 case 传播验证

## 结论

所有关闭结果现在自动分类，零人工约定，与现有 `resolved/converged` 闭环完全兼容。
```

- [ ] **Step 6.2: 运行全量测试（最终确认）**

```bash
cd /opt/src/ccclaw/src && go test ./... 2>&1
```

期望：全部 PASS。

- [ ] **Step 6.3: 提交工程报告**

```bash
cd /opt/src/ccclaw && git add docs/reports/260313_38_deep_analysis_close_reason.md docs/plans/260313_38_deep_analysis_close_reason.md
git commit -m "docs(#38): 工程报告与实现计划"
```

---

## 验收检查清单

- [ ] `go test ./...` 全部 PASS
- [ ] `gap-signals.md` 中 `resolved` 条目含 `escalation_close_reason` 字段
- [ ] skill frontmatter `gap_escalations` 中 `converged` 条目含 `close_reason` 字段
- [ ] 日报 `## 收敛回写` 区块展示 `关闭原因分布:` 行
- [ ] 原有测试无回退（`TestResolveClosedDeepAnalysis*`、`TestRun*`）
- [ ] 工程报告写入 `docs/reports/260313_38_deep_analysis_close_reason.md`
