# ccclaw

> Claude Code + GH Issues + Supervisor + SKILL memory
>
> 以 GitHub Issue 为任务入口的长期异步任务闭环系统。

## 目标

- 以 Claude Code 为执行体
- 以 gh-issue 为异步沟通界面
- 以 skill 为自主升级的能力结构
- 以本地 docs/ 文档目录为长期永久分层记忆
- 以 Debian 兼容系统的 systemd 为调度守护

能长期自主运行的，能主动自学探索成长的任务执行系统。token 消耗可控，异步任务助理。

## 架构

```
GitHub Issue
  └─► ccclaw-ingest          拉取新 Issue，写入本地状态文件（幂等）
        └─► /var/lib/ccclaw/  任务状态目录（JSON 文件，一任务一文件）

/var/lib/ccclaw/
  └─► ccclaw-run             读取 NEW/TRIAGED 任务，调用 claude 执行
        ├─► claude -p         Claude Code worker（非交互模式）
        └─► ccclaw-reporter   执行完成后回写 Issue comment

systemd timer
  ├─► ccclaw-ingest.timer    每 5 分钟触发一次 ingest
  └─► ccclaw-run.timer       每 10 分钟触发一次 run
```

### 任务状态机

```
NEW → TRIAGED → RUNNING → DONE
                        → FAILED  (重试 < 3)
                        → DEAD    (重试 >= 3，死信)
                        → BLOCKED (high-risk 未审批)
```

### 风险门禁

| 风险等级 | 触发条件 | 执行条件 |
|----------|----------|----------|
| `low` | 默认 | 自动执行 |
| `med` | Issue 含 `risk:med` 标签 | 自动执行 |
| `high` | Issue 含 `risk:high` 标签 | 必须同时含 `approved` 标签 |

## 快速开始

### 1. 编译

```bash
make build
# 产物输出到 bin/ccclaw-{ingest,run,status}
```

### 2. 配置环境变量

```bash
sudo mkdir -p /etc/ccclaw
sudo cp scripts/env.example /etc/ccclaw/env
sudo chmod 600 /etc/ccclaw/env
sudo vi /etc/ccclaw/env
```

必填项：

```bash
CCCLAW_GH_TOKEN=ghp_xxxxxxxxxxxx   # GitHub PAT（issues:read + issues:write）
CCCLAW_REPO=NOTAschool/ccclaw      # 目标仓库
```

### 3. 创建运行目录

```bash
sudo mkdir -p /var/lib/ccclaw /var/log/ccclaw
```

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
