# CLAUDE.md — src 作用域

本目录只放源码、构建与发布资产。

## 约束

- 所有 Go 代码保持生产级错误处理
- 安装脚本优先探查再写入，不先破坏现有配置
- 默认围绕 `~/.ccclaw` 程序目录与 `/opt/ccclaw` 本体仓库实现
- 与 Claude/rtk/插件生态相关的接入逻辑优先写在 `dist/` 与 `ops/`
- `Makefile` 负责统一执行 build/package/release 流程
- 任何 release 资产都应先同步到 `dist/`，再打包
