# RFC: ccclaw 架构评估、竞品对比与优化路线

> 日期: 2026-03-09
> 关联 Issue: #1, #3, #5
> 状态: 待 sysNOTA 审阅

---

## 一、当前项目状态总结

### 已完成里程碑

| 阶段 | Issue | 状态 | 关键交付 |
|------|-------|------|----------|
| Phase A MVP | #1 | 完成 | ingest/run/status CLI, 幂等键, 风险门禁, systemd timer |
| Phase B TodoTask | #2 | 已关闭 | TodoWrite 五步模板, explore/plan/code 三阶段 |
| Phase 0 架构重构 | #3 | 进行中 | 单 CLI, SQLite, TOML+.env, 目录重整, 权限门禁 |
| Phase 0.1 安装收口 | #4 | 已关闭 | install.sh 重构, rtk 接入, CLAUDE.md 分层, doctor 增强 |
| Phase 0.2 | #5 | 待决策 | Claude 安装授权, 多 target, 开源协作策略 |

### 当前架构拓扑

> ```text
>  GitHub Issue (control_repo)
>        |
>    [systemd timer: 5min]
>        |
>    ccclaw ingest ──► SQLite (tasks + task_events)
>        |
>    [systemd timer: 10min]
>        |
>    ccclaw run ──► rtk proxy claude -p ──► 执行
>        |                                    |
>    ccclaw status                       Reporter
>        |                              (回写 Issue comment)
>    ccclaw doctor
>        |
>    ccclaw config
>
>  程序目录: ~/.ccclaw
>  本体仓库: /opt/ccclaw (kb/记忆)
> ```

### 代码结构

> ```text
> src/
> ├── cmd/ccclaw/main.go          # 单 CLI (cobra), 5 子命令
> ├── internal/
> │   ├── app/runtime.go          # 编排: ingest→sync→run→report
> │   ├── core/task.go            # 状态机: NEW→RUNNING→DONE/FAILED/DEAD
> │   ├── config/config.go        # TOML + .env(0600) 加载
> │   ├── executor/executor.go    # claude -p 包装
> │   ├── memory/index.go         # kb 关键词匹配
> │   └── adapters/
> │       ├── github/client.go    # gh api 封装
> │       ├── storage/sqlite.go   # SQLite 持久化
> │       └── reporter/reporter.go
> └── dist/
>     ├── install.sh              # 555 行, 探查/模拟/非交互
>     ├── upgrade.sh
>     └── ops/systemd/            # 4 个 user-level unit
> ```

---

## 二、竞品对比核心结论

### 与 ccclaw 最相关的 6 个竞品

| 竞品 | 核心相似点 | 核心差异 | ccclaw 可借鉴 |
|------|-----------|---------|--------------|
| **OpenAI Symphony** | daemon+issue tracker+隔离执行+状态持久化 | Elixir/Linear/PostgreSQL | WORKFLOW.md 版本化 |
| **GitHub Copilot Agent** | Issue 驱动+异步+多模型 | 闭源商业, GitHub Actions | 提交前自审查机制 |
| **Claude Agent SDK** | Claude Code 可编程化 | 库而非系统 | Hooks 管控, MCP server |
| **Open SWE** | 异步+GitHub+多代理 | LangGraph 依赖 | Planner-Reviewer 分层 |
| **OpenClaw** | Skills 生态+记忆 | 消息通道驱动, 非编码专用 | Skills 可扩展机制 |
| **SWE-agent** | Issue→修复→PR 管线 | 一次性 CLI, 无记忆 | ACI 接口优化 |

### ccclaw 的独特定位

> ```text
>  维度对比:
>
>  部署重量    轻 ◄━━━━[ccclaw]━━━━━━━━━━━━━━━━━━► 重
>                systemd       Docker/Elixir    K8s/Cloud VM
>
>  任务入口    GitHub Issue ◄━━[ccclaw]
>                              Linear(Symphony)
>                              PRD(Taskmaster)
>                              消息(OpenClaw)
>
>  记忆深度    无 ━━━━━━━━━━━━━━━━━━━━━━━[ccclaw]━━► 深
>              SWE-agent/OpenHands     kb+skills+journal
>
>  自主程度    人工 ━━━━━━━━━━[ccclaw]━━━━━━━━━━━━━► 全自主
>               Aider/Cline        Devin/Copilot Agent
> ```

**ccclaw 的差异化**: "GitHub Issue 驱动 + Claude Code 执行 + systemd 调度 + kb 记忆" 这个组合在开源生态中没有完全匹配的竞品。最近的是 Symphony (Linear+Elixir), 但 ccclaw 对开源项目和 Linux 服务器更友好。

---

## 三、当前架构的关键评估

### 正确的设计决策 (保持不变)

1. **SQLite 状态后端** — 比 JSON 文件可靠, 比 PostgreSQL 轻量
2. **TOML+固定.env 配置** — 职责清晰, 安全边界明确
3. **单一 CLI 入口** — cobra 子命令, 可维护性好
4. **systemd timer 调度** — 最 Linux 原生, 零外部依赖
5. **GitHub 动态权限判断** — 比硬编码 trusted_actors 更灵活
6. **程序目录/本体仓库分离** — 升级与记忆解耦

