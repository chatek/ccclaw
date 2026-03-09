# ccclaw
<<<<<<< HEAD
> Claude Code + GH Issues + Supervisor + SKILL memory
>
> 以 GitHub Issue 为任务入口的长期异步任务闭环系统。
||||||| bdb2517

> Claude Code + GH Issues + Supervisor + SKILL memory
>
> 以 GitHub Issue 为任务入口的长期异步任务闭环系统。
=======

> Claude Code + GitHub Issues + systemd + kb memory
>>>>>>> f46998dcce522b74c2e64a93628385c8494d69b5

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
<<<<<<< HEAD

### 4. 安装并启动

```bash
sudo make install          # 安装到 /usr/local/bin/
sudo make systemd-enable   # 安装 systemd 单元并启动 timer
```

## 日常使用

### 触发任务

在仓库创建 Issue，添加标签 `ccclaw`（或 `CCCLAW_INGEST_LABEL` 配置的标签）。

最多 5 分钟后自动入队，最多 10 分钟后自动执行。

**手动立即触发**：

```bash
ccclaw-ingest   # 立即拉取
ccclaw-run      # 立即执行队列
```

### 查看状态

```bash
ccclaw-status
```

### high-risk 任务审批

Issue 含 `risk:high` 标签时任务进入 `BLOCKED`，ccclaw 会在 Issue 下回复等待审批。
在 Issue 上添加 `approved` 标签后，下次 run timer 触发时自动执行。

## 调试

```bash
# 查看 systemd 日志
journalctl -u ccclaw-ingest.service -f
journalctl -u ccclaw-run.service -f

# 手动单步（先加载环境变量）
export $(cat /etc/ccclaw/env | grep -v '^#' | xargs)
ccclaw-ingest
ls /var/lib/ccclaw/
ccclaw-run

# 健康检查
bash scripts/healthcheck.sh

# 重置任务（重新执行）
rm /var/lib/ccclaw/253_body.json
```

## 运维

```bash
# 停止自动执行
systemctl stop ccclaw-ingest.timer ccclaw-run.timer

# 查看死信任务
grep -rl '"state":"DEAD"' /var/lib/ccclaw/

# 清理已完成任务
mkdir -p /var/lib/ccclaw/archive
grep -rl '"state":"DONE"' /var/lib/ccclaw/*.json | xargs -I{} mv {} /var/lib/ccclaw/archive/
```

## 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `CCCLAW_GH_TOKEN` | ✓ | — | GitHub PAT |
| `CCCLAW_REPO` | ✓ | — | 目标仓库 `owner/repo` |
| `CCCLAW_CLAUDE_BIN` | | `claude` | claude 可执行文件路径 |
| `CCCLAW_STATE_DIR` | | `/var/lib/ccclaw` | 任务状态目录 |
| `CCCLAW_LOG_DIR` | | `/var/log/ccclaw` | 执行日志目录 |
| `CCCLAW_INGEST_LABEL` | | `ccclaw` | 触发 ingest 的 Issue 标签 |
| `CCCLAW_INGEST_LIMIT` | | `20` | 每次最多拉取 Issue 数 |

## Makefile

```bash
make build           # 编译到 bin/
make test            # 运行单元测试
make install         # 安装到 /usr/local/bin/
make systemd-install # 安装 systemd 单元文件
make systemd-enable  # 启用并启动 timer
make systemd-status  # 查看 timer 状态
make clean           # 清理 bin/
```

## 里程碑

- [x] **Phase A**：ingest / run / status CLI + systemd timer + 幂等键
- [x] **Phase B**：docs/ 记忆注入 + claude -p agent 模式 + SKILL 自学习闭环 + TodoWrite/Task 计划编排
- [ ] **Phase C**：重试/死信/告警 + Token 最小权限手册
- [ ] **Phase D**：指标周报自动化 + 主动扫描 docs/plans/ 发现任务

## tracing

- 260224 NOTAschool/sys4NOTA#253 Phase A MVP 完成，迁移至独立仓库
- 260224 NOTAschool/ccclaw#2 Phase B 计划化与子任务化完成

## refer.

