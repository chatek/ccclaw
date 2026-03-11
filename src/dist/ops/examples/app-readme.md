# ccclaw 程序目录

<!-- ccclaw:managed:start -->
本目录是当前用户的 `ccclaw` 程序安装树。

## 常用命令

- 体检：`~/.ccclaw/bin/ccclaw doctor`
- 运行态快照：`~/.ccclaw/bin/ccclaw status`
- 调度状态：`~/.ccclaw/bin/ccclaw scheduler status`
- 定时器列表：`~/.ccclaw/bin/ccclaw scheduler timers`
- 追随全部调度日志：`~/.ccclaw/bin/ccclaw scheduler logs -f`
- 只看 ingest 日志：`~/.ccclaw/bin/ccclaw scheduler logs ingest --lines 100`
- 升级程序：`bash ~/.ccclaw/upgrade.sh`

## systemd 运维

- 当前托管单元：
  - `ccclaw-ingest.timer`
  - `ccclaw-run.timer`
  - `ccclaw-patrol.timer`
  - `ccclaw-journal.timer`
- 若当前会话可直连 user bus，安装与升级会自动执行 `daemon-reload` 并启用/重启这些 timer
- 若当前会话无法直连 user bus，请在登录会话中手工执行：

```bash
systemctl --user daemon-reload
systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer
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

## 配置说明

- 普通配置：`~/.ccclaw/ops/config/config.toml`
- 敏感信息：`~/.ccclaw/.env`
- `github.limit` 表示 ingest 每轮拉取匹配标签的 open issues 上限，不是并发数
- `scheduler.calendar_timezone` 与 `[scheduler.timers]` 只影响 systemd timer 生成

## 升级约定

- 升级会刷新本文件的受管区块
- 本机长期补充请写到保留区块，升级会保留
<!-- ccclaw:managed:end -->

<!-- ccclaw:user:start -->
<!-- 本区块留给本机人工补充；升级会保留这里的内容。 -->
<!-- ccclaw:user:end -->
