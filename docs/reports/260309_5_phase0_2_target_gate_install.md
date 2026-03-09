# 260309_5_phase0_2_target_gate_install

## 背景

Issue [#5](https://github.com/41490/ccclaw/issues/5) 在 2026-03-09 已完成拍板，范围锁定为：

- 安装阶段允许 `targets = []`
- 默认审批门槛改为仓库 `admin`
- Claude 安装只走官方原生脚本通道，非交互模式需显式 `--install-claude`
- 授权路径只识别 `setup-token` 与代理模式
- `.env` 必须补 `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN`
- `doctor` 需要补 Claude/Go/sudo 相关巡检
- 安装后通过命令或 Issue 追加 target 绑定

## 本次实现

### 1. 配置模型与路由

- `config.toml` 新增 `default_target`
- `targets` 允许为空，不再强制安装阶段绑定首个目标仓库
- target 新增 `disabled` 状态，用于保留配置但停止路由
- 运行时支持从 Issue body 解析 `target_repo: owner/repo`
- 若 Issue 未声明 `target_repo`：
  - 存在 `default_target` 则走默认 target
  - 没有 `default_target` 且无 enabled target 时进入 `BLOCKED`
  - 有 enabled target 但未配置 `default_target` 时也进入 `BLOCKED`

### 2. 审批与阻塞语义

- 默认 `approval.minimum_permission` 从 `write` 改为 `admin`
- `syncIssue()` 不再把 `TargetRepo` 固定写成 control repo
- 阻塞原因改为可组合：
  - 非管理员发起，等待 `/ccclaw approve`
  - 未绑定 target
  - `target_repo` 无效
  - `default_target` 指向 disabled/不存在的 target
- blocked 回帖从固定文案改为输出真实阻塞原因

### 3. target 管理命令

新增 CLI：

- `ccclaw target list`
- `ccclaw target add --repo owner/repo --path /abs/path [--kb-path ...] [--default]`
- `ccclaw target disable --repo owner/repo`

这些命令直接读写 `config.toml`，用于安装完成后再逐步绑定工作仓库。

### 4. install / doctor / upgrade

`install.sh` 已更新为：

- 安装阶段不再收集首个 target
- `.env` 增加：
  - `ANTHROPIC_BASE_URL`
  - `ANTHROPIC_AUTH_TOKEN`
- 普通配置默认写入 `default_target = ""`
- 默认审批门槛写为 `admin`
- 探查 `https://claude.ai/install.sh` 可达性
- 非交互模式只有显式 `--install-claude` 才自动安装 Claude
- 将 Go 纳入依赖探查/安装
- 输出 `sudo visudo` 的人工配置提示

`doctor` 已新增/加强：

- Claude 官方安装通道检查
- Claude 运行态识别：
  - `missing_cli`
  - `install_channel_blocked`
  - `installed_unconfigured`
  - `installed_setup_token_ready`
  - `installed_proxy_ready`
- Go 工具链检查
- sudo 无口令检查
- target 路由状态摘要

`upgrade.sh` 已补最小分轨：

- `UPGRADE_PROGRAM`
- `UPGRADE_CLAUDE`
- `REFRESH_CLAUDE_ASSETS`

并明确声明不覆盖 `/opt/ccclaw` 本体仓库内容。

## 验证

执行：

```bash
bash -n src/dist/install.sh
go test ./...
```

结果：

- `install.sh` 语法检查通过
- `src/` 下 Go 测试全部通过

## 当前未完成项

- `target remove` 仍未实现，按 Issue 拍板延后
- 更复杂的 allowlist / denylist / label policy 还未进入本轮
- Claude `setup-token` 可用态目前通过 `claude auth status --json` 成功与否推断，后续可根据官方 CLI 新接口继续收紧判定

## 结论

本轮已经把 Issue #5 最后拍板中的最小闭环落地：

- 安装可以零 target 启动
- target 路由可在运行期解析与追加绑定
- 开源默认门槛已收紧到 `admin`
- install/doctor/upgrade 已开始按 Claude 原生安装与 B+C 授权路径收敛
