# Issue #27 深化 Issue 驱动规约调研报告

## 背景

2026-03-11 使用 `gh issue view 27 --repo 41490/ccclaw` 核查 Issue #27，议题包含两条主线：

1. 统一“控制仓库 / 本体仓库 / 任务仓库”的术语，改为“本体仓库 / 知识仓库 / 工作仓库”等更贴近自维护场景的说法。
2. 收紧 Issue 驱动门禁，要求只有带 `ccclaw` 标签的 Issue 才进入巡查，并兼容“本体仓库 = 工作仓库”的自举场景。

本报告用于澄清当前实现现状、与 Issue 目标的差距，以及建议的落地顺序。

## 现状核查

### 1. 标签触发机制已部分存在

配置样例已经提供：

- `src/ops/config/config.example.toml`
  - `github.control_repo = "41490/ccclaw"`
  - `github.issue_label = "ccclaw"`

运行时 `ingest` 也确实只拉取带该标签的 open issues：

- `src/internal/app/runtime.go`
  - `Ingest()` 调用 `rt.gh.ListOpenIssues(rt.cfg.GitHub.IssueLabel, rt.cfg.GitHub.Limit)`

GitHub 适配层 `ListOpenIssues()` 会把标签直接拼入 GitHub API 查询参数：

- `src/internal/adapters/github/client.go`
  - `repos/<repo>/issues?state=open&per_page=<n>&labels=<label>`

因此，“没有 `ccclaw` 标签就不进入 ingest” 这一点已经存在。

### 2. 审批门禁已实现为“发起者权限 + 评论批复覆盖”

当前实现已经满足以下规则：

- `maintain` 及以上发起者默认可执行
- 受信任成员评论 `/ccclaw <批准词>` 可放行
- 受信任成员评论 `/ccclaw <否决词>` 可撤回放行
- 审批人权限通过 GitHub 实时查询

关键位置：

- `src/internal/app/runtime.go`
  - `syncIssue()` 中先判定发起者权限，再读取最新评论中的审批命令
- `src/internal/adapters/github/client.go`
  - `FindApproval()` 倒序扫描评论，取最后一个受信任批复
  - `permissionOf()` 通过 GitHub collaborators permission API 动态取权限

这与 AGENTS 中的执行门禁约束基本一致。

### 3. 当前仍是“单控制面仓库”模型，不是“多仓库巡查”模型

当前架构里只有一个 GitHub 控制入口：

- `github.control_repo`

`ccclaw` 只会轮询这个仓库的 Issue，然后再通过以下方式路由到实际执行仓库：

- Issue body 中的 `target_repo: owner/repo`
- `default_target`

关键位置：

- `src/internal/app/runtime.go`
  - `resolveTargetRepo()` 仅做“从控制仓库 Issue 路由到某个 target”的解析

这意味着：

- 当前支持“控制仓库 = 工作仓库”的单仓库场景
- 当前不支持“对所有工作仓库逐个巡查带标签的 Issue”
- 当前也不支持“多个仓库都可直接成为任务入口，而不经 control_repo 汇总”

### 4. 术语现状与 Issue #27 提议不一致

README 仍以“控制仓库 / 本体仓库 / 任务仓库”为主：

- `README.md`
  - “控制仓库负责接任务”
  - “本体仓库负责存记忆”
  - “任务仓库负责干活”
- `README_en.md`
  - `control repository`
  - `home repository`
  - `task repository`

因此术语调整不是补一句话，而是需要系统性修订中英文文档、配置说明、release 模板和安装交互文案。

## 与 Issue #27 的真实差距

### 差距一：标签门禁只覆盖 ingest，未覆盖全生命周期

虽然 `ingest` 只会拉带标签的 Issue，但 `run` 阶段会再次直接 `GetIssue()`，随后 `syncIssue()` 并不会因为标签已被移除而阻断。

这会导致一个语义缺口：

- Issue 初始带 `ccclaw` 标签，被成功入队
- 后续人工移除了 `ccclaw` 标签
- 任务仍可能继续执行，因为当前代码没有把“标签丢失”视为阻塞条件

如果要严格满足“没有 `ccclaw` 标签，巡查器就忽略”，就必须把“标签存在性”提升为任务状态机的一部分，而不是仅作为列表筛选条件。

### 差距二：否决后关闭 Issue 目前没有实现

Issue #27 提议：

- 若批复词是否定，则放弃，并关闭 Issue

当前实现只有：

- 任务阻塞
- 回帖说明阻塞原因

并没有：

- 自动关闭 Issue
- 自动追加统一关闭说明
- 后续是否允许重新贴标签、重新批准再开启的状态机规则

这一点属于行为策略变更，不只是文档更新。

### 差距三：Issue #27 对“无论在哪个仓库中”的表述，需要先拍板含义

该表述至少有两种解释：

