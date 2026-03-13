# 260313_NA_release_2603131547_upgrade_issue22_tracking

## 背景

2026-03-13，按用户要求完成以下闭环：

1. 立即正确发布一个新 release
2. 从本地发布缓存获取安装包，对本机 `ccclaw` 升级
3. 复核升级后的依赖、配置与调度状态
4. 继续追踪 [Issue #22](https://github.com/41490/ccclaw/issues/22) 是否能被正确发现、执行与反馈

本报告记录本次现场操作事实，以及发布前新发现的 release 阻塞根因。

## 发布前阻塞根因

在正式切版前先执行仓库门禁：

```bash
cd /opt/src/ccclaw/src
make test
make test-install
```

其中 `make test` 通过，但 `make test-install` 首次失败。现场复现发现：

```text
install: cannot stat '/opt/src/ccclaw/src/dist/ops/scripts/ccclaude': No such file or directory
```

继续核对后确认根因不是安装器单点故障，而是发布源漂移：

1. `src/dist/install.sh` 仍按 `dist/ops/scripts/ccclaude` 安装 wrapper
2. 但真正的发布源 `src/ops/` 中缺失：
   - `scripts/ccclaude`
   - `systemd/ccclaw-sevolver.service`
   - `systemd/ccclaw-sevolver.timer`
3. 一旦执行 `make dist-sync`，这些缺失资产就会从 `src/dist/` 被同步删除，导致标准 release 树安装失败

## 修复动作

本轮没有修改业务执行语义，只补齐缺失发布资产并重新同步发布树：

- 新增 `src/ops/scripts/ccclaude`
- 新增 `src/ops/systemd/ccclaw-sevolver.service`
- 新增 `src/ops/systemd/ccclaw-sevolver.timer`
- 重新执行 `make dist-sync`
- 同步更新 `src/dist/ops/examples/app-readme.md`

修复提交：

- `522a4f8 fix(release): 补齐缺失发布资产`

已推送到 `origin/main` 后再执行正式发布，满足 `release-preflight` 的“工作树干净且 HEAD 已推送”要求。

## 发布结果

固定版本号：

```text
26.03.13.1547
```

执行：

```bash
cd /opt/src/ccclaw/src
make release VERSION=26.03.13.1547
```

结果：

- GitHub release：<https://github.com/41490/ccclaw/releases/tag/26.03.13.1547>
- 本地缓存目录：`/ops/logs/ccclaw/26.03.13.1547/`
- 资产：
  - `ccclaw_26.03.13.1547_linux_amd64.tar.gz`
  - `SHA256SUMS`

本地缓存校验：

```bash
cd /ops/logs/ccclaw/26.03.13.1547
sha256sum -c SHA256SUMS
```

结果：

```text
ccclaw_26.03.13.1547_linux_amd64.tar.gz: OK
```

同时确认 tar 内已包含上轮缺失资产：

- `ops/scripts/ccclaude`
- `ops/systemd/ccclaw-sevolver.service`
- `ops/systemd/ccclaw-sevolver.timer`
- `install.sh`
- `upgrade.sh`

## 本机升级结果

升级前版本：

```text
26.03.13.0558
```

升级方式：

1. 直接使用本地缓存 `/ops/logs/ccclaw/26.03.13.1547/`
2. 解包 tar
3. 执行随包 `install.sh`
4. 显式固定：
   - `app_dir=/home/zoomq/.ccclaw`
   - `home_repo=/opt/data/9527`

实际命令：

```bash
release_dir=/tmp/.../ccclaw_26.03.13.1547_linux_amd64
"$release_dir/install.sh" --yes --app-dir "$HOME/.ccclaw" --home-repo /opt/data/9527
```

升级后确认：

### 1. 版本已提升

```bash
~/.ccclaw/bin/ccclaw -V
```

结果：

```text
26.03.13.1547
```

### 2. 关键配置保持正确

`~/.ccclaw/ops/config/config.toml` 中确认：

- `paths.home_repo = "/opt/data/9527"`
- `executor.command = ["/home/zoomq/.ccclaw/bin/ccclaude"]`
- `scheduler.mode = "systemd"`
- `scheduler.logs.retention_days = 30`
- `scheduler.logs.max_files = 200`
- `scheduler.logs.compress = true`
- `approval.minimum_permission = "maintain"`

`.env` 权限仍为：

```text
600 /home/zoomq/.ccclaw/.env
```

### 3. 调度栈已自动重载

执行：

```bash
~/.ccclaw/bin/ccclaw scheduler status
~/.ccclaw/bin/ccclaw scheduler timers --wide
```

结果确认：

- 生效后端仍为 `systemd`
- 6 个 timer 均为 `active + enabled`
  - `ccclaw-ingest.timer`
  - `ccclaw-run.timer`
  - `ccclaw-patrol.timer`
  - `ccclaw-journal.timer`
  - `ccclaw-archive.timer`
  - `ccclaw-sevolver.timer`

### 4. 依赖探查通过

执行：

```bash
~/.ccclaw/bin/ccclaw doctor
```

结果：

- 18 项检查全部 `OK`
- 关键依赖解析正常：
  - `claude=/home/zoomq/.local/bin/claude`
  - `rtk=/home/zoomq/.local/bin/rtk`
  - `tmux=3.3a`
  - `gh=2.87.3`
  - `rg=13.0.0`
  - `duckdb=v1.5.0`

## Issue #22 跟踪结果

### 1. 发现链路已正常

升级后执行：

```bash
~/.ccclaw/bin/ccclaw ingest --log-level debug
~/.ccclaw/bin/ccclaw status --json
```

结果显示：

- `41490/ccclaw#22#body` 已被稳定识别
- `approved = true`
- `target_repo = "41490/ccclaw"`
- `task_class = "general"`

这说明：

- label 识别正常
- 权限与批准评论识别正常
- 任务发现链路已恢复

### 2. 当前不能自动重新执行

Issue #22 在本机状态存储中仍保留历史终态：

- `State = DEAD`
- `RetryCount = 1`
- `ErrorMsg = "tmux 会话退出码=127；诊断输出: Error: Failed to execute command: claude"`

继续追加正规触发词：

- 评论：`/ccclaw rerun`
- 链接：<https://github.com/41490/ccclaw/issues/22#issuecomment-4053399280>

随后重新执行 `ingest`，结果仍为：

- `Issue 状态无变化`
- `state = DEAD`

说明当前版本虽然文档历史上提到支持 `/ccclaw rerun`，但运行态还没有把该指令消费成“终态任务重新入队”。

### 3. 当前不能形成新的自动反馈闭环

由于 `run` 只会拾取 `NEW/FAILED`，而 #22 仍停留在 `DEAD`：

- 本轮不会再次进入执行
- 因而不会产生新的 `tmux` 会话
- 也不会产生新的自动回帖

为保证现场可追溯，本轮已把跟踪结论回帖到 Issue #22：

- 反馈链接：<https://github.com/41490/ccclaw/issues/22#issuecomment-4053404715>

回帖结论是：

- `发现`：正常
- `自动重新执行`：未恢复
- `自动反馈`：未恢复

## 结论

本轮已完成并验证：

1. 修复新 release 的真实阻塞根因：补齐缺失发布资产
2. 正式发布 `26.03.13.1547`
3. 从本地发布缓存完成本机升级
4. 确认版本、配置、依赖、systemd 调度全部处于正确状态
5. 继续追踪并回帖说明 Issue #22 的当前现场结论

截至本报告结束，Issue #22 的准确状态是：

- 可以被正确发现
- 不能从历史 `DEAD` 终态自动重入执行
- 因此不能形成新的自动反馈闭环

下一步真正需要修的不是发布、安装或审批识别，而是：

- 让 `/ccclaw rerun` 被运行态正式消费
- 或提供一个等价的受控重试入口，把 `DEAD` 合法恢复到可执行态
