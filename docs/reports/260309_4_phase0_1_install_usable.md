# 260309_4_phase0_1_install_usable

## 背景

承接 Issue #4，目标是把 `phase0` 从“骨架可跑”推进到“单机可安装、可初始化 Claude 环境、可维护本体仓库”的可用状态。

本轮依据已拍板决策：

- 程序目录：`~/.ccclaw`
- 本体仓库：`/opt/ccclaw`
- 本体仓库初始化：优先引导用户创建/接管一个封闭仓库，phase0.1 支持本地 `git init` 与已有仓库接管
- `rtk`：优先系统包/预编译，失败再源码/官方脚本；Claude 默认走 `rtk` 前缀语义
- Claude 官方能力：若本机已有 plugins 则继承，否则补装指定官方 plugins 与 `example-skills`
- 升级：按 release/program tree 升级，不自动覆盖本体仓库记忆内容

## 本次完成

### 1. 安装拓扑收口

明确并落地两层目录：

- 程序目录 `~/.ccclaw`
- 本体仓库 `/opt/ccclaw`

程序目录负责：

- `bin/ccclaw`
- `bin/ccclaude`
- `.env`
- `ops/config/config.toml`
- `ops/systemd/`
- `log/`
- `var/state.db`

本体仓库负责：

- `kb/designs`
- `kb/assay`
- `kb/journal`
- `kb/skills/L1`
- `kb/skills/L2`
- `docs/rfcs`
- `docs/plans`
- `docs/reports`

### 2. install.sh 重构

`src/dist/install.sh` 新增能力：

- `--simulate`：先文本模拟安装过程
- `--yes`：非交互默认安装
- `--skip-deps`：跳过依赖安装
- 环境探查：Claude、plugins、marketplaces、CLI 工具
- 依赖安装：`git` / `gh` / `rg` / `sqlite3` / `curl` / `wget`
- `rtk` 官方 quick install 接入
- 生成 `ccclaude` 包装器，默认 `rtk proxy claude`
- 生成 user-level systemd 单元
- 初始化或接管本体仓库
- 严格区分 `.env` 与 `config.toml`

### 3. Claude 生态接入

探查并利用本机已有 Claude 状态：

- 已安装 Claude 时自动读取本地 plugin / marketplace 现状
- 若本机已有 plugin，则按决策直接继承
- 若本机无 plugin，则补装：
  - `example-skills@anthropic-agent-skills`
  - 指定官方 plugins 集合
- 自动补 `anthropic-agent-skills` 与 `claude-plugins-official` marketplace
- 尝试执行 `rtk init --global`

### 4. 运行时配置增强

Go 运行时代码已支持：

- `paths.app_dir`
- `paths.home_repo`
- `executor.command = ["..."]`
- `~` 路径自动展开
- `doctor` 增加：`claude` / `rtk` / `rg` / `sqlite3` / `git` 检查
- `systemctl --user` 优先检查

### 5. CLAUDE.md 分层

新增分层指令文件：

- 仓库根 `CLAUDE.md`
- `src/CLAUDE.md`
- `src/internal/CLAUDE.md`
- `src/dist/CLAUDE.md`
- `docs/CLAUDE.md`

同时补充安装流程文档：

- `docs/plans/phase0_1_install_flow.md`
- `src/dist/ops/examples/install-flow.md`

## 测试与验证

执行：

```bash
cd src
bash -n dist/install.sh
bash -n dist/upgrade.sh
go test ./...
make build
./dist/install.sh --simulate --yes
./dist/bin/ccclaw config --config ./ops/config/config.example.toml --env-file <temp_env>
```

结果：通过。

## 当前已知边界

- 若本机完全没有 Claude Code，`install.sh` 目前会提示并跳过插件初始化，不会替用户猜测安装来源
- `rtk proxy claude` 的运行效果依赖本机 `rtk` 版本与 Claude 配置状态
- `doctor` 会把 `rtk/sqlite3` 缺失视为异常，符合本期“必须可用”的要求
- 目前首次安装只配置一个主 target repo，多 target 扩展留待后续

## 后续建议

- 为 `install.sh` 增加 `--install-claude` 明确模式，在官方安装方式可稳定确认后接入
- 为本体仓库增加 remote 绑定与私有仓库接管助手
- 为 `upgrade.sh` 增加 release/tag 校验与备份回滚
- 为多目标仓库增加交互式追加配置命令
- 为 plugin/skills 安装增加结果清单与重试机制
