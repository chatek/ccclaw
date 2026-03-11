# 260311_28_local_deployed_upgrade_validation

## 背景

在 Issue #28 完成实现后，继续对当前开发机上已经部署的旧版 `ccclaw` 做一次真实升级验证，目标是确认：

1. 当前旧安装是否能被新 release 正常带起
2. 新版 `upgrade.sh` 在这种“历史安装 + 历史配置”的状态下是否仍能跑通完整控制流

由于 `26.03.11.1227` 仅做了本地 release 打包，尚未发布到 GitHub release，因此第二段验证采用：

- 实际执行已安装的 `~/.ccclaw/upgrade.sh`
- 但通过 fake `gh` 向其提供本地构建好的 `26.03.11.1227` release 资产

这样可以验证真实升级脚本逻辑本身，而不依赖线上 release 是否已发布。

## 验证前现场状态

执行：

```bash
~/.ccclaw/bin/ccclaw -V
```

结果：

```text
26.03.11.0440
```

现场 `~/.ccclaw/ops/config/config.toml` 的关键状态：

- `paths.home_repo = '/opt/data/9527'`
- `scheduler.mode = 'systemd'`
- 仅有 `scheduler.systemd_user_dir`
- 缺少：
  - `scheduler.calendar_timezone`
  - `[scheduler.timers]`
  - `[scheduler.logs]`

这正是用户最初报告的历史问题现场。

## 验证一：用本地 26.03.11.1227 release 直接升级真实安装

执行方式：

1. 解压本地 release 包
2. 直接调用解压树中的 `install.sh`
3. 目标仍指向真实安装目录 `~/.ccclaw`
4. 显式传入当前现场 `home_repo=/opt/data/9527`

执行命令：

```bash
tar -C /tmp/ccclaw-verify-28/release -xzf /ops/logs/ccclaw/26.03.11.1227/ccclaw_26.03.11.1227_linux_amd64.tar.gz
/tmp/ccclaw-verify-28/release/ccclaw_26.03.11.1227_linux_amd64/install.sh \
  --yes \
  --skip-deps \
  --app-dir "$HOME/.ccclaw" \
  --home-repo "/opt/data/9527"
```

结果：

- 退出码：`0`
- `~/.ccclaw/bin/ccclaw -V` 变为 `26.03.11.1227`

升级后配置确认：

- `home_repo` 仍为 `/opt/data/9527`
- `github.control_repo` 被固定为 `41490/ccclaw`
- 已自动补齐：
  - `calendar_timezone = "Asia/Shanghai"`
  - `[scheduler.timers]`
  - `[scheduler.logs]`

结论：

- 当前历史安装可被新 release 正常升级
- 用户最初报告的“版本不升级”和“新配置段不补齐”两个问题，在这一步已经收口

## 验证二：执行已安装的新 `~/.ccclaw/upgrade.sh`

目的：

- 不再只验证“新 release 的 install.sh 能否带起旧安装”
- 而是验证“已经安装到本机的新 `upgrade.sh`”本身能否跑通完整升级控制流

执行方式：

- 在 `PATH` 前放入 fake `gh`
- fake `gh` 返回 tag：`26.03.11.1227`
- fake `gh release download` 提供本地 release 资产：
  - `ccclaw_26.03.11.1227_linux_amd64.tar.gz`
  - `SHA256SUMS`

然后直接执行：

```bash
~/.ccclaw/upgrade.sh
```

结果：

- 退出码：`0`
- 升级日志明确显示：
  - 读取当前 `app_dir=/home/zoomq/.ccclaw`
  - 读取当前 `home_repo=/opt/data/9527`
  - 下载官方 release 资产
  - 校验 `SHA256SUMS`
  - 调用临时 release tree 中的 `install.sh`
- 最终版本保持为：`26.03.11.1227`

关键日志摘录：

```text
- 当前程序目录: /home/zoomq/.ccclaw
- 当前本体仓库: /opt/data/9527
[ccclaw-upgrade] 下载官方 release: repo=41490/ccclaw tag=26.03.11.1227 asset=ccclaw_26.03.11.1227_linux_amd64.tar.gz
[ccclaw-upgrade] 校验 SHA256SUMS
ccclaw_26.03.11.1227_linux_amd64.tar.gz: OK
```

fake `gh` 日志也与预期一致：

```text
release view --repo 41490/ccclaw --json tagName --jq .tagName
release download 26.03.11.1227 --repo 41490/ccclaw --dir ... --pattern ccclaw_26.03.11.1227_linux_amd64.tar.gz --pattern SHA256SUMS
```

结论：

- 新 `upgrade.sh` 的核心控制流在本机真实安装目录上可以跑通
- 不会再回到“调用本地旧 install.sh 自复制”的失败模式
- 会正确继承当前现场的 `/opt/data/9527`

## 兼容性结论

对“当前这种状态”的结论是：

### 兼容，且已收口原问题

具体体现在：

1. 旧版 `26.03.11.0440` 可以被本地新 release 升级到 `26.03.11.1227`
2. 旧配置里缺失的 scheduler 新段会在升级时自动补齐
3. `home_repo=/opt/data/9527` 不会被错误改回 `/opt/ccclaw`
4. 新版 `upgrade.sh` 的真实升级控制流可在该安装目录上稳定执行

## 新发现的残留问题

本轮验证中发现一个非功能性残留：

- 迁移后的 `config.toml` 在 `[scheduler]` 段前出现了一组重复的 scheduler 注释

表现：

- 配置可正常加载
- 版本升级与调度行为均正常
- 属于注释重复，不影响功能

推断原因：

- 当前 scheduler 段重写时，替换范围从 `[scheduler]` header 开始
- 旧配置里紧邻其上的注释未被吸收
- 新渲染出的注释又再次写入，因而出现重复

该问题不影响本轮兼容性结论，但建议后续单独收一轮配置注释清理。

## 产出

本次验证过程中生成的临时日志位于：

- `/tmp/ccclaw-verify-28/install-upgrade.log`
- `/tmp/ccclaw-verify-28/upgrade-runtime.log`
- `/tmp/ccclaw-verify-28/upgrade-gh.log`

这些日志用于本轮现场核验，不纳入版本控制。
