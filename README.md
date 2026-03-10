# ccclaw

> Claude Code + GitHub Issues + systemd + kb memory

`ccclaw` 是一个以 GitHub Issue 为异步任务入口的长期执行系统。

当前仓库处于 `phase0/phase0.2`：

- `phase0` 完成最小闭环骨架
- `phase0.1` 把安装、工具链、Claude 生态接入与本体仓库初始化推进到可用
- `phase0.2` 补齐零 target 安装、target 路由、admin 门禁与 Claude 巡检

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

## 最简安装建议

```bash
cd src
make build
./dist/install.sh
```

最小可用路径：

```bash
~/.ccclaw/bin/ccclaw doctor
```

如果 `doctor` 报 `claude` 缺失，并且你需要非交互安装，可显式执行：

```bash
cd src
./dist/install.sh --yes --install-claude
```

Claude 授权只建议两条路径：

- `claude setup-token`
- 代理模式，在 `~/.ccclaw/.env` 中填写 `ANTHROPIC_BASE_URL` 与 `ANTHROPIC_AUTH_TOKEN`

安装完成后，先绑定目标仓库，再启用定时器：

```bash
~/.ccclaw/bin/ccclaw target add \
  --config ~/.ccclaw/ops/config/config.toml \
  --repo owner/repo \
  --path /abs/path/to/repo \
  --default

systemctl --user daemon-reload
systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer
```

## 安装说明

安装脚本当前会：

- 探查当前 `claude` / plugin / marketplace / CLI 工具环境
- 探查 Claude 官方安装通道可达性
- 非交互模式仅在显式 `--install-claude` 时自动安装 Claude
- 安装 `sqlite3`、`rtk`、`golang` 与基础 CLI 工具
- 初始化本体仓库目录树与 Git 仓库
- 生成 `.env` 与 `config.toml`
- 安装阶段允许 `targets = []`
- 生成 user-level systemd units

`.env` 中最少应检查：

- `GH_TOKEN`
- `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN`，如果走代理模式

## 常用命令

```bash
~/.ccclaw/bin/ccclaw config
~/.ccclaw/bin/ccclaw doctor
~/.ccclaw/bin/ccclaw ingest
~/.ccclaw/bin/ccclaw run
~/.ccclaw/bin/ccclaw status
~/.ccclaw/bin/ccclaw target list
~/.ccclaw/bin/ccclaw target add --repo owner/repo --path /abs/path
~/.ccclaw/bin/ccclaw target disable --repo owner/repo
```

## release 发布

发布统一从 `src/Makefile` 进入：

```bash
cd src
make package
make checksum
make sign GPG_KEY_ID=<gpg-key-id> MINISIGN_KEY_FILE=/path/to/minisign.key
make release GPG_KEY_ID=<gpg-key-id> MINISIGN_KEY_FILE=/path/to/minisign.key
```

约束：

- release tag 使用 `yy.mm.dd.HHMM`
- 首轮默认平台为 `linux/amd64`
- release 资产至少包含 `tar.gz`、`SHA256SUMS`、`gpg` 签名与 `minisign` 签名
- 开发机本地归档目录固定为 `/ops/logs/ccclaw/`，不纳入版本控制
- 若 minisign 私钥无密码保护，可追加 `MINISIGN_UNPROTECTED=1`

## 工程报告

工程报告统一落在 `docs/reports/`，文件命名约定：

```text
yymmdd_[Issue No.]_[Case Summary].md
```
