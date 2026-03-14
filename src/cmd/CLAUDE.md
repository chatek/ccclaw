# CLAUDE.md — src/cmd 作用域

本目录只放 CLI 入口与子命令装配。

## 约束

- `ccclaw` 无参数默认显示帮助
- `ccclaw -h|--help` 显示帮助
- `ccclaw -V|--version` 与 `ccclaw version` 显示构建注入版本
- 不在入口层硬编码业务状态，业务逻辑应下沉到 `internal/app`
- `status`、`stats`、`scheduler`、`sevolver` 等命令只负责参数装配与输出切换，不自行维护第二套聚合口径
- 人类可读输出与 JSON 输出必须并存；新增字段时优先向后兼容，不破坏既有脚本消费路径
- 命令文案需要体现当前语义：默认 `daemon`，`tmux` 为附着/补偿，不再暗示 tmux 是主执行模式
