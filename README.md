# ccclaw

> Claude Code + GitHub Issues + systemd + kb memory

`ccclaw` 是一个以 GitHub Issue 为异步任务入口的长期执行系统。

当前仓库处于 `phase0/phase0.1`：

- `phase0` 完成最小闭环骨架
- `phase0.1` 继续把安装、工具链、Claude 生态接入与本体仓库初始化推进到可用

## 当前拓扑

```text
程序目录: ~/.ccclaw
├── bin/
│   ├── ccclaw
│   └── ccclaude          # 默认通过 rtk proxy 包装 claude
├── .env                  # 仅放敏感配置
├── log/
├── var/
└── ops/
    ├── config/config.toml
    ├── systemd/
    └── scripts/

本体仓库: /opt/ccclaw
├── kb/
│   ├── designs/
│   ├── assay/
│   ├── journal/
│   └── skills/
│       ├── L1/
│       └── L2/
└── docs/
    ├── rfcs/
    ├── plans/
    └── reports/
```

## 关键决策

- 程序目录与本体仓库目录分离
- 程序默认安装到 `~/.ccclaw`
- 本体记忆仓库默认在 `/opt/ccclaw`
- 运行时敏感配置固定落在 `~/.ccclaw/.env`
- 一般配置固定落在 `~/.ccclaw/ops/config/config.toml`
- Claude 执行默认通过 `rtk proxy claude` 包装
- 若本机已存在 Claude plugins，则直接继承；否则补装指定官方集合
- 升级只覆盖程序发布树，不自动覆盖本体仓库内容

## 快速开始

```bash
cd src
make test
make build
./dist/install.sh --simulate
```

## 安装

```bash
cd src
make build
./dist/install.sh
```

安装脚本会：

- 探查当前 `claude` / plugin / marketplace / CLI 工具环境
- 尽量自动复用现有 Claude 配置
- 安装 `sqlite3`、`rtk` 与基础 CLI 工具
- 初始化本体仓库目录树与 Git 仓库
- 生成 `.env` 与 `config.toml`
- 生成 user-level systemd units

## 常用命令

```bash
~/.ccclaw/bin/ccclaw config
~/.ccclaw/bin/ccclaw doctor
~/.ccclaw/bin/ccclaw ingest
~/.ccclaw/bin/ccclaw run
~/.ccclaw/bin/ccclaw status
```

## 工程报告

工程报告统一落在 `docs/reports/`，文件命名约定：

```text
yymmdd_[Issue No.]_[Case Summary].md
```
