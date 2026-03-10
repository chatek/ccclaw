# 260310_11_phase1_phase2_scheduler_visibility

## 背景

承接 Issue #11 `plan: 安装链路第二轮优化策划`，本轮先按优先级推进：

1. Phase 1：补齐 `install.sh` shell 级回归测试
2. Phase 2：先做最小化的调度器状态可见性收口

用户额外说明：

- `~/.config/systemd/user/` 权限已恢复
- 可先做一次 user 级 systemd unit 的部署/启动/清理测试

## 本轮完成

### 1. 先做了 user systemd 真实环境验证

未直接使用正式 `ccclaw` unit，而是临时写入一个 `ccclaw-probe-*` user service/timer：

- 写入 `~/.config/systemd/user/`
- `systemctl --user daemon-reload`
- 手工 `start service`
- `enable --now timer`
- 观察 service 被 timer 至少触发一次
- 最后删除 unit、清理探针文件并 reload

结论：

- 当前主机 user systemd 真实部署与启动能力可用
- 环境已不再受此前目录权限问题阻塞

### 2. Phase 1：新增 shell 回归测试入口

新增：

- `src/tests/install_regression.sh`
- `src/Makefile` 新增 `test-install`

测试策略：

- 纯 shell 驱动，不引入额外测试框架
- 统一使用临时目录
- 不触碰 `/opt/ccclaw`
- 不写入真实 `~/.config/systemd/user`
- 不修改现有 crontab

当前覆盖场景：

1. 首装
2. 幂等重装
3. systemd 降级体检
4. local 仓库无 `origin` 的失败路径
5. remote 仓库路径越界的失败路径

### 3. 顺手修复了一个会阻塞后续阶段的根因

最初 `make test-install` 无法工作，不是测试脚本本身的问题，而是 `src/Makefile` 仍然调用 PATH 中的旧版 `go`：

- 默认 `go` -> `/usr/bin/go` -> `go1.19.8`
- 实际可用新版本 -> `/usr/local/go/bin/go` -> `go1.25.7`

由于仓库 `go.mod` 当前写的是 `go 1.25.7`，旧版 `go` 会直接拒绝解析，导致：

- `make build`
- `make test`
- `make dist-sync`
- `make test-install`

全部被连带阻塞。

本轮已收口：

- `src/Makefile` 增加 `GO` / `GOFMT` 变量
- 默认优先使用 `/usr/local/go/bin/go` 与 `/usr/local/go/bin/gofmt`
- `test-install` 恢复依赖 `dist-sync`，确保安装回归始终跑在最新 release 树上

### 4. Phase 2：先补最小调度器状态可见性

本轮未进入 `cron` 自动写入，只先解决“doctor 看不出当前调度状态”的问题。

已落地：

- `config.toml` 新增 `[scheduler]`
  - `mode`
  - `systemd_user_dir`
- `install.sh` 生成配置时会把请求调度模式和 unit 目录写入配置
- `config example` 已同步补充 `scheduler` 配置段
- `ccclaw doctor` 将原先的 `systemd timers` 检查收口为 `调度器`
- `doctor` 现在会输出：
  - `request=...`
  - `effective=...`
  - `reason=...`
  - 必要时追加 `repair=...`

当前判定逻辑：

- 检查 user systemd timer 是否启用且运行
- 检查受控 crontab 的 ingest/run 规则是否存在
- 输出当前有效模式：`systemd | cron | none`
- 若出现降级或双重调度，会在 doctor 中标红失败

## 额外发现

在自动化回归中还发现一个现实差异：

- `install.sh` 的 user systemd 探查仍依赖 `systemctl --user show-environment`
- 该命令在当前非交互环境下可能误报 `No medium found`
- 但真实的 `service/timer` 部署、启动、状态查询是可用的

这说明安装期 `preflight` 的 user systemd 探查条件仍偏硬，后续还应继续收口，避免把“可用环境”误判成“不可用环境”。

## 验证

本轮直接执行的验证包括：

### 环境与 systemd

```bash
systemctl --user --version
stat -c '%U %G %a %n' ~/.config/systemd/user
```

以及一次临时 user unit 的真实部署/启动/清理验证。

### Go 与 Makefile

```bash
/usr/local/go/bin/go version
make fmt
make test
```

### 安装回归

```bash
make test-install
```

### doctor 抽查

在临时安装目录执行：

```bash
./dist/bin/ccclaw doctor --config <tmp>/app/ops/config/config.toml --env-file <tmp>/app/.env
```

抽查结果中已出现：

```text
[ OK ] 调度器: request=none effective=none reason=未检测到受控 crontab 规则
```

## 结论

本轮已完成：

- Phase 1 全量落地并接入统一入口
- Phase 2 的调度器状态可见性最小实现
- Makefile 的 Go 工具链根因修复

下一轮可继续按 Issue #11 既定顺序推进：

1. 收口 install preflight 的 user systemd 探查误判
2. 继续补 `doctor` 的降级原因与修复建议细节
3. 进入 Phase 3：`cron` 受控写入与更新
