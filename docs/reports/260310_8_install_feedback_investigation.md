# 260310_8_install_feedback_investigation

## 背景

对应 Issue：#8 `release:26.3.10.1142 安装失败`

本轮目标不是直接动代码，而是先基于人工安装反馈，对当前 release 安装链路做一次根因级核查，给出后续优化方案，并明确哪些点需要 `sysNOTA` 先拍板。

## 输入事实

### 1. 来自 Issue #8 的人工反馈

已确认的现场现象：

- 任务仓库模式选择 `local` 后，用户感知上没有继续明确询问本地任务仓库位置
- 安装阶段继续进入 token/key 采集
- 写入 `~/.config/systemd/user/ccclaw-ingest.service` 时出现 `Permission denied`
- 用户认为 `.env` 未生成
- 生成的 `.toml` 缺少足够的功能说明注释

### 2. 当前实现核查结果

核查对象：`src/dist/install.sh`、`src/ops/config/config.example.toml`、`src/internal/app/runtime.go`

当前安装脚本的既有能力：

- 已支持本体仓库 `init|remote|local`
- 已支持任务仓库 `none|remote|local`
- 已区分 `.env` 与 `config.toml`
- 已生成 user-level systemd 单元
- 已有 `doctor` 检查，但发生在安装完成后

当前实现的关键缺口：

- 安装前探查集中在 Claude 生态，没有把 `gh` 登录态、`GH_TOKEN` 可提取性、systemd 用户目录可写性作为同级前置门禁
- 安装交互虽然覆盖了 `task_repo_mode=local` 的路径输入，但交互节奏弱，没有阶段提示、没有“推断结果确认”，用户容易误判脚本已跳过该步骤
- `.env` 只做“已有则复用、没有则整体新建”，不会把本机已探查出的凭据来源、复用路径和缺失原因打印清楚
- `config.toml` 样板和安装产物都几乎没有注释，自解释能力不足
- user systemd 单元写入失败会直接中断安装，但这是“调度器子能力失败”，不应该默认拖垮整个基础安装

## 根因分析

### 1. 探查覆盖不完整

`install.sh` 目前有 `probe_claude()`，但没有对以下内容做等价级别探查：

- `gh auth status`
- `gh auth token` 是否可读
- `~/.config/gh/hosts.yml` 是否存在
- `systemctl --user` 是否可用
- `XDG_CONFIG_HOME/systemd/user` 是否存在、归属用户是否正确、是否可写
- `loginctl enable-linger` / 用户会话能力是否具备

结果是：脚本知道 Claude 生态好不好，但不知道 GitHub 凭据和调度器落地条件是否满足。

### 2. 交互信息架构不清

安装脚本当前是线性 prompt：

1. 目录与仓库模式
2. 敏感配置
3. 文件写入
4. systemd 单元写入

虽然代码里在 `task_repo_mode=local` 分支会询问本地路径，但缺少三类补强：

- 阶段标题
- 根据输入自动推断出的值及其来源说明
- 对“为什么还要问 owner/repo”的解释

因此只要终端输出较长、或者提示串在一起，用户就会把后续的 repo/path 问答与 token 问答混淆为“脚本没问关键项”。

### 3. `.env` 与凭据复用策略过于粗糙

当前 `create_env_file()` 的行为是：

- 若 `.env` 已存在，则直接复用
- 若 `.env` 不存在，则提示输入 `GH_TOKEN` / `ANTHROPIC_*` / `GREPTILE_API_KEY`

缺口在于：

- 没有优先从 `gh auth token` 自动填充 `GH_TOKEN`
- 没有把“Claude 已登录，可直接复用 CLI 会话，不必强求 `ANTHROPIC_API_KEY`”明确写给用户
- 没有把最终采用的是“探查值 / 用户输入 / 留空”打印成摘要
- 安装中断后，用户不知道应该去哪个路径确认 `.env` 是否已经落盘

### 4. user systemd 写入路径处理过硬

当前脚本对 user systemd 单元目录只做：

- `mkdir -p "$SYSTEMD_USER_DIR"`
- 直接 `cat > "$SYSTEMD_USER_DIR/xxx.service"`

没有做：

- 目录所有者检查
- 目录可写性检查
- `systemctl --user` 实际可用性检查
- 失败后的降级路径

因此一旦 `~/.config/systemd/user` 权限异常，脚本会在安装中后段硬失败。该失败并不意味着 `ccclaw` 主程序不可安装，只意味着“默认调度器未就绪”。

### 5. 配置文件可读性不足

`src/ops/config/config.example.toml` 与安装生成的 `config.toml` 都缺少逐项中文注释。

这会直接放大三类问题：

- 用户不知道哪些字段可改、哪些字段不建议改
- 用户不知道 `targets[].repo` 与 `targets[].local_path` 的关系
- 用户不知道 `env_file`、`home_repo`、`kb_dir` 的边界

## 优化方案

建议按四步收口，而不是继续在现有 prompt 上零散打补丁。

### 第一步：补安装前体检

新增一个安装前 `preflight` 阶段，至少检查：

- `gh` 是否存在、是否已登录、token 是否可提取
- `claude` 是否存在、是否已登录、采用何种 auth method
- `rtk` / `rg` / `sqlite3` / `git` / `go` 是否存在
- `systemctl --user` 是否可执行
- `SYSTEMD_USER_DIR` 是否可写、归属是否正确
- `/opt/ccclaw` 是否需要 sudo

