# 260313_49_phase3_default_daemon_tmux_scope_reduction

## 背景

- 执行来源：Issue #49 `phase3: 默认切换 daemon 与 tmux 职责收缩`
- 前置阶段：
  - #46 已冻结 stream-json 事件契约
  - #47 已完成 tmux + stream-json 影子对账
  - #48 已落地 daemon 执行内核与 repo 级灰度

本阶段目标是把默认执行器切到 `daemon`，并把 `tmux` 明确收敛为调试附着与补偿能力。

## 本轮实现

### 1. 默认执行模式切换为 daemon

- `src/internal/config/config.go`
  - 默认值从 `executor.mode=tmux` 调整为 `executor.mode=daemon`
  - `NormalizePaths` 与 `ExecutorMode()/ExecutorModeForRepo()` 的兜底模式同步为 `daemon`
  - 注释化配置输出补充“默认 daemon，可按仓回切 tmux”说明

- 配置与安装模板同步：
  - `src/ops/config/config.example.toml`
  - `src/dist/ops/config/config.example.toml`
  - `src/dist/install.sh`

### 2. status/doctor 语义按 daemon 默认重写

- `src/internal/app/runtime.go`
  - `status` 会话快照拆分为：`tmux 托管任务` + `daemon 任务`
  - 当无 tmux 托管任务时，明确输出“当前无 tmux 托管任务（默认 daemon 模式）”
  - 执行器快照新增默认模式与 tmux/daemon 目标计数
  - 仓位快照增加 `executor_mode` 字段，便于区分槽位归属
  - `doctor` 的 tmux 检查改为按“是否存在 tmux 托管仓库”决定：
    - 无 tmux 仓库时返回说明，不再把 tmux 缺失当作硬失败

### 3. patrol 语义更新到“健康检查/补偿”

- systemd 描述文案更新（源码与 dist 同步）：
  - `src/ops/systemd/ccclaw-patrol.service`
  - `src/ops/systemd/ccclaw-patrol.timer`
  - `src/dist/ops/systemd/ccclaw-patrol.service`
  - `src/dist/ops/systemd/ccclaw-patrol.timer`
- 语义明确：tmux 仓做会话巡查，daemon 仓做补偿健康检查。

### 4. 文档与测试对齐

- 文档更新：
  - `README.md`
  - `README_en.md`
- `src/internal/app/runtime_test.go`
  - tmux 语义相关测试显式声明 `Executor.Mode=tmux`，避免隐式依赖旧默认值
  - “无任务”快照断言切换为 daemon 默认文案

## 验证结果

- 执行：`cd src && go test ./internal/config ./internal/app`
- 执行：`cd src && go test ./...`
- 结果：全部通过

## 回滚策略

- 全局回滚：将 `[executor].mode` 改回 `tmux`
- 单仓回滚：在 `[[targets]]` 里设置 `executor_mode = "tmux"`

## 后续优化建议

1. 在 `status --json` 中增加 mode 维度聚合（tmux/daemon 各自任务与槽位计数）。
2. 为 daemon 健康检查加入“长时间无事件进展”阈值告警与告警去重。
3. 增加一套端到端回归：覆盖“默认 daemon + 单仓 tmux 覆盖 + 回切”三种路径。
