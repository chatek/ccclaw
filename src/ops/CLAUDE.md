# CLAUDE.md — src/ops 作用域

本目录存放发布时会同步到 `src/dist/ops/` 的运维资产源。

## 约束

- `config.example.toml` 是普通配置样板源
- `systemd/` 是发布与安装时同步的单元文件源
- `scripts/` 是发布时一起交付的辅助脚本源
- 修改本目录后，应同步验证 `make dist-sync` 的结果
- `config.example.toml` 必须反映当前默认值，尤其是 `executor.mode=daemon`、调度时区、审批词与日志归档策略
- `systemd/` 与安装脚本描述必须覆盖 `ingest/patrol/journal/archive/sevolver` 五类受管单元
- `examples/` 中的安装、升级、观察、排障示例必须与当前命令面一致，不能继续把 `tmux` 描述为默认执行器