输出形式建议改为：

- `OK`：可直接复用
- `WARN`：可继续安装，但会降级或需要人工确认
- `FAIL`：必须先修复，否则不继续

### 第二步：重构安装交互为分段式向导

建议把交互拆成四段：

1. 安装拓扑
2. 任务仓库绑定
3. 凭据复用与补录
4. 调度器选择与确认

其中任务仓库 `local` 模式要改成：

1. 先问本地路径
2. 自动尝试从 `origin` 推断 `owner/repo`
3. 若推断失败，明确说明“运行时路由仍需要 owner/repo，所以请手填”
4. 最后打印一次绑定摘要，再进入下一阶段

这样用户不会把“repo 路由主键”和“本地路径”当成重复提问。

### 第三步：把 `.env` 与 `config.toml` 做成可解释、可幂等更新

建议调整为：

- `.env`：允许从本机探查结果自动回填缺失项，并打印来源摘要
- `config.toml`：使用带中文注释的模板渲染，而不是纯值拼接
- 已存在配置时，不再简单跳过；应支持：
  - 只补缺项
  - 不覆盖用户手改值
  - 打印“保留 / 更新 / 新增”的变更摘要

最低应明确输出：

- `GH_TOKEN`: 来自 `gh auth token` / 用户输入 / 已有 `.env`
- `ANTHROPIC_API_KEY`: 用户输入 / 留空，原因
- `Claude 登录态`: 已复用 CLI 会话 / 未检测到

### 第四步：把调度器安装改成“可降级能力”

建议把“安装程序”与“启用调度器”解耦。

默认行为建议：

- 主程序、配置、本体仓库初始化成功后即算“核心安装成功”
- user systemd 若失败，改为：
  - 保留单元模板到 `~/.ccclaw/ops/systemd/`
  - 在摘要中明确标记“调度器未激活”
  - 给出修复命令
  - 提供 `cron` 样板或 `none` 模式作为降级方案

这能避免一个 user systemd 权限问题直接把整次安装判定成彻底失败。

## 建议新增的安装参数

为避免继续把策略写死，建议后续增加：

- `--scheduler auto|systemd|cron|none`
- `--preflight-only`
- `--reuse-gh-auth`
- `--reuse-claude-auth`
- `--no-write-env`

其中 `--scheduler` 最关键，用于明确“默认是否坚持 systemd --user”。

## 建议新增的验收标准

### 1. 首装

- 干净主机首次安装可完成
- 已登录 `gh`、已登录 `claude` 的机器上，敏感项交互显著减少

### 2. 幂等重装

- 连续执行两次安装，不覆盖用户已有 `.env` 有效值
- 连续执行两次安装，不重复追加相同 target
- 连续执行两次安装，摘要能指出“本次实际无变更”

### 3. systemd 异常场景

- `~/.config/systemd/user` 不可写时，核心安装仍能成功
- 输出明确修复指令
- `doctor` 能把该问题列为待修项

### 4. 本地任务仓库场景

- `local` 模式下路径提示明确
- 本地仓库无 `origin` 时，说明为什么仍需 `owner/repo`
- 安装后 `target list` 结果符合预期

## 需要 `sysNOTA` 拍板的点

### 1. user systemd 写入失败时的默认策略

建议优先级：

1. 默认降级继续安装，只把调度器标记为未完成
2. 允许用户显式选择 `--scheduler systemd` 强制失败

待拍板问题：

- 是否接受“调度器失败不等于安装失败”的定义
- 对 home 目录下权限异常，是否允许安装器尝试 `sudo chown` 自动修复

这里我倾向于：

- 默认不在用户家目录里自动 `sudo chown`
- 只检测、提示、给修复命令

原因是：home 路径被 sudo 改写所有者，通常是更大的本机治理问题，安装器不宜静默替用户改。

### 2. `GH_TOKEN` 是否默认从 `gh auth token` 自动落入 `.env`

这里我倾向于默认“是”，原因：

- 运行时 `gh` 子进程需要稳定拿到 `GH_TOKEN`
- 用户已经通过 `gh auth login` 登录，再手抄一次 token 没有额外价值

但这涉及一个策略选择：

- 是自动写入 `.env`
- 还是只提示“可复用”，仍要求用户确认一次

### 3. 是否支持 `local` 任务仓库不提供 `owner/repo`

当前运行时路由和 target 主键都依赖 `owner/repo`。

因此短期内我不建议放开这个约束。更现实的做法是：

- 本地有 `origin` 就自动推断
- 没有 `origin` 就清楚解释为什么必须手填

如果要真正支持“纯本地 alias target”，需要连带修改 target 路由、配置校验和 Issue 到目标仓库的匹配逻辑，不适合在本轮安装修复里顺手混入。

## 结论

Issue #8 反映的不是单一脚本 bug，而是安装流程还没有从“开发者可理解”升级到“最终用户可稳定多次执行”的阶段。

下一轮实施建议：

1. 先补 `preflight + scheduler` 决策结构
2. 再补 `.env` 自动回填与注释化配置模板
3. 最后收口 `local task repo` 交互与幂等回归测试

本轮结论已适合先回 Issue 收口讨论，等待 `sysNOTA` 对上述策略分叉拍板后再进入代码实现。
