# CCClaw

```text
  o-o   o-o   o-o o
 /     /     /    |
O     O     O     |  oo o   o   o
 \     \     \    | | |  \ / \ /
  o-o   o-o   o-o o o-o-  o   o
```

> 专注节省 token 消耗的基于 Claude Code 的长效自主编排智能体

默认中文说明，英文版见 [README_en.md](README_en.md)。

`CCClaw` 也可简写为 `3Claw`。名字可以理解为一组开放缩写组合：

- `Claude | Common | Chaos | ...`
- `Code | Continuance | Calm | ...`
- `Claw`

## 项目简介

`ccclaw` 是一个以 **GitHub Issue 作为任务事实源**、以 **Claude Code 作为执行体**、以 **systemd --user 作为后台调度**、以 **本体知识库仓库作为长期记忆** 的 Linux 原生自主编排工具。

它当前已经完成 `phase0.4` 所要求的首个可下载安装 release 闭环，适合先在单机、单维护者或小团队开源协作场景中稳定运行。

相比“每次都手工打开 Claude Code 聊一轮”，`ccclaw` 的重点不是把 agent 做得更花，而是把以下高频损耗尽量移出高价模型执行阶段：

- Issue 巡检、门禁、调度、回写
- 固定目录、固定配置、固定安装拓扑
- 长期记忆与运行记录归档
- 多仓库绑定与默认目标路由
- 非管理员提案的自动拦截

## 核心特点

- **省 token**：把轮询、门禁、调度、目录治理、日志归档交给本地程序和 `systemd`，把高价值 token 留给真正需要 Claude Code 动手的阶段。
- **Issue 驱动**：直接复用开源项目最自然的协作入口，不额外引入新的任务系统。
- **Linux 原生**：默认依赖 `systemd --user`，不强绑 Docker、K8s、外部 SaaS 控制面。
- **记忆与程序分离**：程序树固定在 `~/.ccclaw`，本体记忆仓库固定在 `/opt/ccclaw`，升级不覆盖用户记忆。
- **安装边界清晰**：敏感信息只进 `.env`，普通配置只进 `.toml`，运行时不依赖调用者预先 `export` 环境变量。
- **开源协作可控**：管理员 Issue 自动执行；非管理员 Issue 必须管理员评论 `/ccclaw approve` 后才进入执行。
- **可发布可升级**：release 从 `src/dist/` 出包，产物至少包含安装包与 `SHA256SUMS`，并提供 `upgrade.sh` 升级程序树。

## 适用场景

- 个人维护多个 GitHub 仓库，希望让 Claude Code 在后台持续巡检和执行。
- 小团队开源协作，希望保留 Issue 审计链，而不是把任务入口搬到聊天工具。
- 需要长期积累项目知识、计划、日报、技能模板，而不是每次上下文都重新喂给模型。
- 想把 Claude Code 用在长周期任务里，但不希望无意义轮询和重复解释消耗太多 token。
- 希望先落地一个极简、稳定、Linux-first 的编排内核，再逐步扩到更多 provider 或协作总线。

## 当前状态

- 当前阶段：`phase0.4`
- 当前能力：已具备首个可正确下载安装的 release 流
- 默认平台：`linux/amd64`
- 默认程序目录：`~/.ccclaw`
- 默认本体仓库：`/opt/ccclaw`
- 本体仓库模式：`init | remote | local`
- 任务仓库模式：`none | remote | local`
- 默认调度方式：`systemd --user`
- 版本命名：`yy.mm.dd.HHMM`

## 工作拓扑

```text
GitHub Issue
    |
    v
ccclaw ingest/run
    |
    +--> admin gate / approve gate
    +--> target repo route
    +--> Claude Code executor
    +--> docs + reports + kb memory
    |
systemd --user timers
```

安装后的默认目录结构：

```text
~/.ccclaw
├── bin/
│   ├── ccclaw
│   └── ccclaude
├── .env
├── log/
├── var/
└── ops/
    ├── config/config.toml
    ├── systemd/
    └── scripts/

/opt/ccclaw
├── kb/
│   ├── designs/
│   ├── assay/
│   ├── journal/
│   └── skills/
│       ├── L1/
│       └── L2/
└── docs/
    ├── reports/
    ├── plans/
    └── rfcs/
```

## 为什么是 CCClaw

同类 agent/automation 项目通常会优先强调多模型、前端面板、聊天协作或云端控制面；`ccclaw` 当前阶段反过来，先把开源协作里最难稳定的几个基线收口：

