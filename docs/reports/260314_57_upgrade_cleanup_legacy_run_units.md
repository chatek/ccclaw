# 260314_57_upgrade_cleanup_legacy_run_units

## 背景

- 对应 Issue：#57 `bug: 升级后未清理旧 ccclaw-run.timer/run.service`
- 操作日期：2026-03-14（US/Eastern）
- Issue 已拍板事项：
  1. 接受在正常升级时自动 `disable --now ccclaw-run.timer` 并删除旧 unit 文件
  2. 接受当前会话无法直连 user bus 时，先删除本地 unit 文件，再输出精确手工清理命令
  3. 接受在 `doctor` 中将旧 `ccclaw-run.*` 遗留提升为 `WARN`

## 根因确认

问题不在“新版本仍然生成 `ccclaw-run.*`”，而在“升级路径没有主动收缩旧 unit”：

- `src/internal/scheduler/systemd.go`
  - `installManagedSystemdUnits()` 只写当前受管 unit
  - `EnableSystemd()` / `RestartSystemd()` 只操作当前 5 个 timer
  - `DisableSystemd()` 才会删除本地 unit 文件
- `src/dist/install.sh`
  - systemd 安装/升级时只做 `daemon-reload + enable/restart`
  - 没有升级前置的旧 `ccclaw-run.*` 清理步骤

因此，历史安装过 `run.timer` 的主机，在升级后仍会残留：

- `ccclaw-run.timer`
- `ccclaw-run.service`

## 本次实现

### 1. Go 侧 systemd 链路补遗留收缩

修改 `src/internal/scheduler/systemd.go`：

- 增加 `legacySystemdUnits()` / `legacySystemdTimers()`，显式声明旧 `ccclaw-run.*`
- `EnableSystemd()` 在写入当前 unit 后，先清理遗留 `ccclaw-run.*`
- 若当前会话无法直连 user bus：
  - 不再直接失败
  - 先删除本地遗留 unit 文件
  - 返回登录会话中的精确手工命令
- `DisableSystemd()` 与本地 unit 删除逻辑一并覆盖旧 `ccclaw-run.timer`

结果是：

- CLI `scheduler use systemd`
- 任何复用 `EnableSystemd()` 的路径

都会在启用当前 5 个 timer 之前先处理历史遗留 `run.timer`

### 2. `doctor` 增加真正的 WARN 语义

修改 `src/internal/scheduler/doctor.go`：

- `doctorCheck` 新增 `Warn`
- `doctorSummary` 新增 `warned`
- JSON 输出新增：
  - `summary.warned`
  - `checks[].level`
  - `checks[].warning`
- 文本输出新增 `[WARN]`

同时新增 `遗留 run 单元` 检查：

- 若 `systemd_user_dir` 下发现 `ccclaw-run.timer` / `ccclaw-run.service`
  - 输出 `WARN`
  - 给出精确清理命令
- 若未发现
  - 输出 `[ OK ] 遗留 run 单元`

这样可以把“现网残留”显式暴露出来，但不把它与真正的致命失败项混为一谈。

### 3. 安装器升级路径补清理步骤

修改 `src/dist/install.sh`：

- 新增 `cleanup_legacy_user_systemd_run_units()`
- 在 `activate_user_systemd_timers()` 前置执行：
  - user bus 可用时：`systemctl --user disable --now ccclaw-run.timer || true`
  - 无论是否直连 user bus，都删除本地 `ccclaw-run.timer/service`
- 将遗留清理结果并入 `SYSTEMD_ACTIVATION_REASON`
- 在无 user bus 的手工提示中，明确加入：
  - `systemctl --user disable --now ccclaw-run.timer || true`
  - `systemctl --user daemon-reload`
  - `systemctl --user enable --now ...`

这使安装器升级路径和 Go 侧 scheduler 切换路径的行为保持一致，不再出现“双路径口径分叉”。

## 回归测试

### 1. Go 测试

执行：

```bash
cd /opt/src/ccclaw/src
go test ./internal/scheduler ./cmd/ccclaw
```

结果：通过

新增覆盖：

- `TestEnableSystemdRemovesLegacyRunUnits`
- `TestEnableSystemdFallsBackWithoutUserBus`
- `TestDoctorJSONWarnsOnLegacyRunUnits`

并同步更新 `scheduler doctor` 的 CLI 输出断言。

### 2. 安装器定向回归

执行了本次改动直接影响的两条场景：

```bash
cd /opt/src/ccclaw/src
bash -lc 'source <(awk '\''/^main\(\)/{exit} {print}'\'' tests/install_regression.sh); ROOT_DIR=$(pwd); INSTALL_SCRIPT="$ROOT_DIR/dist/install.sh"; BIN_PATH="$ROOT_DIR/dist/bin/ccclaw"; prepare_dist; test_systemd_install_auto_enable_and_restart'

cd /opt/src/ccclaw/src
bash -lc 'source <(awk '\''/^main\(\)/{exit} {print}'\'' tests/install_regression.sh); ROOT_DIR=$(pwd); INSTALL_SCRIPT="$ROOT_DIR/dist/install.sh"; BIN_PATH="$ROOT_DIR/dist/bin/ccclaw"; prepare_dist; test_systemd_preflight_accepts_busless_deploy'
```

结果：通过

覆盖点：

- systemd 安装/重装时会清掉旧 `ccclaw-run.*`
- 无 user bus 时仍允许部署，并给出手工收口提示

### 3. 全量安装器回归现状

额外执行：

```bash
cd /opt/src/ccclaw/src
bash tests/install_regression.sh
```

结果：

- 本次相关场景均已在前半段通过
- 后续在既有 `cron` 断言处失败：
  - `test_cron_install_update_and_remove`
  - 失败信息为脚本仍断言 crontab 中存在 `ccclaw run --config ...`

该失败与本次 `systemd legacy run unit` 清理改动无直接耦合，本轮未顺手扩改，避免把另一个未拍板问题混入 #57。

## 结果

- 升级/启用 systemd 时，旧 `ccclaw-run.timer/run.service` 现在会被主动收口
- 无 user bus 场景不再硬失败，而是按 Issue 拍板降级为“本地先清理 + 输出精确手工命令”
- `scheduler doctor` 能把旧 `ccclaw-run.*` 以 `WARN` 形式显式暴露出来

## 后续观察点

- 若现网还存在“文件已删，但 systemd 内存态仍保留 failed unit”的个别主机，需要确认仅靠 `disable --now + daemon-reload` 是否已足够收口；若仍有残留，再单开 Issue 讨论是否引入 `reset-failed`
- `tests/install_regression.sh` 中 `cron` 仍断言 `run` 命令，建议另开 Issue 对齐当前 phase0.5 调度基线
