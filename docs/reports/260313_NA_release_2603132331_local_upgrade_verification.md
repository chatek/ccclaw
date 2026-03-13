# 260313_NA_release_2603132331_local_upgrade_verification

## 背景

2026-03-13，用户要求：

1. 将当前主线版本发布为一个新的 release
2. 仅使用本机本地缓存完成本机 `CCClaw` 升级
3. 验证升级确实生效，便于开始下一轮实验

本报告仅记录本次 release 与本地缓存升级的现场事实。

## Issue 决策核查

按仓库约束，先核对现有 Issue 与已拍板约束：

- 2026-03-13 执行 `gh issue list --repo 41490/ccclaw --state all --limit 100`
- 未发现一条专门要求“发布 `26.03.13.2331` 并升级本机”的独立新 Issue
- 但 release/upgrade 链路已有明确历史拍板与闭环记录，可直接按既定流程执行：
  - Issue #7：首个可正确下载安装 release 的交付收口
  - Issue #28：`upgrade.sh` 实际不会升级发布树且不会补齐新配置
  - Issue #37：本机升级到 `26.03.13.0558` 的现场验证闭环

因此，本轮不涉及新的产品分歧，只按既有发布契约执行现场运维操作。

## 发布前现场

源码当前版本号脚本输出：

```bash
src/ops/scripts/version.sh
```

结果：

```text
26.03.13.2331
```

发布前本机已安装版本：

```bash
~/.ccclaw/bin/ccclaw -V
```

结果：

```text
26.03.13.1547
```

发布前关键运行边界：

- 程序目录：`/home/zoomq/.ccclaw`
- 本体仓库：`/opt/data/9527`
- 配置文件：`/home/zoomq/.ccclaw/ops/config/config.toml`
- 当前运行调度模式：`systemd`

## Release 发布结果

执行：

```bash
make release
```

执行位置：

- `/opt/src/ccclaw/src`

实际结果：

- 成功生成 `ccclaw_26.03.13.2331_linux_amd64.tar.gz`
- 成功生成 `SHA256SUMS`
- 成功归档到本机缓存目录 `/ops/logs/ccclaw/26.03.13.2331/`
- 成功创建 GitHub release：
  - tag：`26.03.13.2331`
  - target commit：`7fa7c8853026ac00516550f6dd340c0d86c9daed`
  - publishedAt：`2026-03-13T15:31:53Z`
  - URL：<https://github.com/41490/ccclaw/releases/tag/26.03.13.2331>

release 资产核对：

- `ccclaw_26.03.13.2331_linux_amd64.tar.gz`
- `SHA256SUMS`

本机缓存校验：

```bash
cd /ops/logs/ccclaw/26.03.13.2331 && sha256sum -c SHA256SUMS
```

结果：

```text
ccclaw_26.03.13.2331_linux_amd64.tar.gz: OK
```

## 本地缓存升级路径

本轮明确不走远端下载升级，而是直接使用本机 release 缓存：

- 缓存目录：`/ops/logs/ccclaw/26.03.13.2331/`
- 安装包：`ccclaw_26.03.13.2331_linux_amd64.tar.gz`

先做一次模拟安装，确认关键边界不漂移：

```bash
install.sh --simulate --yes --skip-deps --app-dir ~/.ccclaw --home-repo /opt/data/9527
```

模拟结果确认：

- 程序目录仍为 `/home/zoomq/.ccclaw`
- 本体仓库仍为 `/opt/data/9527`
- 调度模式仍为 `systemd`

随后执行真实升级：

```bash
install.sh --yes --skip-deps --app-dir ~/.ccclaw --home-repo /opt/data/9527
```

说明：

- `--skip-deps` 仅跳过系统依赖安装，不影响程序树更新
- 仍由 release 内安装器负责刷新：
  - `bin/ccclaw`
  - `bin/ccclaude`
  - `install.sh`
  - `upgrade.sh`
  - `ops/*`
