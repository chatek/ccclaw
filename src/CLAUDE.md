# CLAUDE.md — src 作用域

本目录只放源码、构建与发布资产。

## 约束

- 所有 Go 代码保持生产级错误处理
- 安装脚本优先探查再写入，不先破坏现有配置
- 默认围绕 `~/.ccclaw` 程序目录与 `/opt/ccclaw` 本体仓库实现
- 与 Claude/rtk/插件生态相关的接入逻辑优先写在 `dist/` 与 `ops/`
- `Makefile` 负责统一执行 build/package/release 流程
- 任何 release 资产都应先同步到 `dist/`，再打包
- 当前执行内核默认是 `daemon`，`tmux` 相关实现只允许承担兼容、附着与补偿职责
- 执行产物基线是 `stream-json`，涉及结果落盘时必须同时考虑 `*.stream.jsonl`、`*.event.json` 与兼容结果
- 运行态聚合口径优先下沉到 `internal/adapters/storage`，避免 `cmd`、`app`、`scheduler` 各自复制统计逻辑
