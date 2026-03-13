# Go stream-json daemon 与 tmux 包装方案对比调研（ccclaw）

> 日期：2026-03-13  
> 调研模型：GPT-5 Codex（OpenAI）  
> 关联 Issue：[#45](https://github.com/41490/ccclaw/issues/45)  
> 范围：仅调研，不修改运行代码

## 1. 正确理解问题与需求

这次不是简单比较“两个工具哪个好”，而是要回答：

1. `ccclaw` 当前 tmux 包装链路与目标需求（简洁、合理、稳定、有效）之间的真实差距在哪里；
2. Go `stream-json` daemon 方案能否用更少组件、更直接的数据流覆盖当前需求；
3. 应该保留什么、替换什么、如何分阶段迁移并可回滚。

目标是给出可实施的选型建议，而不是只做理念讨论。

## 2. 当前 tmux 包装方案（基于仓库现状）

### 2.1 已落地机制

从当前代码可确认：

- 执行器在启用 tmux 时走 `launchTMux()`，通过 `tmux new-session -d -s ... -c ... <command>` 发射任务。
- Claude 命令形态是单次 `-p`，并使用 `--output-format json`，不是双向 `stream-json` 常驻会话。
- stdout/stderr 分别进入 `stdout 暂存` / `diag` / `log`，之后由收口阶段再物化为最终 `result.json`。
- `patrol` 以周期轮询方式检查会话与槽位（当前默认 `*:0/2`），`ingest` 默认 `*:0/4`。
- 上下文压力重启依赖 `claude hook 状态 + transcript 解析`，超过阈值后再触发重启。

对应代码位置（仅列关键）：

- `src/internal/executor/executor.go`
- `src/internal/tmux/manager.go`
- `src/internal/app/runtime_cycle.go`
- `src/ops/systemd/ccclaw-ingest.timer`
- `src/ops/systemd/ccclaw-patrol.timer`

### 2.2 当前方案优势

- tmux 原生支持 detach/attach，人工接管与现场排障便利。
- 已和 `ccclaw` 的按仓槽位模型（每仓一个 Claude）融合。
- 出问题时可以直接看 pane 现场、`capture-pane` 与 `diag` 文件。

### 2.3 当前方案主要成本

- 状态与结果收口链路长：`tmux pane -> stdout/diag/staging -> finalize -> state`。
- 监控偏轮询，天然有“发现延迟”和“跨轮次收口复杂度”。
- 上下文重启依赖外部 hook/transcript 旁路文件，不是 Claude 输出流原位决策。
- 组件面较宽：tmux server、slot、patrol、artifact materialize、hook state、多阶段 finalize。

## 3. Go stream-json daemon 方案（外部事实基线）

基于官方文档与一手手册：

- Claude Code 文档明确 `--output-format stream-json` 可输出逐行 JSON 事件，配合 `--include-partial-messages` 可拿到增量消息。
- Claude Code 文档明确 `--continue` / `--resume` 支持会话延续。
- tmux 手册明确其本质是 client/server + socket 的终端复用器，并提供 `CONTROL MODE`、`capture-pane`、`pipe-pane`、`remain-on-exit`、`respawn-pane` 等能力。
- Go 标准库 `os/exec.CommandContext` 和 `encoding/json.Decoder` 可以直接支撑“子进程生命周期 + 流式 JSON 解码”。
- systemd `Restart=on-failure` / `RestartSec=` 可直接兜底 daemon 进程级自愈。

## 4. Go daemon vs tmux 主要差异

| 维度 | 当前 tmux 包装 | Go stream-json daemon |
|---|---|---|
| 进程模型 | tmux 承载 shell，会话由外部轮询 | 业务进程内直接管 Claude 子进程 |
| I/O 主通道 | pane 输出 + 文件中转 | stdin/stdout NDJSON 直连 |
| 结果收口 | 依赖 staging/diag/result 物化 | 事件即状态，天然结构化 |
| 上下文阈值处置 | hook + transcript 侧路计算 | 在流事件上实时判定与处置 |
| 故障检测时延 | 受 patrol 周期影响 | 事件驱动，近实时 |
| 运维复杂度 | tmux + patrol + artifact + hook | daemon + systemd（组件更少） |
| 人工接管 | 强（attach pane） | 弱于 tmux（需补调试入口） |
| 对现有改造量 | 低（已在线） | 中高（需迁移执行主链路） |

## 5. 推荐结论

### 5.1 选型建议

推荐将 **Go stream-json daemon** 作为主执行内核；tmux 从“主执行载体”降级为“可选人工排障工具”。

一句话：

- **主链路要简化为“结构化事件流驱动状态机”**；
- **tmux 保留在应急调试层，不再承担核心状态推进责任**。

### 5.2 为什么更“简洁合理稳定有效”

- 简洁：减少 `tmux + 文件中转 + 跨轮次收口` 这条长链。
- 合理：Claude 已提供结构化流输出，直接消费比二次拼接更贴合问题域。
- 稳定：事件驱动比轮询更少竞态窗口；systemd 可做标准进程守护。
- 有效：上下文阈值、错误事件、会话信息可原位判定，降低“事后推断”误差。

## 6. 分阶段实施计划（仅方案，不改码）

1. `Phase 0`：定义统一事件模型
- 抽象最小事件集：`started/progress/usage/result/error/restarted`。
- 明确从 NDJSON 到 `state.json + slot` 的映射规则。

2. `Phase 1`：影子模式（不接管主流程）
- 保持 tmux 主链路不变。
- 新增 daemon 只做“并行观测 + 事件落盘”，验证可观测一致性。

3. `Phase 2`：单仓灰度切流
- 选择一个 `target_repo` 用 daemon 主跑。
- 其他仓保持 tmux，验证多仓并行与回退路径。

4. `Phase 3`：双轨开关与回滚
- 配置开关：`executor_mode = daemon|tmux`。
- 任意仓异常可秒级回退到 tmux。

5. `Phase 4`：默认切换与职责收缩
- daemon 成为默认执行器。
- tmux 保留 `debug attach` 能力，退出核心收口链路。

## 7. 风险与前置约束

- 需要先定义“人工接管替代路径”（如 `ccclaw debug attach` 到专用诊断 shell）。
- 需要把当前 `hook/transcript` 侧路逻辑收敛到单一事件来源，避免双事实源。
- 需要为 `FINALIZING` 与重试退避保留现有语义，不因执行器替换而回退能力。

## 8. 最终建议

在当前 `ccclaw` 架构下：

- **短期**：不建议继续放大 tmux 在主链路中的职责。
- **中期**：建议推进 Go stream-json daemon 主执行，tmux 降级为调试工具。
- **执行原则**：先影子、再灰度、可回滚、按仓切换。

---

# refer. 参考

## A. DuckDuckGo 检索入口（核验链路）

> 说明：本次采用 DuckDuckGo `!ducky` 检索方式；会话后段 DuckDuckGo 出现 `418` 反爬限制，以下链接为本轮已记录的检索入口。

- Claude headless：
  - <https://duckduckgo.com/?q=%21ducky+code.claude.com+docs+headless+stream-json>
- Claude memory：
  - <https://duckduckgo.com/?q=%21ducky+code.claude.com+docs+memory+CLAUDE.md>
- tmux README：
  - <https://duckduckgo.com/?q=%21ducky+github+tmux+tmux+README+terminal+multiplexer>
- tmux man：
  - <https://duckduckgo.com/?q=%21ducky+tmux+man+page+control+mode>
- Go os/exec：
  - <https://duckduckgo.com/?q=%21ducky+go.dev+os/exec+CommandContext>
- Go encoding/json：
  - <https://duckduckgo.com/?q=%21ducky+go.dev+encoding/json+Decoder>
- systemd service：
  - <https://duckduckgo.com/?q=%21ducky+systemd.service+Restart%3Don-failure+docs>

## B. 外部一手资料（按主题）

### Claude Code
- Headless / programmatic usage：<https://code.claude.com/docs/en/headless>
- Memory / CLAUDE.md：<https://code.claude.com/docs/en/memory>

### tmux
- 官方仓库 README：<https://github.com/tmux/tmux>
- tmux(1) 手册：<https://www.man7.org/linux/man-pages/man1/tmux.1.html>

### Go 标准库
- `os/exec`：<https://pkg.go.dev/os/exec>
- `encoding/json`：<https://pkg.go.dev/encoding/json>

### systemd
- `systemd.service`：<https://www.freedesktop.org/software/systemd/man/systemd.service.html>

## C. 仓库内现状依据

- `src/internal/executor/executor.go`
- `src/internal/tmux/manager.go`
- `src/internal/app/runtime_cycle.go`
- `src/internal/app/runtime.go`
- `src/ops/systemd/ccclaw-ingest.timer`
- `src/ops/systemd/ccclaw-patrol.timer`
- `src/ops/config/config.example.toml`