- [NOTAschool/sys4NOTA#253](https://github.com/NOTAschool/sys4NOTA/issues/253) — 实效开发计划
- [OpenClaw](https://github.com/openclaw/openclaw) — 上游参考架构
- [learn-claude-code](https://github.com/shareAI-lab/learn-claude-code) — SKILL 模式参考
||||||| bdb2517

### 4. 安装并启动

```bash
sudo make install          # 安装到 /usr/local/bin/
sudo make systemd-enable   # 安装 systemd 单元并启动 timer
```

## 日常使用

### 触发任务

在仓库创建 Issue，添加标签 `ccclaw`（或 `CCCLAW_INGEST_LABEL` 配置的标签）。

最多 5 分钟后自动入队，最多 10 分钟后自动执行。

**手动立即触发**：

```bash
ccclaw-ingest   # 立即拉取
ccclaw-run      # 立即执行队列
```

### 查看状态

```bash
ccclaw-status
```

### high-risk 任务审批

Issue 含 `risk:high` 标签时任务进入 `BLOCKED`，ccclaw 会在 Issue 下回复等待审批。
在 Issue 上添加 `approved` 标签后，下次 run timer 触发时自动执行。

## 调试

```bash
# 查看 systemd 日志
journalctl -u ccclaw-ingest.service -f
journalctl -u ccclaw-run.service -f

# 手动单步（先加载环境变量）
export $(cat /etc/ccclaw/env | grep -v '^#' | xargs)
ccclaw-ingest
ls /var/lib/ccclaw/
ccclaw-run

# 健康检查
bash scripts/healthcheck.sh

# 重置任务（重新执行）
rm /var/lib/ccclaw/253_body.json
```

## 运维

```bash
# 停止自动执行
systemctl stop ccclaw-ingest.timer ccclaw-run.timer

# 查看死信任务
grep -rl '"state":"DEAD"' /var/lib/ccclaw/

# 清理已完成任务
mkdir -p /var/lib/ccclaw/archive
grep -rl '"state":"DONE"' /var/lib/ccclaw/*.json | xargs -I{} mv {} /var/lib/ccclaw/archive/
```

## 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `CCCLAW_GH_TOKEN` | ✓ | — | GitHub PAT |
| `CCCLAW_REPO` | ✓ | — | 目标仓库 `owner/repo` |
| `CCCLAW_CLAUDE_BIN` | | `claude` | claude 可执行文件路径 |
| `CCCLAW_STATE_DIR` | | `/var/lib/ccclaw` | 任务状态目录 |
| `CCCLAW_LOG_DIR` | | `/var/log/ccclaw` | 执行日志目录 |
| `CCCLAW_INGEST_LABEL` | | `ccclaw` | 触发 ingest 的 Issue 标签 |
| `CCCLAW_INGEST_LIMIT` | | `20` | 每次最多拉取 Issue 数 |

## Makefile

```bash
make build           # 编译到 bin/
make test            # 运行单元测试
make install         # 安装到 /usr/local/bin/
make systemd-install # 安装 systemd 单元文件
make systemd-enable  # 启用并启动 timer
make systemd-status  # 查看 timer 状态
make clean           # 清理 bin/
```

## 里程碑

- [x] **Phase A**：ingest / run / status CLI + systemd timer + 幂等键
- [ ] **Phase B**：docs/ 记忆注入 + claude -p agent 模式 + SKILL 自学习闭环
- [ ] **Phase C**：重试/死信/告警 + Token 最小权限手册
- [ ] **Phase D**：指标周报自动化 + 主动扫描 docs/plans/ 发现任务

## tracing

- 260224 NOTAschool/sys4NOTA#253 Phase A MVP 完成，迁移至独立仓库

## refer.

- [NOTAschool/sys4NOTA#253](https://github.com/NOTAschool/sys4NOTA/issues/253) — 实效开发计划
- [OpenClaw](https://github.com/openclaw/openclaw) — 上游参考架构
- [learn-claude-code](https://github.com/shareAI-lab/learn-claude-code) — SKILL 模式参考

=======
>>>>>>> f46998dcce522b74c2e64a93628385c8494d69b5
