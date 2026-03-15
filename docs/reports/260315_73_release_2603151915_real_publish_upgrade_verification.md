# 260315_73_release_2603151915_real_publish_upgrade_verification

## 背景

- 对应 Issue：[#73](https://github.com/41490/ccclaw/issues/73)
- 目标：
  1. 基于 `fix(#73): unify release artifact flow` 做一次真实 release
  2. 使用本地 release 缓存包升级当前主机
  3. 验证“本地缓存包与 GitHub release 资产严格一致”这一修复目标
  4. 回帖 Issue 说明真实发布与升级结果

## 发布前状态

### 1. 仓库与版本

- 当前分支：`main...origin/main`
- 工作树：干净
- 发布 commit：`4d8f49dd014930631dc3f13b5a035418a263507a`
- 升级前主机版本：
  - `ccclaw -V`：`26.03.15.1721`
  - `/home/zoomq/.ccclaw/bin/ccclaw -V`：`26.03.15.1721`

### 2. 发布前回归

执行：

```bash
cd /opt/src/ccclaw/src
make test
make release-preflight VERSION=26.03.15.1914
```

结果：

- `go test ./...` 全部通过
- `release-preflight` 通过

说明：

- 实际执行 `make release` 时，版本号按北京时间滚动到 `26.03.15.1915`
- 后续所有核对均以真实发布版本 `26.03.15.1915` 为准

## 真实发布

执行：

```bash
cd /opt/src/ccclaw/src
make release VERSION=26.03.15.1915
```

结果：

- release URL：<https://github.com/41490/ccclaw/releases/tag/26.03.15.1915>
- tag：`26.03.15.1915`
- target commit：`4d8f49dd014930631dc3f13b5a035418a263507a`
- 发布时间（UTC）：`2026-03-15T11:15:07Z`

GitHub release 资产：

- `ccclaw_26.03.15.1915_linux_amd64.tar.gz`
  - digest：`sha256:f99a00f6142f5bb07040efef276afc45c560da5fe4dd9af95fcd1f8b689b6e8d`
  - size：`7043283`
- `SHA256SUMS`
  - digest：`sha256:500a4963827e1eb8c8a38773c5cc17d97d81c6492ab777fa860360e865490d8c`

## 关键修复目标验证：本地缓存与 GitHub 资产一致

### 1. 本地缓存校验

执行：

```bash
cd /ops/logs/ccclaw/26.03.15.1915
sha256sum -c SHA256SUMS
sha256sum ccclaw_26.03.15.1915_linux_amd64.tar.gz SHA256SUMS
```

结果：

- `ccclaw_26.03.15.1915_linux_amd64.tar.gz: OK`
- 本地缓存包 digest：`f99a00f6142f5bb07040efef276afc45c560da5fe4dd9af95fcd1f8b689b6e8d`
- 本地缓存 `SHA256SUMS` digest：`500a4963827e1eb8c8a38773c5cc17d97d81c6492ab777fa860360e865490d8c`

### 2. 从 GitHub release 下载回验

执行：

```bash
gh release download 26.03.15.1915 --repo 41490/ccclaw \
  --pattern 'ccclaw_26.03.15.1915_linux_amd64.tar.gz' \
  --pattern 'SHA256SUMS'
cmp -s <downloaded package> /ops/logs/ccclaw/26.03.15.1915/ccclaw_26.03.15.1915_linux_amd64.tar.gz
cmp -s <downloaded SHA256SUMS> /ops/logs/ccclaw/26.03.15.1915/SHA256SUMS
sha256sum -c SHA256SUMS
```

结果：

- `package_equal=yes`
- `checksum_equal=yes`
- 下载回来的 package digest：`f99a00f6142f5bb07040efef276afc45c560da5fe4dd9af95fcd1f8b689b6e8d`
- 下载回来的 `SHA256SUMS` digest：`500a4963827e1eb8c8a38773c5cc17d97d81c6492ab777fa860360e865490d8c`
- `ccclaw_26.03.15.1915_linux_amd64.tar.gz: OK`

### 3. 结论

Issue #73 的 Phase 1 修复已通过真实发布验证：

1. `/ops/logs/ccclaw/26.03.15.1915/` 中缓存包
2. GitHub release 下载回来的资产
3. `.release/` 中上传源资产

三者现已对齐为同一份 artifact 语义。

## 使用本地缓存升级当前主机

执行：

```bash
tmpdir=$(mktemp -d /tmp/ccclaw-local-upgrade-1915.XXXXXX)
tar -C "$tmpdir" -xzf /ops/logs/ccclaw/26.03.15.1915/ccclaw_26.03.15.1915_linux_amd64.tar.gz
CCCLAW_VERSION=26.03.15.1915 \
  "$tmpdir/ccclaw_26.03.15.1915_linux_amd64/install.sh" \
  --yes \
  --app-dir /home/zoomq/.ccclaw \
  --home-repo /opt/data/9527 \
  --upgrade-home-repo
```

升级器输出：

- 版本：`26.03.15.1915`
- 本体仓库模式：`upgrade-refresh`
- 调度器：`request=systemd, effective=systemd`
- 自动完成 `daemon-reload` 与 timer 重启

## 升级后关键验收

### 1. 版本与工具链

- `ccclaw -V`：`26.03.15.1915`
- `/home/zoomq/.ccclaw/bin/ccclaw -V`：`26.03.15.1915`
- `claude --version`：`2.1.76 (Claude Code)`
- `rtk --version`：`rtk 0.27.2`

### 2. 调度与 systemd

- `ccclaw scheduler status`
  - `request=systemd effective=systemd`
- `ccclaw scheduler timers --wide`
  - `ingest/patrol/journal/archive/sevolver` 五类 timer 均为 `active enabled`
- `systemctl --user list-unit-files 'ccclaw*'`
  - 仅剩 10 个目标 unit
  - 不再出现 `ccclaw-run.*`

### 3. 健康体检

- `ccclaw doctor`
  - `18` 项检查全部通过

### 4. 运行态状态

`ccclaw status` 关键事实：

- 默认模式：`daemon`
- `tmux 托管任务: 0`
- `daemon 任务: 0`
- 当前任务概览：
  - `BLOCKED=7`
  - `DEAD=5`

### 5. 本体仓库边界

- `git -C /opt/data/9527 status --short --branch`
  - 仍为 `## main...origin/main [ahead 3]`
- 升级后没有新增本地 seed commit

这说明本轮升级没有破坏已有 `home_repo` 边界。

## 新观察到的残留问题

本轮真实发布链路已经证明 Issue #73 的 artifact 分叉问题被修复，但运行态仍暴露了一个新的独立问题：

- `ccclaw status` 中可见 `#72` 已变为 `DEAD`
- 最近异常为：
  - `解析 Claude JSON 输出失败: stream-json 解码失败: 无法识别事件类型`
- 诊断片段显示新的 hook 事件：
  - `type=system`
  - `subtype=hook_started`

这表明当前 `stream-json` 解码链路对 Claude 新增的 `hook_started` 系统事件还未完全兼容。

结论：

- 这不是 Issue #73 的 artifact 一致性回归
- 它是新的执行期兼容问题，应独立跟踪

## 结论

本轮已完成：

1. 将 `fix(#73)` 的修复作为真实 release `26.03.15.1915` 发布
2. 证明本地缓存包与 GitHub release 资产严格一致
3. 使用本地缓存包正确升级当前主机到 `26.03.15.1915`
4. 完成版本、调度、systemd、doctor、status 等关键验收

Issue #73 在其 Phase 1 范围内已通过真实发布验证。
