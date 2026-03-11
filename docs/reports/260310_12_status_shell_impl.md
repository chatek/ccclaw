# 260310_12_status_shell_impl

## 背景

Issue #12 在拍板后明确了两条执行边界：

1. `status` 负责“当前快照”，历史 token 聚合继续留在 `stats`
2. shell 集成默认关闭，只有显式参数才允许写入 `~/.bashrc`，且必须可回滚

在 #10 已完成 tmux/session 与 token 统计落地后，本轮开始把上述语义正式补进主线实现。

## 本轮实现

### 1. 增强 `ccclaw status`

本轮将 `status` 从“仅列任务状态”扩展为“当前运行态快照”，新增：

- 调度快照：显示请求模式、生效模式、当前说明与修复建议
- 会话快照：基于 tmux session/task 映射汇报运行中任务、接近超时、超时、已退出、丢失、孤儿会话
- token 快照：显示累计 runs/sessions/tokens/cost，并附 RTK 对比摘要入口
- 最近异常：显示最近 7 天内的 `FAILED` / `DEAD` / `WARNING` 事件摘要
- 原有任务列表保留，但不再是 `status` 的唯一输出

### 2. 修复最近异常数据面根因

实现过程中发现：

- `task_events.created_at` 使用 SQLite `CURRENT_TIMESTAMP`
- 先前若直接在 SQL 层用 Go `time.Time` 做范围过滤，时间格式口径并不稳定
- 会造成 `status` 抓不到刚写入的异常事件

因此本轮没有继续堆条件拼接，而是补了：

- `storage.ListTaskEvents(limit)`：先稳定读取最近事件
- 再在 Go 层按“最近 7 天”窗口筛选

这样避免了不同时间格式混用导致的漏报。

### 3. 增加 shell 集成显式入口

安装器 `src/dist/install.sh` 新增：

- `--inject-shell bashrc`
- `--remove-shell bashrc`

约束如下：

- 默认仍关闭，不会静默写入 `~/.bashrc`
- 仅支持受控标记块写入
- 当前只补 PATH，不追加 alias/function
- 重复执行会先替换旧受控块，保持幂等
- 回滚只移除受控块，不碰用户自定义内容

同时更新了 release README，补充显式写入与回滚命令。

## 验证

已执行：

- `go test ./...`
- `bash src/tests/install_regression.sh`

新增覆盖：

- `status` 运行态快照输出
- `status` 在无任务、无 tmux 时的降级输出
- 安装器 shell 集成写入、幂等重装与回滚

## 结果

Issue #12 中本轮已推进并落地：

- `status` 当前快照语义
- shell 集成显式注入与可回滚边界

仍可后续继续演进的点：

- shell 集成若后续需要 `source` 入口，可在同一受控块内追加，但仍应保持默认关闭
- `status` 可继续补充更细的 scheduler/timer 最近执行记录
- `stats` 后续仍可按 #10 既定方向补日期范围和按天聚合