1. **任务事实源固定为 GitHub Issue**
2. **安装拓扑固定，不靠调用方自由拼环境变量**
3. **敏感配置与普通配置分轨**
4. **长期记忆与程序升级解耦**
5. **开源仓库默认先门禁，再执行**

这使它非常适合“低噪声、可审计、长期运行”的工程环境。

## 安装前准备

在下载 release 之前，建议先准备好以下依赖与账号条件。

### 账户与凭据

- 一个可正常使用的 GitHub 帐号
- 一个具备目标仓库访问权限的 GitHub token，供 `gh` CLI 与运行时 `.env` 中的 `GH_TOKEN` 使用
- 二选一的 Claude 接入方式：
  - 有可用的 Claude Code 订阅帐号，并能完成对应登录/授权
  - 或者有可供 Claude Code 代理调用的 API `URL + token`

### 网络与地区条件

- 推荐直接位于 Claude Code 官方允许使用的国家/地区
- 若不在允许范围内，请自行确认你的网络出口、账号使用方式和服务条款是否合规
- 本项目文档只说明官方安装与代理配置入口，不提供绕过区域校验的方法

### 主机条件

- Linux 主机
- 建议存在 `systemd --user`
- 建议当前用户具备 `sudo` 能力，以便安装基础依赖与写入 `/opt/ccclaw`
- 推荐预装或允许安装：`git`、`gh`、`curl`、`wget`、`rg`、`sqlite3`、`golang`

### 建议预先完成的动作

```bash
gh auth login
gh auth status
```

如果你计划使用代理模式，而不是直接走 Claude Code 官方登录，请预先确认至少准备好：

- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_AUTH_TOKEN`

## 下载与安装

### 1. 下载最新 release

可直接从 Releases 页面下载最新安装包与校验文件：

- <https://github.com/41490/ccclaw/releases>

如果已经装好 `gh`，也可以在目标主机执行：

```bash
mkdir -p /tmp/ccclaw-release
cd /tmp/ccclaw-release
gh release download --repo 41490/ccclaw --pattern 'ccclaw_*_linux_amd64.tar.gz' --pattern 'SHA256SUMS'
```

### 2. 校验安装包

```bash
sha256sum -c SHA256SUMS
```

### 3. 解压并执行安装脚本

```bash
tar -xzf ccclaw_*_linux_amd64.tar.gz
cd ccclaw_*_linux_amd64
bash install.sh
```

默认安装会引导你交互式确认以下信息：

- 程序目录，默认 `~/.ccclaw`
- 控制仓库，默认 `41490/ccclaw`
- 本体仓库模式：`init | remote | local`
- 本体仓库目录，默认 `/opt/ccclaw`
- 任务仓库模式：`none | remote | local`
- `GH_TOKEN`
- 可选的 Claude 代理配置

### 4. 安装脚本会做什么

`install.sh` 当前会按真实实现执行以下动作：

- 探查 `claude`、`gh`、`rg`、`sqlite3`、`rtk`、`git`、`node`、`npm`、`uv`
- 探查 Claude 官方安装通道可达性
- 在需要时安装基础系统依赖
- 安装 `rtk`，并生成 `ccclaude` 包装器
- 初始化或接管本体仓库
- 生成固定 `.env`
- 生成固定 `config.toml`
- 安装 `~/.ccclaw/bin/ccclaw`
- 安装 `install.sh` 与 `upgrade.sh`
- 安装 `systemd --user` unit 文件
- 按需绑定任务仓库并写入 `[[targets]]`
- 输出安装完成摘要、后续命令和日常工作流程

## 推荐安装路径

### 路径 A：交互式首次安装

适合第一次部署、希望看清每一步提示的人：

```bash
bash install.sh
```

### 路径 B：非交互安装，初始化本体仓库

```bash
bash install.sh \
  --yes \
  --home-repo-mode init \
  --task-repo-mode none
```

### 路径 C：非交互安装，并接管远程本体仓库

```bash
bash install.sh \
  --yes \
  --home-repo-mode remote \
  --home-repo /opt/ccclaw \
  --home-repo-remote owner/private-home-repo
```

### 路径 D：非交互安装，并绑定远程任务仓库

```bash
bash install.sh \
  --yes \
  --home-repo-mode init \
  --task-repo-mode remote \
  --task-repo-remote owner/work-repo \
  --task-repo owner/work-repo \
  --task-repo-path ~/work-repo
