# 260310_8_issue8_install_decision_impl

## 背景

对应 Issue：#8 `release:26.3.10.1142 安装失败`

决策来源：

- Issue 评论：<https://github.com/41490/ccclaw/issues/8#issuecomment-4029090959>

本报告记录该拍板后的首轮实现落地，不重复展开前一份调查草稿中的现象与根因。

## 拍板映射

Issue 评论中的 4 个关键信号，本轮分别落地为：

1. `systemd` 异常接受降级继续
   - 安装器新增 `--scheduler auto|systemd|cron|none`
   - `auto` 下优先使用 `systemd --user`
   - 若 user systemd 不可用，则自动降级为 `none`，安装继续完成
   - 若用户显式指定 `--scheduler systemd`，则仍按强约束失败

2. `GH_TOKEN` 若本机已有，直接复用
   - 安装前体检新增 `gh auth status`
   - `.env` 缺失 `GH_TOKEN` 时，优先从 `gh auth token` 回填
   - 安装摘要明确打印 `GH_TOKEN` 来源

3. `local task repo` 本次必须支持“只给本地路径”
   - `local` 模式改为先问路径
   - 若本地仓库存在 `origin`，自动推断 `owner/repo`
   - 推断成功时不再强制重复询问
   - 推断失败时，明确解释运行时仍依赖 `owner/repo`，再要求手工补录

4. `remote` 任务仓库固定 clone 入口
   - 新增默认 clone 根目录 `/opt/src/3claw`
   - remote 任务仓库默认落到 `/opt/src/3claw/owner/repo`
   - 若 remote 模式显式传入 `--task-repo-path`，也要求仍位于该入口下
   - clone 前会先创建 `/opt/src/3claw`，必要时走 `sudo`，随后刷回当前用户权限

## 实现范围

本轮实际修改：

- `src/dist/install.sh`
- `src/ops/config/config.example.toml`
- `src/dist/ops/config/config.example.toml`
- `src/dist/README.md`
- `src/ops/examples/install-flow.md`
- `src/dist/ops/examples/install-flow.md`
- `README.md`
- `README_en.md`

## 关键实现

### 1. 安装前体检

新增统一体检输出，覆盖：

- Claude 登录态
- `gh auth` 登录态与 token 可复用性
- `systemctl --user` 与 user systemd 目录可用性
- remote task repo clone 根目录约束
- 请求调度模式与最终生效调度模式

同时补入 `--preflight-only`，用于只做体检不落盘。

### 2. 调度器降级

安装器现在区分：

- 请求调度模式：`SCHEDULER`
- 实际调度模式：`SCHEDULER_EFFECTIVE`

行为约定：

- `auto`：可用则 `systemd`，不可用则降级 `none`
- `systemd`：不可用即失败
- `cron`：不写 user systemd 单元，只输出 cron 样板
- `none`：不写 user systemd 单元

这使“调度器异常”不再默认等同于“整次安装失败”。

### 3. `.env` 复用与来源摘要

新增行为：

- 已有 `.env` 时补齐缺省键位
- 若 `GH_TOKEN` 缺失且本机 `gh auth token` 可取，则自动回填
- 安装摘要打印 `GH_TOKEN` 来源
- 若已检测到 Claude CLI 登录态，则明确提示 `ANTHROPIC_API_KEY` 可留空

### 4. 注释化配置模板

`config.toml` 样板与安装产物新增中文注释，明确：

- `default_target` 的路由语义
- `control_repo` 与任务仓库的边界
- `app_dir` / `home_repo` / `kb_dir` / `env_file` 的职责
- `[[targets]]` 的推荐写法与 remote clone 入口

### 5. 分段式交互

交互补成 4 段：

- 阶段 1/4：安装拓扑
- 阶段 2/4：任务仓库绑定
- 阶段 3/4：调度器
- 阶段 4/4：凭据复用与补录

这样至少先把“本地任务仓库路径”与“敏感项录入”从视觉上拆开，降低误判。

## 验证

本轮完成的直接验证：

```bash
bash -n src/dist/install.sh
src/dist/install.sh --help
src/dist/install.sh --yes --simulate --home-repo-mode init --task-repo-mode remote --task-repo-remote 41490/ccclaw --scheduler auto
src/dist/install.sh --yes --simulate --home-repo-mode init --task-repo-mode local --task-repo-local /opt/src/ccclaw --scheduler none
```

验证结果：

- `install.sh` 语法检查通过
- `--help` 已包含 `--preflight-only`、`--scheduler`、`--task-clone-root`
- remote 场景默认落到 `/opt/src/3claw/41490/ccclaw`
- local 场景可从当前仓库 `origin` 自动推断 `41490/ccclaw`
- `auto` 调度模式下可正确解析为 `systemd`
- `none` 调度模式下会跳过 user systemd 写入

## 残余风险

本轮仍未完成的部分：

- 未补专门的 shell 自动化回归测试，只做了语法与模拟场景验证
- `local` 仓库若没有可解析的 GitHub `origin`，仍需要人工补 `owner/repo`
- `cron` 目前只输出样板，不自动写入用户 crontab

## 结论

Issue #8 的拍板项已进入安装实现主路径，至少完成了：

- 安装前体检
- scheduler 降级
- `GH_TOKEN` 自动复用
- `local task repo` 自动推断
- remote clone 入口固定化
- 注释化配置模板

后续若继续收口，可优先补 shell 级回归测试与 `doctor` 对 scheduler 降级状态的显式诊断。
