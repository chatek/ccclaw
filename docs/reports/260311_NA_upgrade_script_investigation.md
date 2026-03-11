# 260311_NA_upgrade_script_investigation

## 背景

用户反馈在本机执行：

```bash
bash ~/.ccclaw/upgrade.sh
~/.ccclaw/bin/ccclaw -V
```

输出版本仍为：

```text
26.03.11.0440
```

同时发现 `~/.ccclaw/ops/config/config.toml` 没有补入近期新增的调度相关配置项。

本轮目标不是直接动修复代码，而是先把升级链路的真实行为、根因、修复方向与待决策点查清，并沉淀到 Issue。

## 复现与证据

### 1. 当前已安装配置仍是旧结构

现场配置文件中仅有：

- `[scheduler] mode`
- `[scheduler] systemd_user_dir`

缺少当前安装模板里已经存在的：

- `scheduler.calendar_timezone`
- `[scheduler.timers]`
- `[scheduler.logs]`

同时现场配置中的：

- `paths.home_repo = '/opt/data/9527'`

与 `upgrade.sh` 默认使用的 `/opt/ccclaw` 不一致。

### 2. `upgrade.sh` 只会调用已安装目录里的 `install.sh`

`src/dist/upgrade.sh` 当前逻辑：

```bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="${APP_DIR:-$HOME/.ccclaw}"
HOME_REPO="${HOME_REPO:-/opt/ccclaw}"
...
"$SCRIPT_DIR/install.sh" "${args[@]}"
```

关键事实：

- `SCRIPT_DIR` 指向已安装目录 `~/.ccclaw`
- 被调用的是 `~/.ccclaw/install.sh`
- 没有任何 “拉取 release / 解包 release / 同步源码仓库 dist/” 的步骤

因此它根本拿不到仓库里的新 `install.sh`、新 `upgrade.sh`、新二进制和新模板。

### 3. `install.sh` 的 `DIST_DIR` 也是脚本所在目录

`src/dist/install.sh` 中：

```bash
DIST_DIR="$(cd "$(dirname "$0")" && pwd)"
...
install -m 755 "$DIST_DIR/bin/ccclaw" "$APP_DIR/bin/ccclaw"
install -m 755 "$DIST_DIR/install.sh" "$APP_DIR/install.sh"
install -m 755 "$DIST_DIR/upgrade.sh" "$APP_DIR/upgrade.sh"
cp -R "$DIST_DIR/ops/." "$APP_DIR/ops/"
```

如果执行入口是 `~/.ccclaw/install.sh`，那么：

- `DIST_DIR=~/.ccclaw`
- 源和目标都是已安装旧文件

这不是“从新发布覆盖旧安装”，而是“把旧安装再复制回自己”。

### 4. 实际跟踪执行会在自复制阶段失败

对现场脚本执行 `bash -x ~/.ccclaw/upgrade.sh` 可见：

```text
/home/zoomq/.ccclaw/install.sh --yes --app-dir /home/zoomq/.ccclaw --home-repo /opt/ccclaw
...
install: '/home/zoomq/.ccclaw/bin/ccclaw' and '/home/zoomq/.ccclaw/bin/ccclaw' are the same file
```

说明当前升级链路不仅不会升级，甚至会在“把自己安装到自己”时退出失败。

### 5. 现有安装器会保留旧 `config.toml`

`create_config_file()` 当前逻辑是：

- 若 `config.toml` 已存在，则直接保留
- 只额外调用 `ccclaw config migrate-approval`
- 不会做通用 schema 补齐

这意味着旧安装即使被“重新安装”，也不会自动补入：

- `scheduler.calendar_timezone`
- `[scheduler.timers]`
- `[scheduler.logs]`

### 6. 现场已安装脚本确实落后于仓库当前版本

对比发现：

- 仓库里的 `src/dist/install.sh` 已支持新的 scheduler 配置写入与同步
- 现场 `~/.ccclaw/install.sh` 仍是旧版，只支持 `--mode` 和 `--systemd-user-dir`

由于升级入口永远只会执行现场旧脚本，所以新逻辑无法进入用户机器。

## 根因归纳

这是三个问题叠加，不是单一补丁能解决：

### 1. 升级入口缺少“获取新发布树”的来源

`upgrade.sh` 没有定义升级源：

- 不拉 GitHub release
- 不读取本地 release 包
- 不从工作仓库 `src/dist/` 同步
- 不校验版本差异

所以“升级”实际上只是重跑本地旧安装器。

### 2. 升级参数没有从现有配置回填安装上下文