```

### 路径 E：非交互安装，并绑定本地任务仓库

```bash
bash install.sh \
  --yes \
  --home-repo-mode init \
  --task-repo-mode local \
  --task-repo-local /abs/path/to/repo \
  --task-repo owner/work-repo
```

## Claude Code 接入准备

`ccclaw` 当前只建议两类 Claude 接入方式：

### 方式 1：Claude Code 官方登录/授权

- 已安装 Claude Code CLI
- 已完成对应登录
- 安装脚本会探查当前本机 `claude` 状态

如果本机未安装 `claude`，交互安装时会提示是否执行官方安装脚本；非交互模式下只有显式传入 `--install-claude` 才会自动安装。

### 方式 2：代理 API 模式

在安装或安装后，确保 `.env` 中至少配置：

```bash
ANTHROPIC_BASE_URL=
ANTHROPIC_AUTH_TOKEN=
```

如果你走的是代理模式，README 中所有“Claude 登录”相关步骤都可以替换为“保证代理 URL 与 token 可用”。

## 安装完成后先做什么

```bash
~/.ccclaw/bin/ccclaw doctor \
  --config ~/.ccclaw/ops/config/config.toml \
  --env-file ~/.ccclaw/.env
```

如果需要启用默认后台调度：

```bash
systemctl --user daemon-reload
systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer
```

如果需要检查任务仓库绑定结果：

```bash
~/.ccclaw/bin/ccclaw target list --config ~/.ccclaw/ops/config/config.toml
```

## 常用命令

```bash
ccclaw
ccclaw -h
ccclaw -V
ccclaw doctor
ccclaw config
ccclaw ingest
ccclaw run
ccclaw status
ccclaw target list
ccclaw target add --repo owner/repo --path /abs/path/to/repo
ccclaw target disable --repo owner/repo
```

CLI 约定：

- 无参数：输出帮助
- `-h | --help`：输出帮助
- `-V | --version`：输出版本

## 配置文件

### 敏感配置

固定在：

```text
~/.ccclaw/.env
```

至少应检查：

- `GH_TOKEN`
- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_AUTH_TOKEN`

可按需补充：

- `ANTHROPIC_API_KEY`
- `GREPTILE_API_KEY`

### 普通配置

固定在：

```text
~/.ccclaw/ops/config/config.toml
```

这里会记录：

- 控制仓库
- 路径拓扑
- 执行器命令
- 审批门禁
- `targets`

## 日常使用流程

1. 绑定工作任务仓库
2. 在控制仓库创建或维护 Issue
3. 由 `ccclaw` 巡检并在满足门禁后执行
4. 检验工作成果，并继续在 Issue 中回复交流

开源协作门禁流程：

1. 管理员创建的 Issue 自动进入执行判定
2. 外部成员 Issue 默认只巡查与讨论
3. 管理员评论 `/ccclaw approve` 后，Issue 才进入执行

## 升级与发布

程序树升级入口：

```bash
~/.ccclaw/upgrade.sh
```

升级约束：

- 升级只刷新程序树与受管契约文件
- 不覆盖 `/opt/ccclaw` 中的用户记忆
- `kb/**/CLAUDE.md` 使用受管区块方式无损合并

开发者发布入口统一在 `src/Makefile`：

```bash
cd src
make fmt
make test
make build
make package
make checksum
make archive
make release
```

release 约束：

- 从 `src/dist/` 打包
- 至少包含安装包与 `SHA256SUMS`
- 本机归档目录固定为 `/ops/logs/ccclaw/`
- `make release` 仅允许在干净工作树上执行

## 从源码构建

如果你不是使用 release 包，而是直接从源码构建：

```bash
cd src
make build
make package
```

构建后的主二进制位于：

```text
src/dist/bin/ccclaw
```

release 安装树位于：

```text
src/dist/
```

## 项目优势总结

- 对开源维护者友好：直接接 GitHub Issue，而不是要求团队切换协作入口
- 对 Linux 主机友好：默认 `systemd --user`，不强迫引入额外编排系统
- 对 token 预算友好：把确定性流程收敛到本地程序与固定目录
- 对长期项目友好：记忆树、报告、计划、技能模板都有固定落点
- 对升级友好：程序树和本体仓库分离，降低误覆盖风险

## 已知边界

- 当前默认只覆盖 `linux/amd64`
- 当前执行器仍以 Claude Code 为中心，尚未进入 phase1 的多 provider 扩展
- 当前主协作入口仍是 GitHub Issue，Matrix 等实时协作总线尚未进入默认安装面

## 许可证

本项目使用 [MIT License](LICENSE)。
