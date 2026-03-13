# 260313_42_ingest_slot_finalizing_refactor

## 背景

Issue #42 已拍板的新运行模型包含 4 个核心约束：

- `run` 定时任务并回 `ingest`
- 按 `target_repo` 分片排队，而不是全局混排
- 每个任务仓库只允许 1 个 Claude 进程，不同仓库之间允许并行
- `patrol` 只负责 Claude 进程健康与 sidecar 运行态，不再直接收口 `DONE/FAILED/DEAD`

同时，`sync/push/comment` 失败不能再把任务直接打回普通 `FAILED`，否则会导致 Claude 对已完成编码任务重复执行。

## 本轮实现

### 1. 主状态新增 `FINALIZING`

- 在 [src/internal/core/task.go](/opt/src/ccclaw/src/internal/core/task.go) 新增 `FINALIZING`
- 语义改为：
  - `RUNNING`: Claude 正在执行
  - `FINALIZING`: Claude 已完成，正在做 `sync_target` / `sync_home` / `Issue comment`
  - `DONE`: 收尾全部完成

这样 `sync/push/report` 失败时，不会把任务重新放回普通执行阶段。

### 2. `state.json` 补原子更新能力

- 在 [src/internal/adapters/storage/state_store.go](/opt/src/ccclaw/src/internal/adapters/storage/state_store.go) 增加：
  - `GetTask`
  - `ApplyTask`
- `ApplyTask` 在文件锁内完成：
  - 读取旧值
  - 应用变更
  - 自动维护 `CreatedAt/UpdatedAt`

这一步直接收了 Issue #41 暴露出的“旧快照整条覆盖新状态”根因。

### 3. 新增按仓 sidecar 槽位存储

- 新增 [src/internal/adapters/storage/slot_store.go](/opt/src/ccclaw/src/internal/adapters/storage/slot_store.go)
- sidecar 按 `target_repo` 保存运行槽位
- 当前持久化字段包括：
  - `phase`
  - `task_id`
  - `session_name`
  - `restart_count`
  - `sync_target`
  - `sync_home`
  - `report_issue`
  - `last_error`
  - `hints`

当前 `phase` 采用：

- `running`
- `pane_dead`
- `restarting`
- `finalizing`
- `finalize_failed`

### 4. `ingest` 改为按仓循环

- 新增 [src/internal/app/runtime_cycle.go](/opt/src/ccclaw/src/internal/app/runtime_cycle.go)
- `ingest` 现在在完成 Issue 同步后，会执行：
  - 升级兜底：为旧版遗留的 `RUNNING/FINALIZING` 任务补 sidecar
  - 按仓推进已有 slot
  - 按仓选择一个空槽位发射队首任务

关键行为：

- 每个仓每轮最多处理 1 个活跃槽位
- 同一轮收口过的仓，不再立刻重发下一个任务，避免“同轮自撞”
- `run` 子命令保留为兼容壳，内部直接转发到新循环

### 5. `patrol` 收缩为健康代理

- `patrol` 不再直接把任务写成 `DONE/FAILED/DEAD`
- `patrol` 只做：
  - hook 中 `session_id` 变化同步回 `state.json`
  - tmux `pane_dead` 识别并写 sidecar
  - 上下文腐败 / 超时后的原地重启

原地重启后的运行结果：

- 任务主状态仍保持 `RUNNING`
- sidecar 记录新的 `session_name` 与 `restart_count`

### 6. `FINALIZING` 阶段统一做 `jj` 收尾

- 收尾阶段统一从 `ingest` 执行，不再从 `patrol` 执行 repo sync
- 任务收尾步骤被拆成：
  - `sync_target`
  - `sync_home`
  - `report_issue`
- 仓库同步统一走现有 `jj` 路径，失败只在 `jj` 明确不可用或不兼容时退化

其中：

- `target_repo` 与 `home_repo` 分开记录结果
- 遇到 `jj` 冲突时，把 sidecar 标为 `conflict`
- Issue 自动回帖失败时，只保留 `FINALIZING`，等待后续补回帖

### 7. 新增收尾失败提示回帖

- 在 [src/internal/adapters/reporter/reporter.go](/opt/src/ccclaw/src/internal/adapters/reporter/reporter.go) 新增 `ReportFinalizing`
- 当 `sync_target` / `sync_home` 失败时，会尝试回帖说明：
  - 哪一步失败
  - 错误摘要
  - GitHub 与本机 `jj` 建议处理动作

## 调度与发布资产同步

### 1. 删除 `run.timer`

已从以下位置去掉 `run` 定时任务：

- [src/internal/scheduler/systemd.go](/opt/src/ccclaw/src/internal/scheduler/systemd.go)
- [src/internal/scheduler/systemd_units.go](/opt/src/ccclaw/src/internal/scheduler/systemd_units.go)
- [src/ops/systemd/ccclaw-run.service](/opt/src/ccclaw/src/ops/systemd/ccclaw-run.service)
- [src/ops/systemd/ccclaw-run.timer](/opt/src/ccclaw/src/ops/systemd/ccclaw-run.timer)
- [src/dist/ops/systemd/ccclaw-run.service](/opt/src/ccclaw/src/dist/ops/systemd/ccclaw-run.service)
- [src/dist/ops/systemd/ccclaw-run.timer](/opt/src/ccclaw/src/dist/ops/systemd/ccclaw-run.timer)

### 2. 调整默认周期

- `ingest` 默认周期从 `*:0/5` 改为 `*:0/4`
- `patrol` 维持 `*:0/2`

### 3. `scheduler logs run` 保留兼容

- [src/internal/scheduler/logs.go](/opt/src/ccclaw/src/internal/scheduler/logs.go) 保留 `run` scope
- 但现在会映射到 `ccclaw-ingest.service`

### 4. cron 规则同步收缩

- `cron` 受控规则从 `ingest/run/patrol/journal` 四条收缩为三条：
  - `ingest`
  - `patrol`
  - `journal`

### 5. 安装与升级链路已同步

已同步修改：

- [src/dist/install.sh](/opt/src/ccclaw/src/dist/install.sh)
- [src/dist/upgrade.sh](/opt/src/ccclaw/src/dist/upgrade.sh)
- [src/dist/README.md](/opt/src/ccclaw/src/dist/README.md)
- [src/ops/config/config.example.toml](/opt/src/ccclaw/src/ops/config/config.example.toml)
- [src/dist/ops/config/config.example.toml](/opt/src/ccclaw/src/dist/ops/config/config.example.toml)
- [src/ops/examples/app-readme.md](/opt/src/ccclaw/src/ops/examples/app-readme.md)
- [src/dist/ops/examples/app-readme.md](/opt/src/ccclaw/src/dist/ops/examples/app-readme.md)

## 验证

已执行：

```bash
cd src && go test ./...
bash src/tests/install_regression.sh
```

验证结果：

- Go 全量测试通过
- `install.sh` 回归测试全部通过

## 剩余注意事项

### 1. `paths.state_db` 命名仍是历史兼容

本轮已经彻底不再依赖 SQLite 运行态，但配置项命名仍叫 `state_db`。  
这是兼容层，后续可再迁移为更准确的 `state_dir/var_dir`。

### 2. `FINALIZING` 目前不会自动转 `DEAD`

这是按 Issue #42 已拍板执行的：

- `FINALIZING` 表示“编码已完成，但交付收尾待人工或后续补齐”
- 不应被误判成执行失败终态

### 3. `PR` 模式尚未引入

当前仍按“`jj` 优先直接同步”处理。  
若目标仓库启用严格 branch protection，需要后续再评估“自动建 PR”模式。