### 需要关注的结构性问题

#### 问题 1: 执行层仍是"黑盒 API 调用"

当前 `executor.go` 把 Claude Code 当成文本输出工具:

```go
cmd := exec.CommandContext(ctx, e.command[0], args...)
```

**影响**: 丢失了 Claude Code 的工具调用能力 (读写文件、执行命令、gh CLI)
**建议**: phase0.2+ 考虑切换到 Claude Agent SDK, 让执行体有真正的 agentic 能力

#### 问题 2: 多 target 路由缺失

`config.toml` 已有 `[[targets]]` 模型, 但 `runtime.syncIssue()` 仍把 `TargetRepo` 固定为 `control_repo`。

**影响**: 不能为不同仓库分配不同任务
**建议**: Issue #5 中已提出路由方案, 待决策

#### 问题 3: 记忆注入粗糙

`memory/index.go` 用关键词匹配, 无语义理解:

```go
// 只做大小写不敏感的字符串包含
if strings.Contains(lower, kw) { score++ }
```

**影响**: phase0 够用, 但长期会导致注入噪声
**建议**: phase1 考虑嵌入向量检索或 Claude 内置的 CLAUDE.md 加载机制

#### 问题 4: 缺少 Planner 层

当前架构是 `ingest → run → execute`, 没有独立的计划层。

> ```text
>  当前:  Issue → run → claude -p (一次性执行)
>
>  建议:  Issue → planner → 拆分子任务 → executor (分步执行)
>                  ↑                          ↓
>                  └── reviewer ◄── 结果审查 ──┘
> ```

**影响**: 复杂任务容易上下文漂移
**建议**: Phase B 的 TodoWrite 模板是软约束; 长期应引入 Planner-Reviewer 模式 (参考 Open SWE)

---

## 四、Issue #5 (phase0.2) 待决策点评估

Issue #5 评论中 CCoder 已提出 Q1-Q6, 以下为 CCoder 评估和建议:

### Q1. Claude 首次安装默认行为

**CCoder 建议**: 交互模式提示后安装; 非交互需 `--install-claude` 显式指定

**理由**: 自动安装 npm 全局包有安全风险, 应给用户选择权

### Q2. 授权模式

**CCoder 建议**: A(auth login) + B(setup-token) + C(rtk 代理) 三路径并存

**理由**: 开源场景用户环境多样, 不应锁死单一路径

### Q3. 多 target 路由

**CCoder 建议**: Issue body 显式 `target_repo:` + 缺省走 `default_target`

**理由**: 最小可用, label 不承担主路由避免歧义

### Q4. 开源默认权限门槛

**CCoder 建议**: 仅 admin 自动执行, 仅 admin 的 `/ccclaw approve` 生效

**理由**: 安全第一, `write/maintain` 不应默认自动执行开源仓库任务

### Q5. Go/sudo/代理变量

**CCoder 建议**: Go 纳入 install/doctor; sudo 只探查不改 sudoers; `.env` 新增 `ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN`

### Q6. 本体仓库定位

**CCoder 建议**: phase0.2 不做 remote 绑定, 只保证升级不覆盖

---

## 五、优化建议路线 (分期)

### 近期: phase0.2 (Issue #5 范围)

1. Claude 安装/授权状态机 (4 态: missing_cli / installed_unauthed / installed_authed_official / installed_proxy_mode)
2. 多 target 路由最小版 (显式 target_repo + default_target)
3. 开源默认策略引擎 (admin-only auto-execute)
4. doctor/upgrade 联动检查
5. Go/sudo/.env 变量补全

### 中期: phase1

1. 引入 Claude Agent SDK 替代 `exec.Command` 包装
2. Provider 抽象 (claude-code / openai-compatible)
3. Token/cost budget 三层限制 (任务/阶段/日)
4. 记忆注入升级 (从关键词匹配到语义检索)
5. Planner-Reviewer 模式引入

### 远期: phase2+

1. Matrix 协作通道 (全局 ops room + 可选任务子 room)
2. Worktree 并行执行
3. SKILL 自学习闭环 (执行→发现→写入→审核)
4. 周报自动汇总
5. 国产模型分级路由 (Qwen / DeepSeek / GLM)

---

## 六、关键借鉴建议

从竞品中提取 5 个可直接落地的模式:

1. **Symphony 的 WORKFLOW.md** — 将代理行为规范放入仓库, 随分支同步。ccclaw 已有 CLAUDE.md 分层, 可进一步增强
2. **Copilot 的提交前自审查** — executor 完成后, 在回写前增加一轮自动 code review
3. **Open SWE 的 Planner-Reviewer** — phase1 引入, 解决复杂任务上下文漂移
4. **OpenClaw 的 Skills 注册表** — 当前 L1/L2 目录结构已对齐, 补充 metadata 标准即可
5. **Claude Agent SDK 的 Hooks** — phase1 用 Hooks 替代当前的硬编码管控逻辑

---

## 七、完整竞品报告

详见 `docs/reports/260309_NA_autonomous_agent_orchestration_landscape.md`, 覆盖 20+ 项目。
