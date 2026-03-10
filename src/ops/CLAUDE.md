# CLAUDE.md — src/ops 作用域

本目录存放发布时会同步到 `src/dist/ops/` 的运维资产源。

## 约束

- `config.example.toml` 是普通配置样板源
- `systemd/` 是发布与安装时同步的单元文件源
- `scripts/` 是发布时一起交付的辅助脚本源
- 修改本目录后，应同步验证 `make dist-sync` 的结果
