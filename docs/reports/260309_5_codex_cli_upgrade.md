# 260309_5_codex_cli_upgrade

## 背景

2026-03-09 对当前执行环境中的 Codex CLI 做一次原地升级验证，目标是确认安装来源、升级路径与升级后版本状态是否正常。

## 本次处理

### 1. 安装来源确认

本机 `codex` 可执行文件位于：

- `/home/zoomq/.bun/bin/codex`

其实际包目录位于：

- `/home/zoomq/.bun/install/global/node_modules/@openai/codex`

说明当前 Codex CLI 通过 Bun 全局安装，包名为 `@openai/codex`。

### 2. 升级前状态

升级前版本：

- `codex-cli 0.111.0`

通过 npm registry 查询到当日最新版本：

- `@openai/codex 0.112.0`

### 3. 执行升级

执行命令：

```bash
bun add -g @openai/codex@latest
```

安装完成后，Bun 回报已安装：

- `@openai/codex@0.112.0`

### 4. 升级后验证

执行版本检查：

```bash
codex --version
```

结果为：

- `codex-cli 0.112.0`

说明升级成功，命令入口与全局包状态一致。

## 结论

- 当前 Codex CLI 已从 `0.111.0` 升级到 `0.112.0`
- 升级路径可固化为 Bun 全局升级命令
- 本次未改动仓库业务代码，仅补充工程记录

## 后续建议

- 若后续需要可将 Codex CLI 版本巡检纳入运维脚本
- 若要避免未来出现 Bun 与其他包管理器混装，建议统一约束 Codex CLI 仅保留一种安装来源