`upgrade.sh` 直接用默认值：

- `APP_DIR=~/.ccclaw`
- `HOME_REPO=/opt/ccclaw`

但现场 `config.toml` 中已经明确记录：

- `app_dir=/home/zoomq/.ccclaw`
- `home_repo=/opt/data/9527`

升级不读取已安装配置，导致本体仓库路径回退到默认值，存在误指向风险。

### 3. 缺少通用配置迁移机制

当前只有定向的 `migrate-approval`，没有：

- `config migrate`
- 安装器升级补齐缺失字段
- 保守合并新默认配置段

于是旧配置无法随着版本演进自动增加新字段。

## 影响

### 直接影响

- `bash ~/.ccclaw/upgrade.sh` 不能升级二进制
- 新版 `install.sh` / `upgrade.sh` / `ops/*` 无法进入已安装目录
- 新增配置段无法随升级进入 `config.toml`

### 间接影响

- 用户会误以为升级成功，因为入口名叫 `upgrade.sh`
- 一旦现场 `home_repo` 不是默认 `/opt/ccclaw`，升级脚本会带着错误默认值执行
- 后续每次新增配置项都可能重复出现“模板已更新，存量安装无迁移”的问题

## 建议修复方案

### 方案骨架

分两层修：

1. 先修“发布树升级来源”
2. 再修“已安装配置迁移”

### A. 把 `upgrade.sh` 改成真正的发布树升级器

建议至少支持一种明确来源，禁止再用“已安装目录自复制”冒充升级：

- 方案 A1：GitHub release 拉取
  - 通过 `gh release download` 或 `curl` 下载指定版本/最新版发布包
  - 校验 `SHA256SUMS`
  - 解压到临时目录
  - 从临时目录里的 `install.sh` 执行升级
- 方案 A2：本地 release 包升级
  - `upgrade.sh --from /path/to/ccclaw_xxx.tar.gz`
  - 适合离线和受控环境
- 方案 A3：开发模式从工作仓库升级
  - 显式传 `--from-repo /opt/src/ccclaw/src/dist`
  - 只作为开发者入口，不作为默认用户路径

最低要求：

- 默认路径必须能拿到“新文件”
- 不能再直接执行 `~/.ccclaw/install.sh`
- 临时目录完成后再覆盖安装目录

### B. 升级时从现有 `config.toml` 回填上下文

`upgrade.sh` 应先读取现有配置里的：

- `paths.app_dir`
- `paths.home_repo`
- `scheduler.mode`
- `scheduler.systemd_user_dir`

至少保证：

- 不错误回退到 `/opt/ccclaw`
- 现有部署拓扑不被默认值冲掉

### C. 增加通用配置迁移

建议新增：

```bash
ccclaw config migrate
```

职责：

- 补齐缺失的已知配置段
- 保留用户已写值
- 对废弃字段给出定向迁移
- 允许重复执行且幂等

安装器升级阶段调用顺序建议为：

1. 用新发布树覆盖程序文件
2. 运行 `ccclaw config migrate`
3. 再执行针对调度器的同步命令
4. 最后刷新 systemd/cron

### D. 避免同路径自安装报错

即使未来仍存在“源目录即安装目录”的特殊场景，也应显式处理：

- 若源目标为同一 inode，则跳过复制
- 或必须要求升级源是临时解压目录

更稳妥的做法仍是只允许“临时发布树 -> 安装目录”的覆盖，不允许“安装目录 -> 安装目录”。

## 待决策点

以下问题不建议实现者自行脑补，需先在 Issue 拍板：

1. `upgrade.sh` 的默认升级源是什么
   - GitHub 最新 release
   - 指定 release tag
   - 本地 release 包
   - 开发仓库 `src/dist`

2. 是否要求升级必须做 `SHA256SUMS` 校验
   - 从当前发布约束看，建议必须校验

3. `config migrate` 的保真策略
   - 允许按段重写，丢失段内注释
   - 还是要做更高保真合并，尽量保留用户注释

4. 是否保留当前 `upgrade.sh` 入口语义
   - 保留名字但修正行为
   - 或拆成 `upgrade.sh` 与 `install-from-release.sh`

## 结论

当前 `~/.ccclaw/upgrade.sh` 失效不是偶发问题，而是升级模型本身未闭环：

- 没有新发布来源
- 没有读取现有安装上下文
- 没有通用配置迁移
- 现场执行还会在自复制阶段失败

下一步应先在 Issue 上拍板升级源与迁移策略，再进入实现。