1. 仍只看 `github.control_repo`，但允许它与某个工作仓库重叠
2. 开始对多个仓库统一巡查，只要仓库在 target 集合中且 Issue 带 `ccclaw` 标签，就都可成为任务入口

这两种解释的实现复杂度完全不同：

- 解释 1 主要是术语和门禁收紧，属于小中型改造
- 解释 2 属于架构升级，需要引入“入口仓库集合”“跨仓库权限判定”“任务唯一键增加 repo 维度”等系统改造

在未拍板前，不能把它们混成一个需求。

## 建议方案

### 阶段 A：先做术语统一与标签语义收紧

建议先把本轮范围锁定为：

1. 术语重命名
   - 中文：`控制仓库` -> `入口仓库` 或按 Issue 倾向改为 `本体仓库` 需谨慎
   - 中文：旧 `本体仓库` -> `知识仓库`
   - 中文：`任务仓库` -> `工作仓库`
   - 英文同步调整
2. 明确标签门禁
   - 只有带 `ccclaw` 标签的 Issue 才允许入队、继续执行、重新恢复执行
   - 标签被移除时，任务转为 `BLOCKED`
3. 明确否决语义
   - 受信任成员给出否决词后，任务转 `BLOCKED`
   - 是否自动关闭 Issue，作为单独决策项拍板
4. 明确单仓库自举
   - 允许“入口仓库 = 工作仓库”
   - 不引入多仓库统一巡查

这一阶段可以在不推翻当前架构的前提下完成，且能满足 Issue #27 的大部分动机。

### 阶段 B：如确需“多仓库直接巡查”，单开 RFC/Issue

如果最终目标真的是“所有工作仓库都可以直接贴 `ccclaw` 标签触发”，建议单开后续议题，不和术语调整混在 Issue #27 中。

原因：

1. 任务唯一键当前只有 `issueNumber#body`
   - 若多仓库巡查，必须升级为 `repo#issueNumber#body`
2. 目前所有评论回写、权限判定、Issue URL 都默认来自同一个 `control_repo`
3. 运行态报表、状态库、日志、审计信息也默认只有单入口仓库语义
4. 多仓库巡查还要定义：
   - 是否只扫描 `[[targets]]` 中声明了 `repo` 的仓库
   - 是否允许某仓库仅作为入口、不作为执行目标
   - `target_repo:` 在“入口仓库即工作仓库”场景里是否可以省略

因此建议把它视为下一阶段架构题，而不是与术语替换一起一次性落地。

## 建议落地改动清单

若按阶段 A 执行，建议涉及以下区域：

1. 文档与样例
   - `README.md`
   - `README_en.md`
   - `src/ops/examples/release-notes-template.md`
   - `src/ops/examples/install-flow.md`
   - `src/ops/config/config.example.toml`
2. 运行时门禁
   - `src/internal/app/runtime.go`
   - 为 `syncIssue()` 增加“缺少标签即阻塞”的规则
   - 为关闭/忽略语义补状态转换与回帖文案
3. GitHub 适配层
   - `src/internal/adapters/github/client.go`
   - 若决定自动关闭 Issue，需要新增 close/reopen 能力
4. 测试
   - `src/internal/app/runtime_test.go`
   - `src/internal/adapters/github/client_test.go`
   - 增加“标签移除后阻塞”“否决后关闭/不关闭”的覆盖

## 必须先拍板的决策

### 1. 三类仓库最终命名

Issue #27 的正文中，新旧命名写法前后有一次对调，当前存在歧义：

- 新“本体仓库”是否指固定发布仓库 `41490/ccclaw`
- 还是仍指 `/opt/ccclaw` 上保存长期记忆的本机仓库

建议明确三者最终映射表，否则 README 和配置文案无法一次改对。

### 2. “无论在哪个仓库中”是否真的指多仓库巡查

建议在以下两种模式中二选一：

- 模式 A：仍只有一个入口仓库，只是允许它与工作仓库重叠
- 模式 B：所有已配置工作仓库都可直接成为 Issue 入口

建议优先模式 A，风险更可控。

### 3. 否决后是否自动关闭 Issue

建议明确：

- 是否立刻 `close`
- 是否保留 reopen 流程
- 重新贴标签或新批准评论后，是否允许重新进入执行

若未拍板，建议先只阻塞不自动关闭。

### 4. 标签移除后的动作

建议明确以下二选一：

- 方案 A：移除标签即阻塞，等待重新加标签后再恢复
- 方案 B：移除标签即视为取消，直接结束并关闭

建议优先方案 A，保守且更容易回滚。

## 结论

Issue #27 不是纯文案任务，至少包含一个明确的行为改造点：

- 将 `ccclaw` 标签从“ingest 列表筛选条件”升级为“全生命周期执行门禁”

建议本轮先聚焦：

- 术语统一
- 标签全生命周期门禁
- 单仓库自举定义

而把“多仓库直接巡查”拆为后续议题单独拍板。
