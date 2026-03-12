# 260312_NA_local_release_2603130558_upgrade_verification

## 背景

2026-03-12，用户要求：

1. 从本地 release 缓存 `/ops/logs/ccclaw/26.03.13.0558/` 获取 `ccclaw 26.03.13.0558`
2. 正确升级当前开发机中的 `ccclaw`
3. 复核关键版本与配置是否完成迁移
4. 用 `gh` 创建一条升级汇报 Issue，说明主要变化、验证方式，以及 [Issue #22](https://github.com/41490/ccclaw/issues/22) 是否已经能正确触发执行并反馈

本报告只记录本机现场升级与验证事实，不改动仓库业务代码。

## 升级前现场

升级前已安装版本：

```bash
~/.ccclaw/bin/ccclaw -V
```

结果：

```text
26.03.12.0315
```

升级前配置关键点：

- `paths.home_repo = "/opt/data/9527"`
- `scheduler.mode = "systemd"`
- 仅启用了 4 个旧 timer：
  - `ccclaw-ingest.timer`
  - `ccclaw-run.timer`
  - `ccclaw-patrol.timer`
  - `ccclaw-journal.timer`
- `Issue #22` 历史上已有一次 `DEAD`
  - 原错误：`tmux 会话 ... 已丢失，且未找到结果文件`

## Release 缓存核验

本地缓存目录存在：

- `/ops/logs/ccclaw/26.03.13.0558/ccclaw_26.03.13.0558_linux_amd64.tar.gz`
- `/ops/logs/ccclaw/26.03.13.0558/SHA256SUMS`

同时 `gh release view 26.03.13.0558 --repo 41490/ccclaw` 可确认该 tag 已于 `2026-03-12T21:58:41Z` 发布。

## 现场发现的 release 资产问题

直接执行缓存包中的：

```bash
install.sh --yes --skip-deps --app-dir ~/.ccclaw --home-repo /opt/data/9527
```

失败，报错为：

```text
install: cannot stat '.../ops/scripts/ccclaude': No such file or directory
```

进一步核对发现：

1. `26.03.13.0558` 缓存包中缺少 `ops/scripts/ccclaude`
2. 安装器会启用 `ccclaw-sevolver.timer`
3. 但该安装器自身并未在 user systemd 目录生成 `ccclaw-sevolver.service/.timer`

因此，`26.03.13.0558` 的 release 安装器在当前现场不能直接完成闭环升级。

## 实际升级路径

为满足“从本地 release 缓存升级当前环境”的目标，本轮采用补偿式运维升级：

1. 以缓存包中的 `bin/ccclaw`、`install.sh`、`upgrade.sh`、`ops/*` 为主更新程序树
2. 显式保留当前程序目录 `~/.ccclaw`
3. 显式保留当前知识仓库 `home_repo=/opt/data/9527`
4. 补装缓存包缺失但当前仓库 `src/dist/` 已存在的辅助资产：
   - `src/dist/ops/scripts/ccclaude`
   - `src/dist/ops/systemd/ccclaw-sevolver.service`
   - `src/dist/ops/systemd/ccclaw-sevolver.timer`
5. 用新版 CLI 执行：
   - `ccclaw config set-scheduler ...`
   - `ccclaw scheduler use systemd`

这样做的目的不是“混装一个任意版本”，而是用缓存包中的 `26.03.13.0558` 主二进制完成升级，再补齐其自身安装契约缺失的资产。

## 升级后结果

### 1. 版本已升级

执行：

```bash
~/.ccclaw/bin/ccclaw -V
```

结果：

```text
26.03.13.0558
```

### 2. 配置已更新到新版 schema

`~/.ccclaw/ops/config/config.toml` 中确认：

- `home_repo` 仍为 `/opt/data/9527`
- `executor.command = ["/home/zoomq/.ccclaw/bin/ccclaude"]`
- `scheduler.logs` 已补齐：
  - `retention_days = 30`
  - `max_files = 200`
  - `compress = true`

说明升级没有把本体仓库错误改回默认 `/opt/ccclaw`，且新版日志归档配置已到位。

### 3. 新 timer 已启用

执行：

```bash
~/.ccclaw/bin/ccclaw scheduler status
~/.ccclaw/bin/ccclaw scheduler timers
```

结果显示 `systemd` 已恢复为当前生效后端，且 6 个 timer 均已启用：

- `ccclaw-ingest.timer`
- `ccclaw-run.timer`
- `ccclaw-patrol.timer`
- `ccclaw-journal.timer`
- `ccclaw-archive.timer`
- `ccclaw-sevolver.timer`

### 4. 新版运行态可见变化

对比 `26.03.12.0315`，本机直接可观测到的主要变化包括：

1. CLI 新增：
   - `archive`
   - `sevolver`
2. `doctor` 新增执行器解析信息：
   - `configured`
   - `resolved`
   - `claude`
   - `rtk`
3. `status` / `scheduler status` 对完整 systemd 托管的判定已纳入：
   - `archive.timer`
   - `sevolver.timer`

## 对 Issue #22 的现场验证

## 验证前提

升级后新版状态存储 `~/.ccclaw/var/state.json` 中，`41490/ccclaw#22#body` 被识别为：

- `Approved = true`
- `State = NEW`

这说明在新版状态模型下，#22 已经满足执行前提。

## 实际触发结果

手工执行：

```bash
~/.ccclaw/bin/ccclaw run --log-level debug
```

结果：

- 成功重新挂入 tmux 会话
- 会话名：`ccclaw-41490_ccclaw_22_body-4430`

但运行日志 `~/.ccclaw/log/41490_ccclaw_22_body.log` 显示：

```text
Error: Failed to execute command: claude

Caused by:
    No such file or directory (os error 2)
```

## 根因判断

这次失败已经不同于旧问题：

1. 新版执行器已能解析出绝对路径：
   - `claude=/home/zoomq/.local/bin/claude`
   - `rtk=/home/zoomq/.local/bin/rtk`
2. 但当前 `ccclaude` 包装器优先执行的是：
   - `exec "$rtk_bin" proxy claude "$@"`
3. `rtk proxy` 这一跳仍然按字面量 `claude` 再次查 PATH
4. tmux pane 内 PATH 不含 `~/.local/bin`
5. 因而失败从“wrapper 找不到 claude”前移为“rtk proxy 找不到 claude”

也就是说：

- `26.03.13.0558` 已修复了执行器对 `claude/rtk` 的绝对路径发现
- 但尚未修复 `rtk proxy claude` 这条链仍依赖 tmux PATH 的问题

## 自动反馈是否成功

没有成功。

原因不是 GitHub 回帖权限，而是 `patrol` 当前还存在另一处独立问题：

- `tmux list-panes -a` 在本机会枚举到其它非 `ccclaw` 活跃会话
- 活跃 pane 的 `#{pane_dead_status}` 为空
- 现实现对所有 pane 先全量解析，再按前缀过滤
- 于是 `patrol/status` 会在解析阶段直接报：

```text
解析 tmux exit code 失败: strconv.Atoi: parsing "": invalid syntax
```

这导致：

1. `run` 能挂入 tmux
2. tmux pane 实际已死
3. `patrol` 却无法完成失败收口与 GitHub 自动回帖

## 对 #22 的结论

截至本轮升级验证结束时，答案是：

### 还不能“正确触发执行并自动反馈”

更准确地说：

1. **触发前置条件已经满足**
   - `#22` 可重新进入 `NEW`
   - `run` 能发起执行
2. **执行链路仍会失败**
   - 当前失败点位于 `rtk proxy claude`
3. **失败后的自动反馈仍不闭环**
   - 当前 `patrol` 会被 tmux 活跃 pane 的空 exit code 解析问题拦住

因此本轮不能把 `#22` 结论写成“已恢复自动闭环”。

## 现场收口

为避免 timer 恢复后持续重复踩坑，本轮做了最小现场清理：

1. 杀掉已死亡的 `tmux` 会话 `ccclaw-41490_ccclaw_22_body-4430`
2. 将 `~/.ccclaw/var/state.json` 中的 `#22` 校正为本次真实失败态：
   - `State = DEAD`
   - `RetryCount = 1`
   - `ErrorMsg = "tmux 会话退出码=127；诊断输出: Error: Failed to execute command: claude"`
3. 重新启用 6 个 systemd timer，确保升级后的调度栈处于一致状态

## 验证命令清单

本轮实际使用的关键验证命令包括：

```bash
~/.ccclaw/bin/ccclaw -V
~/.ccclaw/bin/ccclaw doctor
~/.ccclaw/bin/ccclaw status
~/.ccclaw/bin/ccclaw scheduler status
~/.ccclaw/bin/ccclaw scheduler timers
~/.ccclaw/bin/ccclaw run --log-level debug
tmux capture-pane -p -t ccclaw-41490_ccclaw_22_body-4430
gh issue view 22 --repo 41490/ccclaw --comments
```

## 结论

本轮已经完成：

1. 使用本地 release 缓存将当前环境主版本提升到 `26.03.13.0558`
2. 保持 `home_repo=/opt/data/9527` 不变
3. 补齐并启用了 `archive/sevolver` 调度能力
4. 证明了 `#22` 已能重新进入“可执行前置条件满足”的状态

但本轮同时坐实两个残留问题：

1. `26.03.13.0558` release 安装器资产不完整
2. `#22` 仍不能通过标准 `tmux + patrol` 链路自动完成失败收口与 GitHub 回帖

因此，本次升级结论应表述为：

### 程序已升级成功，但 Issue #22 的自动执行反馈闭环尚未恢复
