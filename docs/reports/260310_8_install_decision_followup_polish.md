# 260310_8_install_decision_followup_polish

## 背景

对应 Issue：#8 `release:26.3.10.1142 安装失败`

拍板来源：

- Issue 评论：<https://github.com/41490/ccclaw/issues/8#issuecomment-4029090959>

在首轮实现基础上，本轮补做一次输出行为收口，目标不是新增策略，而是让安装器在 `preflight`、`simulate`、调度降级三类场景下的用户感知更准确。

## 本轮调整

### 1. `print_flow` 改为“计划执行流程”

此前无论真实安装、`--simulate` 还是 `--preflight-only`，都会先打印“模拟安装流程”，这会把真实安装和体检模式都说错。

本轮统一改成“计划执行流程”，保持中性描述。

### 2. Claude 授权说明与当前实现对齐

此前流程说明仍写“授权态只识别 setup-token / proxy 两条路径”，但首轮实现已经支持：

- CLI session 复用
- proxy 环境变量
- 手工补录 API Key

本轮把说明同步为当前真实行为，避免误导。

### 3. `print_summary` 按运行模式输出不同结论

此前即使传入：

- `--simulate`
- `--preflight-only`

摘要顶部也会打印“安装完成”，与实际不符。

本轮改为：

- `--preflight-only` -> `体检完成，未写入文件`
- `--simulate` -> `模拟执行完成，未写入文件`
- 默认安装 -> `安装完成`

同时把结果区标题同步调整为：

- `当前主机体检结果`
- `当前主机模拟结果`
- `当前主机部署成果`

### 4. 调度器后续动作按生效模式分支提示

此前即使生效调度器是：

- `cron`
- `none`

摘要仍会先提示用户启用 `systemd --user` timer，再给 `crontab` 样板，顺序和主次都不准确。

本轮改为按 `SCHEDULER_EFFECTIVE` 输出：

- `systemd`：先给 `systemctl --user` 启用命令，再给 `crontab` 兜底样板
- `cron`：先给 `crontab` 样板，再给后续切回 `systemd` 的命令
- `none`：明确当前不自动调度，若需要后台运行可先写 `crontab`

## 验证

本轮验证命令：

```bash
bash -n src/dist/install.sh
src/dist/install.sh --yes --simulate --home-repo-mode init --task-repo-mode remote --task-repo-remote 41490/ccclaw --scheduler auto
src/dist/install.sh --yes --simulate --home-repo-mode local --home-repo /opt/src/ccclaw --task-repo-mode local --task-repo-local /opt/src/ccclaw --scheduler none
src/dist/install.sh --yes --preflight-only --home-repo-mode init --task-repo-mode remote --task-repo-remote 41490/ccclaw --scheduler auto
```

验证结论：

- 脚本语法正常
- `--preflight-only` 不再误报“安装完成”
- `--simulate` 输出保持无落盘，并明确标识为模拟结果
- `none` 模式下摘要优先给出 `crontab` 路径，不再默认先提示启用 `systemd`

## 结论

Issue #8 对应拍板项的主逻辑未改；本轮主要解决的是：

- 输出措辞与真实模式不一致
- 调度器后续动作提示顺序错误

这样至少把安装器在“真实安装 / 体检 / 模拟 / 降级调度”四种路径下的用户感知收口到了同一套语义上。
