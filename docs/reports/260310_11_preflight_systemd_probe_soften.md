# 260310_11_preflight_systemd_probe_soften

## 背景

承接 Issue #11 `plan: 安装链路第二轮优化策划` 的当前顺序，本轮继续推进上一条评论中列出的下一项：

1. 先修 `preflight` 的 user systemd 误判
2. 再继续补 `doctor` 的降级原因与修复建议细节
3. 然后进入 Phase 3：`cron` 受控写入与更新

执行前已按仓库门禁用 `gh api` 动态核查 Issue 作者 `ZoomQuiet` 在 `41490/ccclaw` 中的权限，返回为 `admin`，满足“管理员 Issue 自动执行”的约束。

## 问题复盘

此前 `src/dist/install.sh` 中的 `probe_systemd_user()` 采用了偏硬的判断：

- 只要 `systemctl --user show-environment` 失败
- 就直接把 user systemd 视为不可用
- 进而把 `--scheduler auto` 降级为 `none`
- 或让 `--scheduler systemd` 直接失败

但上一轮已经做过真实 user unit 验证，确认存在一类环境差异：

- 当前会话里 `systemctl --user show-environment` 可能报 `No medium found`
- 但 user unit 目录本身可写
- 安装脚本实际只需要写入 unit 文件，并不会在安装期强行 `enable --now`
- 用户后续在登录会话中手工执行 `daemon-reload` / `enable --now` 仍然可行

因此根因不是“没有 systemd 能力”，而是把“当前会话是否能直连 user bus”误当成了“是否允许按 systemd 模式部署”。

## 本轮改动

### 1. 拆分两类状态

在 `src/dist/install.sh` 中，把原先混在一起的 user systemd 判断拆成了两层：

- `SYSTEMD_READY`
  - 表示是否满足 unit 部署条件
  - 关注 `systemctl` 是否存在、unit 目录归属是否正确、目录是否可写/可创建
- `SYSTEMD_CONTROL_READY`
  - 表示当前会话是否能直接访问 user bus
  - 仍通过 `systemctl --user show-environment` 做软探测

这样 `show-environment` 失败时，不再直接推翻“可部署”结论。

### 2. 调整调度判定语义

`decide_scheduler()` 现在的行为变为：

- `auto`
  - 只要 user unit 可部署，就保持生效模式为 `systemd`
  - 若当前会话未直连 user bus，则在原因中明确提示“安装后需手工启用 timer”
- `systemd`
  - 只要 unit 可部署，就允许继续安装
  - 不再因为当前会话的 bus 软失败而直接报错
- `none|cron`
  - 维持原逻辑不变

仍然保留真正不可部署时的失败或降级：

- 未安装 `systemctl`
- unit 目录属主异常
- unit 目录不可写且不可创建

### 3. 收口体检输出

`print_preflight()` 现在会区分：

- `OK: user systemd 可部署，当前会话可直接控制`
- `WARN: user systemd 单元可部署，但当前会话未直连 user bus；安装后需在登录会话中手工启用 timer`

也就是把“软失败”从“误降级”改成“明确提醒”。

### 4. 补回归测试

在 `src/tests/install_regression.sh` 中新增了一个伪造 `systemctl` 的场景：

- `systemctl --user show-environment` 固定失败并输出 `No medium found`
- 但 `XDG_CONFIG_HOME` 指向可写沙箱目录

断言目标：

- `--scheduler auto` 体检后应为 `请求=auto, 生效=systemd`
- 输出中必须包含“未直连 user bus”和“手工启用 timer”

同时保留原有只读目录降级用例，继续验证真正不可部署时会回到 `none`。

## 验证

本轮执行：

```bash
cd src
make test-install
```

结果通过，安装回归当前覆盖 6 类场景：

1. 首装
2. 幂等重装
3. systemd 降级体检
4. systemd 无 user bus 但允许部署
5. local 无 `origin` 失败路径
6. remote 路径越界失败路径

## 结论

本轮已把安装期 `preflight` 的 user systemd 误判收口到更准确的语义：

- “当前会话不能直接控 timer” 不再等于 “不能按 systemd 模式安装”
- 安装器会保留 `systemd` 结论，同时把后续动作明确提示给用户
- 真正不可部署的环境仍会继续降级或失败

按 Issue #11 的既定顺序，下一步可以继续推进：

1. 补 `doctor` 对 systemd 降级原因与修复建议的细节输出
2. 再进入 Phase 3 的 `cron` 受控写入与更新
