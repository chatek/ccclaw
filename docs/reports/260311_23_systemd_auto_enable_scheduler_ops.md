# 260311_23_systemd_auto_enable_scheduler_ops

## 背景

承接 Issue #23 关于以下拍板：

1. 安装/升级在可直连 `user bus` 时自动启用 `systemd --user` timer
2. 调度运维入口收敛到 `ccclaw scheduler ...`
3. 运维手册落在 `~/.ccclaw/README.md`
4. timer 周期采用 `OnCalendar + calendar_timezone`
5. `github.limit` 保持原义，并在 TOML 中补注释

本轮目标是把上述决策收口到可安装、可升级、可观察的最小闭环。

## 实施

### 1. 安装/升级自动守护

- `src/dist/install.sh`
  - 新增对已有 scheduler 配置的读取，避免 upgrade 时被默认值回写覆盖
  - `systemd` 可控时自动执行：
    - `systemctl --user daemon-reload`
    - `systemctl --user enable --now ...`
  - 若本次属于重装/升级，额外自动 `restart` 托管 timer，确保新 unit 生效
  - 若当前会话无法直连 `user bus`，继续保留手工提示，不伪成功
- `src/dist/upgrade.sh`
  - 文案改为“仅在本轮未自动直连 `user bus` 时，才需手工 restart”

### 2. 调度运维入口

- `src/cmd/ccclaw/main.go`
  - 新增 `ccclaw scheduler timers`
  - 新增 `ccclaw scheduler logs [all|ingest|run|patrol|journal]`
- `src/internal/scheduler/`
  - 新增 `logs.go`
  - 新增 `systemd_units.go`
  - 重构 `systemd.go`
    - unit 不再从静态模板生搬，而是按配置动态生成
    - `timers` 可显示：
      - 主机本地时区下次/上次触发
      - 配置时区下次/上次触发
      - 当前 `OnCalendar` 生效表达式

### 3. 配置与注释

- `src/internal/config/config.go`
  - 新增：
    - `scheduler.calendar_timezone`
    - `[scheduler.timers]`
  - 默认值：
    - `calendar_timezone = "Asia/Shanghai"`
    - `ingest = "*:0/5"`
    - `run = "*:0/10"`
    - `patrol = "*:0/2"`
    - `journal = "*-*-* 23:50:00"`
  - `config.Save()` 改为输出带注释的配置文件，避免 `target add` 等操作把注释全部洗掉
  - `config set-scheduler` / `scheduler use` 改为优先保留 scheduler 段注释
- `src/ops/config/config.example.toml`
  - 为 `github.limit`、`calendar_timezone`、`scheduler.timers.*` 补中文说明
  - 明确 `*-*-* 23:50:00` 与 `*-*-* 01:01:42` 的含义

### 4. 程序目录手册

- 新增 `src/ops/examples/app-readme.md`
- 安装时同步到 `~/.ccclaw/README.md`
- 使用受管区块，升级时保留用户补充

### 5. 默认 unit 示例

- `src/ops/systemd/*.timer`
  - 默认表达式更新为带 `Asia/Shanghai`
  - 描述文案改为通用的 “on schedule”

## 验证

执行：

```bash
cd src
go test ./...
make dist-sync
bash tests/install_regression.sh
```

结果通过。

新增回归覆盖：

- `scheduler timers` 命令输出
- `scheduler logs` 命令参数拼装
- `UpdateSchedulerSection()` 保留外部内容
- 安装链：
  - `systemd` 可控时自动 `enable --now`
  - 重装时自动 `restart`
  - `~/.ccclaw/README.md` 落盘

## 结论

本轮已把 Issue #23 当前拍板的第一阶段收口为可交付状态：

- 正常 `systemd --user` 环境下，安装后即开始守护
- 已有统一的 timer/logs 运维入口
- `config.toml` 可直接解释 `limit`、时区与 timer 周期
- 升级后程序目录手册可自动刷新

## 后续

仍可继续追加，但本轮未实现：

1. 日志级别配置化
2. `scheduler doctor` 独立子命令
3. 更细的跨时区展示与更多格式化视图
