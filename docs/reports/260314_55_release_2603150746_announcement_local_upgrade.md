# 260314_55_release_2603150746_announcement_local_upgrade

## 背景

- 对应 Issue：#55 `ops: 发布 26.03.15.0746 并完成本机缓存升级验收`
- 操作日期：
  - 本地时区（US/Eastern）：2026-03-14
  - release 发布时间（UTC）：2026-03-14T23:47:47Z
- 本轮目标：
  1. 正确发布当前 `main` 为新 release
  2. 在 `Announcements` 分类补对应公告
  3. 直接使用本机缓存中的最新 release 对当前主机升级
  4. 核验升级后的关键状态、版本与运行形态

## 发布前核对

### 1. Issue 与分支状态

- `gh issue list --limit 20 --state all`
  - `#46/#47/#48/#49` 已关闭，对应 `stream-json -> daemon -> 默认 daemon` 决策链路已收口
  - 仓库当前还有 `#53 audit` 等开放项，但不阻塞本轮发布
- `git status --short --branch`
  - 当前分支：`main...origin/main`
  - 工作树干净

### 2. 版本与发布入口

- `src/ops/scripts/version.sh` 输出：`26.03.15.0746`
- `src/Makefile`
  - `release` 目标会先执行 `release-preflight`
  - 打包源固定为 `src/dist/`
  - 产物归档到 `/ops/logs/ccclaw/<version>/`
  - release 资产至少包括安装包与 `SHA256SUMS`

### 3. 发布前回归

- 执行：`cd src && go test ./...`
- 结果：全部通过
- 执行：`cd src && make release-preflight VERSION=26.03.15.0746`
- 结果：通过

## 发布与公告

### 1. Release

执行：

```bash
cd /opt/src/ccclaw/src
make release VERSION=26.03.15.0746
```

结果：

- release URL：<https://github.com/41490/ccclaw/releases/tag/26.03.15.0746>
- tag：`26.03.15.0746`
- target commit：`f6a12c6db3ed0f5ccc634577fa7b478c223a8947`
- 发布时间：`2026-03-14T23:47:47Z`

资产：

- `ccclaw_26.03.15.0746_linux_amd64.tar.gz`
  - digest：`sha256:d2085794dfa67d93127c850fa22ca56746a6df259c5345dfadbeadf8e5ff1989`
- `SHA256SUMS`
  - digest：`sha256:73aa38d86b7e483f13b9ff541854f24b13be815c654465a549cbc6b11350104f`

本机归档校验：

```bash
cd /ops/logs/ccclaw/26.03.15.0746
sha256sum -c SHA256SUMS
```

结果：

- `ccclaw_26.03.15.0746_linux_amd64.tar.gz: OK`

### 2. Discussion 公告

通过 GitHub GraphQL 创建 `Announcements` 分类讨论，分类 ID 为 `DIC_kwDORXxVic4C37tM`。

结果：

- discussion URL：<https://github.com/41490/ccclaw/discussions/54>
- 标题：`ccclaw 26.03.15.0746 已发布`

公告正文明确同步了以下语义：

- 默认执行模式已切到 `daemon`
- Claude Code 运行产物以 `stream-json` 为基线
- `tmux` 仅保留 debug attach、旧链路兼容与 patrol 补偿用途
- 升级后优先用 `status` / `doctor` / `scheduler status` 复核

## 本机缓存升级

### 1. 升级前基线

- 原版本：`26.03.13.2331`
- 当前程序目录：`/home/zoomq/.ccclaw`
- 当前知识仓库：`/opt/data/9527`
- `gh auth status` 已登录，具备 `repo` 与 `write:discussion` 权限
- 升级前 `ccclaw status` 已显示：
  - 调度后端为 `systemd`
  - 执行器入口为 `/home/zoomq/.ccclaw/bin/ccclaude`
  - `tmux 会话: 0`
  - 运行中任务：1

### 2. 升级方式

没有走远端下载，而是直接使用本机 release 归档缓存：

```bash
tmpdir=$(mktemp -d /tmp/ccclaw-local-upgrade.XXXXXX)
tar -C "$tmpdir" -xzf /ops/logs/ccclaw/26.03.15.0746/ccclaw_26.03.15.0746_linux_amd64.tar.gz
"$tmpdir/ccclaw_26.03.15.0746_linux_amd64/install.sh" \
  --yes \
  --app-dir /home/zoomq/.ccclaw \
  --home-repo /opt/data/9527
```

升级结果：

- 程序版本切到 `26.03.15.0746`
- user systemd 自动执行了 `daemon-reload` 与受管 timer 重启
- 没有改写 shell 集成
- 普通配置继续使用 `/home/zoomq/.ccclaw/ops/config/config.toml`
- 敏感配置继续使用 `/home/zoomq/.ccclaw/.env`

### 3. 升级时新观察

- 安装脚本在 `/opt/data/9527` 新增了一次 `seed ccclaw home repo` 提交
- `git -C /opt/data/9527 status --short --branch` 显示：
  - `## main...origin/main [ahead 3]`

这说明当前知识仓库在升级过程中仍会自动写入受管内容；虽然没有直接覆盖用户记忆结论，但会形成新的本地提交，需要后续进一步界定是否符合“仅无损刷新受管区块”的预期。

## 升级后核验

### 1. 版本与工具链

- `ccclaw -V`：`26.03.15.0746`
- `/home/zoomq/.ccclaw/bin/ccclaw -V`：`26.03.15.0746`
- `claude --version`：`2.1.76 (Claude Code)`
- `rtk --version`：`rtk 0.27.2`

