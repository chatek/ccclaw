# ccclaw 程序目录

<!-- ccclaw:managed:start -->
本目录是当前用户的 `ccclaw` 程序安装树。

## 常用命令

- 体检：`~/.ccclaw/bin/ccclaw doctor`
- 运行态快照：`~/.ccclaw/bin/ccclaw status`
- 调度状态：`~/.ccclaw/bin/ccclaw scheduler status`
- 调度专用体检：`~/.ccclaw/bin/ccclaw scheduler doctor`
- 定时器列表：`~/.ccclaw/bin/ccclaw scheduler timers`
- 追随全部调度日志：`~/.ccclaw/bin/ccclaw scheduler logs -f`
- 只看 ingest 日志：`~/.ccclaw/bin/ccclaw scheduler logs ingest --lines 100`
- 归档 warning 及以上日志：`~/.ccclaw/bin/ccclaw scheduler logs all --level warning --archive`
- 升级程序：`bash ~/.ccclaw/upgrade.sh`
  - 固定从 `41490/ccclaw` 最新官方 release 下载、校验并覆盖程序树

## systemd 运维

- 当前托管单元：
  - `ccclaw-ingest.timer`
  - `ccclaw-run.timer`
  - `ccclaw-patrol.timer`
  - `ccclaw-journal.timer`
  - `ccclaw-archive.timer`
  - `ccclaw-sevolver.timer`
- 若当前会话可直连 user bus，安装与升级会自动执行 `daemon-reload` 并启用/重启这些 timer
- 若当前会话无法直连 user bus，请在登录会话中手工执行：

```bash
systemctl --user daemon-reload
systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer
```

## 日志观察

- 追随全部服务：

```bash
~/.ccclaw/bin/ccclaw scheduler logs -f
```

- 指定范围：

```bash
~/.ccclaw/bin/ccclaw scheduler logs ingest --since "1 hour ago"
~/.ccclaw/bin/ccclaw scheduler logs patrol --lines 200
```

- 仅看 warning 及以上，并落盘归档：

```bash
~/.ccclaw/bin/ccclaw scheduler logs all --level warning --archive
```

- 归档目录默认读取 `scheduler.logs.archive_dir`
- 默认级别读取 `scheduler.logs.level`
- 默认保留策略读取 `scheduler.logs.retention_days`、`max_files`、`compress`

## 配置说明

- 普通配置：`~/.ccclaw/ops/config/config.toml`
- 敏感信息：`~/.ccclaw/.env`
- `github.control_repo` 固定为官方控制仓库 `41490/ccclaw`
- `github.limit` 表示 ingest 每轮拉取匹配标签的 open issues 上限，不是并发数
- `scheduler.calendar_timezone` 与 `[scheduler.timers]` 只影响 systemd timer 生成
- `[scheduler.logs]` 控制 `scheduler logs` 的默认级别过滤、归档目录与受管保留策略

## 升级约定

- 升级会刷新本文件的受管区块
- 本机长期补充请写到保留区块，升级会保留
<!-- ccclaw:managed:end -->

<!-- ccclaw:user:start -->
<!-- 本区块留给本机人工补充；升级会保留这里的内容。 -->
<!-- ccclaw:user:end -->