- 不覆盖用户本体仓库路径；继续保留 `home_repo=/opt/data/9527`

## 升级后验证

### 1. CLI 版本已切换

执行：

```bash
~/.ccclaw/bin/ccclaw -V
/home/zoomq/.local/bin/ccclaw -V
```

结果：

```text
26.03.13.2331
26.03.13.2331
```

说明程序目录内二进制与本地命令链接都已切到新版本。

### 2. 已安装二进制与 release 包内二进制完全一致

执行：

```bash
cmp -s <release>/bin/ccclaw ~/.ccclaw/bin/ccclaw
sha256sum <release>/bin/ccclaw ~/.ccclaw/bin/ccclaw
```

结果要点：

- `cmp -s` 返回一致
- 两边 `sha256` 完全相同：
  - `ee859174a6f02153a89ca6c44577d0cd08ed20492d90db253b8d64f4ac9fc34a`

这说明本机当前运行二进制不是“近似升级”，而是与 release 缓存中的目标二进制逐字节一致。

### 3. 配置边界已保留

升级后 `~/.ccclaw/ops/config/config.toml` 复核确认：

- `paths.app_dir = "/home/zoomq/.ccclaw"`
- `paths.home_repo = "/opt/data/9527"`
- `paths.kb_dir = "/opt/data/9527/kb"`
- `executor.command = ["/home/zoomq/.ccclaw/bin/ccclaude"]`
- `scheduler.mode = "systemd"`
- `scheduler.logs.retention_days = 30`
- `scheduler.logs.max_files = 200`
- `scheduler.logs.compress = true`

说明升级没有把本体仓库错误改回默认 `/opt/ccclaw`，也没有把执行器入口和调度策略打乱。

### 4. 调度器已重新接管

执行：

```bash
~/.ccclaw/bin/ccclaw scheduler status --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env
~/.ccclaw/bin/ccclaw scheduler timers --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env
```

结果：

- `request=systemd`
- `effective=systemd`
- 原因：`ccclaw-ingest.timer, ccclaw-patrol.timer, ccclaw-journal.timer, ccclaw-archive.timer, ccclaw-sevolver.timer 已启用且运行中`

关键 timer 视图显示：

- `ingest active enabled`
- `patrol active enabled`
- `journal active enabled`
- `archive active enabled`
- `sevolver active enabled`

说明升级后 user systemd 单元已被重新装载并恢复托管。

### 5. doctor 总诊断通过

执行：

```bash
~/.ccclaw/bin/ccclaw doctor --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env
```

结果摘要：

- `failed=0`
- `checks=18`

关键通过项包括：

- 配置文件
- `.env` 权限
- 执行器入口解析
- Claude 运行态
- `gh` / `git` / `tmux` / `rg` / `duckdb`
- 状态存储读写
- 调度器托管状态

### 6. 本体仓库路径仍然可用

执行：

```bash
git -C /opt/data/9527 status --short --branch
git -C /opt/data/9527 log -1 --oneline --decorate
```

结果：

- 当前仍是 `main...origin/main [ahead 2]`
- 最新提交：`575dd4c (HEAD -> main) seed ccclaw home repo`

说明升级过程仍在原本体仓库 `/opt/data/9527` 上工作，没有切换到默认路径，也没有把该仓库替换成新的空目录。

## 结论

本轮已完成以下闭环：

1. 当前主线源码成功发布为 release `26.03.13.2331`
2. release 资产已归档到本机缓存 `/ops/logs/ccclaw/26.03.13.2331/`
3. 本机 `CCClaw` 已从 `26.03.13.1547` 正确升级到 `26.03.13.2331`
4. 本机安装二进制与 release 包内二进制逐字节一致
5. `home_repo=/opt/data/9527`、执行器入口、systemd 托管均已保留并通过诊断

因此，可以在当前机器上以 `26.03.13.2331` 作为新的实验起点继续下一轮验证。
