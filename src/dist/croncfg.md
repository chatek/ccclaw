# ccclaw cron 专家手工配置说明

本文件说明 `ccclaw` 的 `cron` 收缩策略，以及在必须使用 `cron` 时的专家手工配置方式。

若你的程序目录不是默认 `~/.ccclaw`，请把下文中的 `~/.ccclaw` 替换成实际安装目录。

## 1. CCClaw 自动使用的调度主路径

- `install.sh` / `upgrade.sh` 只自动托管 `systemd --user`
- `--scheduler auto` 只会优先选择 `systemd --user`
- 当 `systemd --user` 不可用时，安装器会进入 `none + 手工 cron 指引`
- 安装器不会自动写入、刷新或接管当前用户 `crontab`
- 如需清理历史托管块，请显式执行 `scheduler disable-cron` 或 `install.sh --remove-cron`

这意味着：

- `cron` 仍被支持，但只保留为专家手工备选
- 日常安装、升级、回归与发布说明都以 `systemd --user` 为主路径
- 不要同时保留 `systemd` timer 和 `cron` 规则，否则会产生重复执行

## 2. 何时才应该手工启用 cron

仅在以下情况使用：

- 当前主机确实无法稳定使用 `systemd --user`
- 你明确接受 `cron` 仅作为例外链路，不再属于安装器主支持面
- 你能自行复核 `crontab` 内容，并承担后续维护责任

若 `systemd --user` 可用，优先执行：

```bash
~/.ccclaw/bin/ccclaw scheduler use systemd --config ~/.ccclaw/ops/config/config.toml
```

## 3. 专家手工配置方式

### 3.1 使用 CLI 专家工具

这是最快的专家入口，仍会写入受控块：

```bash
~/.ccclaw/bin/ccclaw --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env scheduler enable-cron
```

清理受控块：

```bash
~/.ccclaw/bin/ccclaw --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env scheduler disable-cron
```

### 3.2 直接手工写入 crontab

如果你不想使用 CLI 专家工具，可手工写入以下样板：

```cron
# >>> ccclaw managed cron >>>
*/4 * * * * ~/.ccclaw/bin/ccclaw ingest --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env
*/2 * * * * ~/.ccclaw/bin/ccclaw patrol --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env
50 23 * * * ~/.ccclaw/bin/ccclaw journal --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env
# <<< ccclaw managed cron <<<
```

如程序目录不是 `~/.ccclaw`，请把路径整体替换为实际安装目录。

## 4. 复核步骤

写入后至少执行以下检查：

```bash
crontab -l
~/.ccclaw/bin/ccclaw scheduler status --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env
~/.ccclaw/bin/ccclaw scheduler doctor --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env
```

重点确认：

- `crontab -l` 中只存在一份 `ccclaw managed cron` 块
- `scheduler status` 不再同时看到 `systemd+cron`
- `doctor` 没有提示双重调度或规则缺失

## 5. 回切到 systemd

如果后续环境恢复为可用 `systemd --user`，请按以下顺序回切：

```bash
~/.ccclaw/bin/ccclaw --config ~/.ccclaw/ops/config/config.toml --env-file ~/.ccclaw/.env scheduler disable-cron
systemctl --user daemon-reload
systemctl --user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer
```

## 6. 风险提醒

- 不要同时手工维护 `cron` 与 `systemd` 两套调度
- 不要把 `cron` 当作安装器默认能力；它已经被收缩为专家手工备选
- 升级后若需要继续保留 `cron`，请主动复核本文件，不要假定安装器会代为刷新规则
