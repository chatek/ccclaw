# 260312_34_tmux_wrapper_path_patrol_fix

## 背景

2026-03-12，Issue [#34](https://github.com/41490/ccclaw/issues/34) 明确要求按已建议方案推进，目标是修复 tmux pane 内 `PATH` 缺失 `~/.local/bin` 时：

- `ccclaude` 找不到 `claude` / `rtk`
- `tmux send-keys` 启动链路不稳
- `stderr-only` 失败被误报成“tmux 会话丢失”

本轮按 Issue 已拍板方向直接落地，不再停留在取证阶段。

## 根因

问题分成三层：

1. 执行器只保证能找到 `ccclaude`，没有把 wrapper 依赖的 `claude` / `rtk` 解析成稳定绝对路径。
2. tmux 通过 `new-session` 后再 `send-keys` 注入命令，pane 生命周期与真实命令生命周期耦合不紧，且额外依赖交互 shell 行为。
3. patrol 收口主要依赖 `result.json`，当结果为空或为非 JSON stderr 时，会退化成“会话丢失/无结果”。

## 本轮改动

### 1. 执行器显式解析二进制路径

修改 [executor.go](/opt/src/ccclaw/src/internal/executor/executor.go)：

- `executor.New()` 不再只做 `exec.LookPath`，而是补充 `PATH + ~/.local/bin + ~/bin` 的回退搜索。
- 自动解析并注入：
  - `CCCLAW_CLAUDE_BIN`
  - `CCCLAW_RTK_BIN`
- `ccclaude` 场景下的 RTK 探测不再只依赖当前 PATH。

这样即使 `ccclaw` 运行在 user systemd / tmux / cron 的收窄 PATH 下，也能把 wrapper 依赖的真实二进制路径带进去。

### 2. tmux 改为直接带命令启动

修改 [manager.go](/opt/src/ccclaw/src/internal/tmux/manager.go)：

- `tmux new-session -d -s <name> -c <dir> "<command>"`
- 删除 `send-keys`
- 删除 `pipe-pane`

同时修改 [executor.go](/opt/src/ccclaw/src/internal/executor/executor.go) 的 tmux 命令构造：

- 从 `stdout | tee result.json`
- 改为 `stdout+stderr -> tee result.json log`

这样 pane 启动就是实际命令启动，stderr 也会被同步落到结果与日志。

### 3. patrol 收口保留真实诊断

修改 [runtime.go](/opt/src/ccclaw/src/internal/app/runtime.go)：

- 当 `LoadResult()` 无法解析 JSON，但结果文件里有原始输出时，保留原始输出。
- 当结果文件为空时，回退读取日志尾部作为诊断。
- 失败错误文案追加 `诊断输出:`，避免 Issue 回帖只剩“会话丢失”。

## 回归测试

新增/更新测试：

- [executor_test.go](/opt/src/ccclaw/src/internal/executor/executor_test.go)
  - 覆盖 `~/.local/bin` 自动解析
  - 覆盖 `claude not found`
  - 覆盖 `rtk not found`
  - 覆盖 tmux 命令同时采集 stderr
- [manager_test.go](/opt/src/ccclaw/src/internal/tmux/manager_test.go)
  - 覆盖 tmux 直接 `new-session` 带命令
  - 断言不再调用 `send-keys` / `pipe-pane`
- [runtime_test.go](/opt/src/ccclaw/src/internal/app/runtime_test.go)
  - 覆盖 raw stderr 失败提示
  - 覆盖空 `result.json` 时回退日志尾部
  - 覆盖不再误报“会话丢失”

## 验证

已执行：

```bash
go test ./internal/executor ./internal/tmux ./internal/app
bash -n src/dist/install.sh
```

结果均通过。

## 影响

本轮修复后：

- `ccclaude` 不再依赖 tmux pane 的交互 PATH 才能找到 `claude` / `rtk`
- tmux pane 生命周期与真实执行命令绑定更紧
- 缺命令、stderr-only、空结果文件这三类失败都会回到任务错误模型
- Issue 对外失败描述将更接近真实根因，而不是泛化成“会话丢失”