### 2. 调度状态

- `ccclaw scheduler status`
  - `request=systemd effective=systemd`
- `ccclaw scheduler timers --wide`
  - `ingest/patrol/journal/archive/sevolver` 五类 timer 均为 `active enabled`
  - 时区解释符合配置 `Asia/Shanghai`

### 3. 运行态状态

`ccclaw status` 关键事实：

- 默认模式：`daemon (daemon_targets=1 tmux_targets=0)`
- `tmux 托管任务: 0`
- `daemon 任务: 1`
- `tmux 状态: 当前无 tmux 托管任务（默认 daemon 模式）`
- `Sevolver` 聚合视图已在 `status` 中直接展示

这表明当前版本对外展示的主执行链路已经是：

1. 默认执行器为 `daemon`
2. 运行态观测口径以 `status/stats/storage` 聚合为准
3. `tmux` 已不是默认执行载体

### 4. 健康体检

- 执行：`ccclaw doctor`
- 结果：18 项检查全部通过
- 其中 tmux 检查结果为：
  - `当前无 tmux 托管仓库（tmux 仅作为 debug attach）`

这与 `phase3` 的架构基线一致。

### 5. systemd 单元现状

`systemctl --user list-unit-files 'ccclaw*'` 显示：

- 已安装：
  - `ccclaw-archive.timer`
  - `ccclaw-ingest.timer`
  - `ccclaw-journal.timer`
  - `ccclaw-patrol.timer`
  - `ccclaw-run.timer`
  - `ccclaw-sevolver.timer`

`systemctl --user list-units 'ccclaw*'` 显示：

- timer 为 active
- `ccclaw-ingest.service`
- `ccclaw-patrol.service`
- `ccclaw-run.service`
  以上 3 个 service 当前显示为 `failed`

这里需要注意两点：

1. `scheduler status` 只把 `ingest/patrol/journal/archive/sevolver` 作为受管事实口径，并未把 `run.timer` 列入原因串。
2. `run.timer/run.service` 仍残留在 systemd 中，说明旧单元尚未完全退出；这不是本轮升级失败，但属于需要继续收敛的运维残留。

## 关于“当前是否通过 Claude Code stdout NDJSON 运行时探查，而不是 tmux 终端复用”

结论：是，当前主链路已经是基于 Claude Code `stdout` 的 `stream-json`/NDJSON 事实流做运行时探查；`tmux` 不是默认执行内核。

依据：

1. 架构报告已明确：
  - `#46` 冻结 `stream-json` 事件契约
  - `#47` 在 tmux 载体下切到 `stream-json`
  - `#48` 引入 `daemon` 执行内核，按行解码 `stream-json`
  - `#49` 将默认模式切到 `daemon`，tmux 收缩为 debug attach/补偿
2. 当前主机 `ccclaw status` 实际输出：
  - 默认模式 `daemon`
  - `tmux 托管任务: 0`
  - `daemon 任务: 1`
3. 当前 `doctor` 对 tmux 的判断已是：
  - “当前无 tmux 托管仓库（tmux 仅作为 debug attach）”

因此，最新 CCClaw 的“Issue 启动 Claude 并持久健康运行”观察机制，应优先理解为：

- ingest 发现并放行 Issue
- daemon 执行器直接启动 `ccclaude -> claude`
- Claude 的 `stdout stream-json` 被逐行解码
- 运行态映射到 `RepoSlot / Task / state.db / status / stats`
- 最终同时沉淀：
  - `*.stream.jsonl`
  - `*.event.json`
  - 兼容 `*.json`
  - `*.meta.json`

而不是靠 `tmux pane` 是否存在来判断是否仍在运行。

## 建议人工测试路径

建议按以下最小链路做一次端到端人工验证：

1. 在控制仓库 `41490/ccclaw` 新建一个最小任务 Issue，并在正文里写清 `target_repo: 41490/ccclaw`
2. 由受信任成员评论 `/ccclaw approve`
3. 先看 `ccclaw status`
  - 预期：出现 `daemon 任务 > 0`
  - 预期：`tmux 托管任务` 仍为 `0`
4. 再看调度推进
  - `ccclaw scheduler logs patrol --level info`
  - `journalctl --user -u ccclaw-ingest.service -u ccclaw-patrol.service -n 200`
5. 再看结果产物
  - `ls ~/.ccclaw/var/results/`
  - 预期新增：
    - `*.stream.jsonl`
    - `*.event.json`
    - `*.json`
    - `*.meta.json`
6. 再看结构化状态
  - `ccclaw status --json`
  - 预期：任务经历 `RUNNING -> FINALIZING -> DONE/FAILED`
7. 仅在需要 debug attach 时，再对单仓显式配置 `executor_mode = "tmux"` 验证兼容路径

关键判读原则：

- 看 `status --json`、`scheduler status`、`scheduler logs`、`var/results/*`
- 不再把 “tmux 会话存在” 当成主健康信号

## 结论

本轮已完成以下闭环：

1. 正式发布 `26.03.15.0746`
2. 补发 `Announcements` 公告
3. 直接使用本机缓存成功升级当前主机
4. 复核确认当前默认执行模式已是 `daemon`
5. 复核确认当前运行态观测口径已转向 Claude Code `stdout stream-json`，不是默认依赖 `tmux`

但仍有两个后续点需要继续处理：

1. `ccclaw-run.timer` / `ccclaw-run.service` 旧单元残留
2. 知识仓库升级过程中自动新增 `seed ccclaw home repo` 提交的边界是否完全符合“仅无损刷新受管区块”
