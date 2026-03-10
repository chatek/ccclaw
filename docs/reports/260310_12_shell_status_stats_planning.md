# 260310_12_shell_status_stats_planning

## 背景

用户在完成 Issue #11 的 `doctor` 调度诊断细化后，继续提出两项新诉求：

1. 规划把 `ccclaw` 指令受控注入 `~/.bashrc`
2. 追加或扩展 `status` 指令，用于汇报当前 `ccclaw` 监察的 Claude Code 进程健康情况，以及 rtk 历史 token 节省情况

按仓库治理要求：

- 默认先查 Issue 决策，再动代码
- 未拍板的分歧点必须先回 Issue 讨论，禁止脑补式决定

因此本轮先做策划收口，不直接实现。

## 现状核查

### 1. `status` 已经存在

核查 `src/cmd/ccclaw/main.go` 可见：

- 当前已有 `ccclaw status`
- 其语义是“查看任务状态”
- 输出内容来自 `Runtime.Status()`
- 目前只覆盖任务队列，不覆盖运行态健康或历史 token 统计

这意味着“追加 status 指令”并不是新增命令，而是要先拍板：

- 是扩展现有 `status`
- 还是把历史统计拆到单独 `stats`

### 2. shell 注入尚无现成约束

核查 `src/dist/install.sh`、README 与 docs 后可确认：

- 安装器当前只会创建 `~/.local/bin/ccclaw` 链接
- 未写入 `~/.bashrc`
- 也未提供 `--inject-shell` 之类的显式参数
- 仓库中没有现成的 shell 注入边界说明

这属于用户登录环境持久修改，不能在没有 Issue 决议时直接落地。

### 3. 运行态观测与 token 统计已有上位策划

Issue #10 `feat: 轻量集成 Claude CLI JSON 协议 + tmux 巡查器` 已明确规划：

- Claude CLI JSON 输出
- `token_usage` 表
- `ccclaw stats`
- tmux patrol / session 观测

因此用户这次提出的：

- Claude Code 进程健康
- rtk 历史 token 节省

本质上与 #10 高度耦合，不应绕开既有技术路线单独硬做。

## 本次操作

### 1. 新建策划 Issue #12

已使用 `gh issue create` 新建：

- Issue #12
- 标题：`plan: shell 集成与 status/stats 运行态观测策划`
- 链接：<https://github.com/41490/ccclaw/issues/12>

Issue #12 中已整理：

- shell 集成边界
- `status` / `stats` 语义分工
- Claude 会话健康观测来源
- rtk/token 节省统计与 #10 的依赖关系

### 2. 在策划中明确了建议方向

当前建议如下：

1. `~/.bashrc` 注入默认不应静默开启
- 倾向仅在显式参数下启用
- 采用受控标记块
- 必须可回滚

2. `status` 不应承载全部历史统计
- 倾向让 `status` 负责当前快照
- 倾向让 `stats` 负责历史聚合

3. Claude Code 进程健康不应只靠裸 `ps` / `pgrep`
- 倾向依托 #10 的 tmux/session 模型
- 裸进程探测最多只做 fallback

### 3. 保持与 #10 的边界一致

由于 #10 已经把“可观测、可巡查、可度量”列为主轴，本轮没有直接改代码，而是把新诉求整理成上层策划，避免：

- 在 `status` 命令上先写一套临时语义
- 后面再被 #10 的 `stats` / tmux 设计推翻

## 结论

本轮已完成需求收束与决策入口搭建：

- 通过现状核查确认：`status` 已存在，shell 注入尚未拍板
- 通过 Issue #12 把新增需求转为可讨论、可分期的正式策划
- 并明确其与 #10 的依赖关系，避免重复建设

后续建议顺序：

1. 先在 Issue #12 拍板 `status` / `stats` 分工与 shell 注入边界
2. 再结合 Issue #10 的 JSON/tmux/token_usage 落地顺序进入实现
