# CLAUDE.md — src/cmd 作用域

本目录只放 CLI 入口与子命令装配。

## 约束

- `ccclaw` 无参数默认显示帮助
- `ccclaw -h|--help` 显示帮助
- `ccclaw -V|--version` 与 `ccclaw version` 显示构建注入版本
- 不在入口层硬编码业务状态，业务逻辑应下沉到 `internal/app`
