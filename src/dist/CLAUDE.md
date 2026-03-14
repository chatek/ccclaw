# CLAUDE.md — src/dist 作用域

本目录是发布树与安装入口源。

## 约束

- `install.sh` 必须先探查环境再决定交互
- 优先复用本机现有 Claude 配置，不无故覆盖
- 默认安装路径是 `~/.ccclaw`
- 默认本体仓库路径是 `/opt/ccclaw`
- 升级脚本不能覆盖本体仓库中的用户记忆；仅允许无损刷新关键 `kb/**/CLAUDE.md`
- release 必须从本目录打包，且附带安装包与 `SHA256SUMS`
- 安装脚本必须明确区分本体仓库 `init|remote|local` 与任务仓库 `remote|local` 绑定模式
- `kb/**/CLAUDE.md` 必须使用受管区块与用户区块，保证后续升级可无损合并
- 安装脚本在可直连 user bus 时应自动启用/重启托管 timer；不可直连时再退回手工命令提示
- 安装完成后必须打印部署成果、触发方式、日常使用流程与开源协作流程
- `jj.md` 必须随安装同步到 `~/.ccclaw/jj.md`，作为本机仓库同步速查表
- 安装与升级输出必须明确“默认执行器是 daemon，tmux 仅用于 debug attach/补偿”
- 发布树文档必须覆盖 `stream-json` 产物链路、`status/stats` 观察入口、`sevolver` 自维护链路与五类受管 timer
- `upgrade.sh` 固定走官方 release 下载与 `SHA256SUMS` 校验；若 user bus 不可直连，必须打印绝对准确的手工 `systemctl --user` 补救命令
