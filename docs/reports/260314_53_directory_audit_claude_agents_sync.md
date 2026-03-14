# 260314_53_directory_audit_claude_agents_sync

## 背景

- 来源：Issue #53 `audit: 清查目录状态并同步 CLAUDE/AGENTS 与安装使用手册`
- 目标：清查当前目录职责、同步多层 `CLAUDE.md` / `AGENTS.md`、给出当前最新机能与安装升级及排障流程
- 基线版本：`ccclaw 26.03.13.2331`

## 目录状态清查

### 根目录

- `AGENTS.md`：仓库级执行约束，现已补充当前架构基线
- `CLAUDE.md`：仓库级总规范，现已补充默认 `daemon`、`stream-json`、统一存储口径、`sevolver` 收敛链路
- `README.md` / `README_en.md`：对外说明入口，当前已覆盖 `phase0.5` 基线
- `docs/`：开发过程文档区，不承担运行时配置
- `src/`：源码、发布树、测试与本机 release 资产

### `docs/`

- `docs/reports/`：工程报告主目录，已有安装、调度、`sevolver`、`stream-json`、release 验证记录
- `docs/plans/`：计划与实施设计
- `docs/rfcs/`：架构与方案 RFC
- `docs/assay/` / `docs/superpowers/`：专项实验与补充规划
- 状态判断：职责清晰，已补 `docs/CLAUDE.md`，要求安装/升级/排障说明给出实际命令与判读路径

### `src/`

- `src/cmd/`：CLI 入口与参数装配
- `src/internal/`：运行时实现、执行器、调度、存储、`sevolver`
- `src/ops/`：运维源资产，负责同步到 `src/dist/ops/`
- `src/dist/`：release 安装树源，也是发布包解压后的事实安装入口
- `src/tests/`：安装回归脚本
- `src/.release/`：本机历史打包产物与校验文件，不是当前功能事实源；当前发布源仍以 `src/dist/` 为准

### `src/internal/` 架构状态

- 默认执行模式已切换为 `daemon`
- `tmux` 仍保留，但职责收敛为 debug attach、兼容链路与 patrol 补偿
- 执行输出基线已切换为 `stream-json`
- 运行态事实由 `state store + JSONL/hashchain + repo slot store` 共同承载
- `stats/status` 的 `task_class` 聚合已统一下沉到 `storage`
- `sevolver` 已具备：
  - skill 命中计数与生命周期维护
  - journal gap + task event gap 聚合
  - deep-analysis Issue 升级
  - Issue 关闭后的 `close_reason` 收敛回写

### `src/ops/` 与 `src/dist/`

- 安装脚本 `install.sh` 负责预检、目录落盘、调度部署、手工兜底提示
- 升级脚本 `upgrade.sh` 固定走官方 release 下载与 `SHA256SUMS` 校验
- 受管 timer 当前是五类：
  - `ccclaw-ingest.timer`
  - `ccclaw-patrol.timer`
  - `ccclaw-journal.timer`
  - `ccclaw-archive.timer`
  - `ccclaw-sevolver.timer`
- 状态判断：发布树职责完整，但规范文档此前未完全显式写出 `daemon` 默认语义与新观察面，现已同步

### `src/dist/kb/`

- `kb/` 是本体仓库初始化模板
- `skills/` 已支持 `gap_escalations[].close_reason`、`issue_number`、`issue_url`、`updated_at`、`gap_ids`
- `assay/` 更适合承接 `daemon/tmux`、`stream-json`、`status/stats`、`sevolver` 收敛链路的观测复盘
- 状态判断：知识模板结构可继续用，但需要显式约束 sevolver 自动维护字段，现已补齐

## 本次同步到的规范文件

- `AGENTS.md`
- `CLAUDE.md`
- `docs/CLAUDE.md`
- `src/CLAUDE.md`
- `src/cmd/CLAUDE.md`
- `src/internal/CLAUDE.md`
- `src/internal/adapters/CLAUDE.md`
- `src/ops/CLAUDE.md`
- `src/dist/CLAUDE.md`
- `src/dist/ops/CLAUDE.md`
- `src/dist/kb/CLAUDE.md`
- `src/dist/kb/assay/CLAUDE.md`
- `src/dist/kb/skills/CLAUDE.md`
- `src/dist/kb/skills/L1/CLAUDE.md`
- `src/dist/kb/skills/L2/CLAUDE.md`

