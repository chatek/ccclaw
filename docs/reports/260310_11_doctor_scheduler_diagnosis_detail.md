# 260310_11_doctor_scheduler_diagnosis_detail

## 背景

承接 Issue #11 `plan: 安装链路第二轮优化策划`，上一轮已完成安装期 `preflight` 对 user systemd 误判的收口，本轮继续按既定顺序推进下一项：

1. 补 `doctor` 对 systemd 降级原因与修复建议的细节输出
2. 再进入 Phase 3：`cron` 受控写入与更新

执行前继续以 `gh` 核查 Issue 作者 `ZoomQuiet` 在仓库中的协作权限，当前仍为 `admin`，满足自动执行门禁。

## 问题复盘

上一轮虽然已经把 `doctor` 的调度器检查统一收口为：

- `request=...`
- `effective=...`
- `reason=...`
- `repair=...`

但当前 `reason/repair` 仍然偏粗，主要问题有三类：

1. systemd 失败原因过于扁平
- “unit 已写入但 timer 未启用”
- “timer 已启用但当前未运行”
- “当前会话无法连接 user bus”
- “系统缺少 systemctl”

这些情况此前都可能只落成一段原始错误字符串，用户需要自己反推下一步动作。

2. cron 失败原因缺少环境区分
- “未找到 crontab”
- “当前没有 crontab”
- “已有 crontab，但没有受控的 ccclaw 规则”

这些情况的修复动作并不相同，但此前只给出一个很泛的 repair 提示。

3. 失败上下文不够完整
- 出现降级、双重调度或单元已写入但未激活时
- `doctor` 之前只输出主结论
- 不会同时带出 `systemd` / `cron` 各自观测到的底层状态

这会让用户在排障时仍然需要二次猜测。

## 本轮改动

### 1. 在 `runtime.go` 中拆出调度诊断层

本轮把 `summarizeScheduler()` 中原本混在一起的判断拆成了几个辅助函数：

- `diagnoseScheduler()`
- `summarizeInstalledButInactiveSystemd()`
- `summarizeUnavailableScheduler()`
- `systemdRepairHint()`
- `cronRepairHint()`
- `schedulerContext()`

这样 `doctor` 仍保持原有输出外形，但内部可以按失败来源给出更准确的文案。

### 2. systemd 失败改成按场景给 repair

当前已显式区分：

- 已写入 unit，但 `is-enabled` 失败
  - 输出“timer 尚未启用”
  - repair 指向 `daemon-reload && enable --now`
- 已写入 unit，但 `is-active` 失败
  - 输出“timer 当前未处于运行态”
  - repair 指向 `restart` 与 `journalctl --user`
- 当前会话无法连接 user bus
  - 输出“当前会话无法连接 user systemd 总线”
  - repair 明确要求在用户登录会话中执行 `systemctl --user ...`
- 缺少 `systemctl`
  - 输出“当前环境缺少 systemctl，无法使用 user systemd”
  - repair 明确提示安装 systemd 组件或改用 `cron/none`

### 3. cron 失败改成按环境给 repair

当前已显式区分：

- 缺少 `crontab`
  - repair 提示安装 `cron/cronie` 或切回 `systemd/none`
- 当前没有 crontab
  - repair 提示按安装摘要补充受控规则
- 已有 crontab 但未检测到受控规则
  - repair 同样明确为补齐 `ingest/run` 两条规则

### 4. 在需要时追加上下文

以下场景会额外附带：

- `systemd=<...>`
- `cron=<...>`

典型触发条件：

- 自动调度降级
- 同时检测到 `systemd+cron`
- systemd unit 已写入但未激活
- 请求模式与实际生效模式不一致

这样主结论保持简洁，但底层观测值也不会丢失。

## 测试补充

本轮在 `src/internal/app/runtime_test.go` 中新增或强化了以下断言：

1. `auto` + user bus 不可达
- 断言 reason 会明确为“当前会话无法连接 user systemd 总线”
- 断言 repair 会提示在用户登录会话中执行 `systemctl --user ...`

2. `systemd` + unit 已写入但未启用
- 断言 reason 为“timer 尚未启用”
- 断言 detail 中会带出原始 `systemd=...` 上下文

3. `auto` + 双重调度
- 断言 detail 中同时包含 `systemd=...` 与 `cron=...`

4. `cron` + 缺少 `crontab`
- 断言 reason 明确指出环境缺少 `crontab`
- 断言 repair 提示安装 `cron/cronie` 或改用其他调度方式

## 验证

本轮执行：

```bash
cd src
/usr/local/go/bin/go test ./internal/app
make test
```

结果通过。

## 结论

本轮完成后，`doctor` 的调度器诊断已经从“最小可见性”推进到“可定位、可操作”的层级：

- 用户能直接区分是 bus、timer、命令缺失还是 cron 规则缺失
- 修复建议开始对应具体失败面，而不是统一泛化为一条提示
- 在降级和冲突场景下，systemd/cron 的底层观测值也能一并看到

按 Issue #11 的既定顺序，下一步可以继续进入：

1. Phase 3：`cron` 受控写入与更新
