# ccclaw

> Claude Code + GitHub Issues + systemd + kb memory

`ccclaw` 是一个以 GitHub Issue 为异步任务入口的长期执行系统。

本仓库当前进入 `phase0`：先把“可安装、可配置、可审计、可持续维护自身仓库”的最小可用闭环落稳。

## 当前目录

```text
ccclaw/
├── CLAUDE.md
├── AGENTS.md
├── LICENSE
├── README.md
├── docs/
│   ├── rfcs/
│   ├── plans/
│   └── reports/
└── src/
    ├── Makefile
    ├── go.mod
    ├── dist/
    ├── cmd/
    ├── internal/
    └── ops/
```

## phase0 目标

- 单一 `ccclaw` CLI：`ingest / run / status / doctor / config`
- 状态持久化切换为 SQLite，并记录事件表
- 所有非敏感配置统一走 `.toml`
- 所有敏感配置统一走固定 `.env` 文件，并做权限检查
- 默认部署方式保持 `systemd`
- 仅自动执行管理员 Issue；普通用户 Issue 需管理员评论 `/ccclaw approve`

## 快速开始

```bash
cd src
make test
make build
```

构建产物默认输出到 `src/dist/bin/ccclaw`。

## 安装部署

参考：

- `src/dist/install.sh`
- `src/dist/ops/config/config.example.toml`
- `src/dist/.env.example`

典型流程：

```bash
cd src
make build
./dist/install.sh
```

## 配置约定

- 敏感项：`/opt/ccclaw/.env`
- 普通配置：`/opt/ccclaw/ops/config/config.toml`
- 知识库：`/opt/ccclaw/kb/`
- 状态数据库：`/var/lib/ccclaw/state.db`

## 核心门禁

- 管理员本人发起的 Issue：自动进入可执行队列
- 普通用户发起的 Issue：只有管理员评论 `/ccclaw approve` 后才进入可执行队列
- 管理员身份：运行时通过 GitHub 协作权限动态判断

## 常用命令

```bash
cd src
./dist/bin/ccclaw config --config ./ops/config/config.example.toml --env-file ./dist/.env.example
./dist/bin/ccclaw doctor --config ./ops/config/config.example.toml --env-file ./dist/.env.example
./dist/bin/ccclaw ingest --config ./ops/config/config.example.toml --env-file ./dist/.env
./dist/bin/ccclaw run --config ./ops/config/config.example.toml --env-file ./dist/.env
./dist/bin/ccclaw status --config ./ops/config/config.example.toml --env-file ./dist/.env
```

## 工程报告

本仓库实施报告统一落在 `docs/reports/`，文件命名约定：

```text
yymmdd_[Issue No.]_[Case Summary].md
```