## 当前最新机能

### 1. 执行内核与产物

- 默认执行器：`daemon`
- repo 级可覆盖回 `tmux`
- 默认输出协议：`stream-json`
- 运行产物同时保留：
  - `*.stream.jsonl`
  - `*.event.json`
  - 兼容 `*.json`
  - `*.diag.txt`
  - `log/<task>.log`

### 2. 调度与后台自治

- 调度后端支持 `systemd|cron|none`
- `auto` 会优先尝试 `systemd --user`
- `scheduler use/status/doctor/timers/logs` 已形成完整观测闭环

### 3. 运行态可观测性

- `status` 支持人类可读与 `--json`
- `status` 已区分：
  - `tmux 托管任务`
  - `daemon 任务`
  - repo slot 及 `executor_mode`
  - 最近告警
  - token 摘要
  - `sevolver` 自进化链路聚合
- `stats` 支持：
  - 日期范围
  - `--daily`
  - `--rtk-comparison`
  - `sevolver` / `sevolver_deep_analysis` 聚合指标

### 4. sevolver 自进化链路

- 扫描 `journal/` 中的 skill 命中
- 扫描 journal 与 task events 的 gap 信号
- 更新 skill frontmatter：`last_used/use_count/status/gap_signals/gap_escalations`
- 对活跃 gap 触发或复用 deep-analysis Issue
- 在 deep-analysis Issue 关闭后，把 `close_reason` 回写到对应 skill

### 5. 运维与归档

- `journal` 可生成指定日期日报
- `archive` 可导出历史周 JSONL 为 Parquet
- `doctor` 可检查部署、依赖、调度和 Claude 运行前提

## 正确安装流程

### 推荐路径：从官方 release 安装

1. 获取 release 包与 `SHA256SUMS`
2. 校验后解压
3. 执行安装脚本

示例：

```bash
tar -xzf ccclaw_26.03.13.2331_linux_amd64.tar.gz
cd ccclaw_26.03.13.2331_linux_amd64
bash install.sh
```

### 安装期关键选择

- 程序目录：默认 `~/.ccclaw`
- 本体仓库：默认 `/opt/ccclaw`
- 本体仓库模式：`init|remote|local`
- 任务仓库模式：`none|remote|local`
- 调度模式：`auto|systemd|cron|none`

### 安装后立即核验

```bash
~/.ccclaw/bin/ccclaw version
~/.ccclaw/bin/ccclaw doctor
~/.ccclaw/bin/ccclaw config
~/.ccclaw/bin/ccclaw target list
~/.ccclaw/bin/ccclaw scheduler status
```

### 若 user bus 无法直连

安装脚本若不能自动启用 user systemd，需要在登录会话手工执行：

```bash
systemctl --user daemon-reload
systemctl --user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer
```

## 正确升级流程

### 推荐路径：使用安装树自带升级脚本

```bash
bash ~/.ccclaw/upgrade.sh
```

升级特性：

- 固定从官方仓库下载最新 release
- 自动校验 `SHA256SUMS`
- 仅刷新程序树与受管 `kb/**/CLAUDE.md`
- 不覆盖 `/opt/ccclaw` 中的用户记忆内容

### 升级后核验

```bash
~/.ccclaw/bin/ccclaw version
~/.ccclaw/bin/ccclaw doctor
~/.ccclaw/bin/ccclaw scheduler status
~/.ccclaw/bin/ccclaw status --json
```

### 升级后若 timer 未自动恢复

```bash
systemctl --user daemon-reload
systemctl --user restart ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer
```

## 日常使用流程

### 首轮检查

```bash
ccclaw doctor
ccclaw scheduler status
ccclaw target list
```

### 日常观察

```bash
ccclaw status
ccclaw status --json
ccclaw stats --daily --rtk-comparison
ccclaw scheduler logs -f
```

### 手工推动一次完整轮询

```bash
ccclaw ingest
ccclaw run --limit 10
ccclaw patrol
ccclaw sevolver
ccclaw journal --date 2026-03-14
```

### 目标仓治理

```bash
ccclaw target list
ccclaw target add --help
ccclaw target disable --help
```

## 如何通过 ccclaw 指令观察关系与行为并判定处置

建议把排障对象拆成五层关系：

1. 调度层：timer 或 cron 是否按预期触发
2. 任务层：Issue 是否成功入队为 task
3. 执行层：task 当前走 `daemon` 还是 `tmux`
4. 产物层：结果文件、事件流、日志是否完整
5. 收敛层：`journal/stats/status/sevolver` 是否反映到长期记忆与 gap 收敛

### 观察矩阵

#### 1. 先看调度关系

```bash
ccclaw scheduler status
ccclaw scheduler doctor
ccclaw scheduler timers --wide
ccclaw scheduler logs all --lines 100
```

判定与处置：

- 若 `requested` 与 `effective` 不一致：优先执行 `ccclaw scheduler doctor`
- 若 unit 漂移或缺失：执行 `ccclaw scheduler use systemd`
- 若 user systemd 不可用但 cron 可用：切到 `ccclaw scheduler use cron`
- 若仅需停机观察：切到 `ccclaw scheduler use none`

#### 2. 再看 Issue 到 task 的关系

```bash
ccclaw status --json
ccclaw config
ccclaw target list
```

重点观察：

- `tasks.items[].issue_repo`
- `tasks.items[].issue_number`
- `tasks.items[].task_class`
- `tasks.items[].target_repo`
- `tasks.items[].state`

判定与处置：

- 如果 Issue 没入队：先检查标签、审批词、成员权限、`github.control_repo`
- 如果 target 路由不对：检查 `[[targets]]`、默认 target、Issue 所在仓库与 `target list`

#### 3. 再看执行关系

```bash
ccclaw status
ccclaw status --json
ccclaw patrol
```

重点观察：

- `默认模式`
- `tmux 托管任务`
- `daemon 任务`
- slot 的 `executor_mode`

判定与处置：

- 若默认是 `daemon` 且无 tmux 任务：这是正常现状
- 若任务卡在 `RUNNING/FINALIZING`：执行 `ccclaw patrol` 触发补偿
- 若某仓必须人工附着排障：把该 `target.executor_mode` 临时切成 `tmux`

#### 4. 再看产物关系

重点文件：

- `~/.ccclaw/var/results/<task>.stream.jsonl`
- `~/.ccclaw/var/results/<task>.event.json`
- `~/.ccclaw/var/results/<task>.json`
- `~/.ccclaw/var/results/<task>.diag.txt`
- `~/.ccclaw/log/<task>.log`

判定与处置：

- 有 `stream.jsonl` 无 `event.json`：优先怀疑聚合或 finalize 链路
- 有 `diag.txt` 且 `json` 缺失：优先按诊断文本定位 Claude 或外部依赖失败
- 只有 `log`：说明结构化产物未完整落盘，需结合 `scheduler logs` 与本轮命令 `--log-level debug`

#### 5. 最后看收敛关系

```bash
ccclaw stats --daily --rtk-comparison
ccclaw status --json
ccclaw sevolver
ccclaw journal --date 2026-03-14
```

重点观察：

- `status` / `stats` 中的 `task_class` 聚合
- `sevolver` 输出的 `hits/gaps/task_event_gaps`
- deep-analysis 是否 `created` 或 `reused`
- `deep_analysis_resolved` 的 `reasons`

判定与处置：

- 若 `sevolver` backlog 持续上升：说明 skill 缺口或深度分析收敛不足，应回看对应 Issue 与 skill frontmatter
- 若 `close_reason=duplicate` 或 `external_resolved` 增多：优先检查 gap 去重与外部依赖边界，不要盲目继续加 skill
- 若 `success_rate` 下降且 token 成本上升：先看 `stats` 范围视图，再回查最近任务的 stream 产物与日志

## 结论

- 当前项目目录职责整体清晰，实际变化主要集中在“执行默认值、产物契约、聚合口径、sevolver 收敛链路”四个方面
- 本轮已把这些变化同步进根与关键子目录规范文件
- 后续若继续推进 `branch-audit`、更细的 mode 聚合或 deep-analysis 关闭语义扩展，应继续先改存储口径与运行态输出，再同步 `CLAUDE.md`
