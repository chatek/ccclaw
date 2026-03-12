# 260312_26_scheduler_observability_closure

## 背景

Issue #26 要求继续收口调度可观测性，重点有三类缺口：

1. `scheduler doctor` 还缺 linger、unit 漂移、最近失败 service 摘要等深诊断
2. `scheduler timers` 只有一张偏宽的表，窄终端与脚本消费都不理想
3. `status / scheduler status / scheduler doctor / scheduler timers` 的术语与查看路径还不够清晰

本轮按 Issue 已拍板范围，只处理 systemd 调度观测与文档收口，不扩展到日志模型或 `scheduler use auto`。

## 实施

### 1. 深化 `scheduler doctor`

- `src/internal/scheduler/doctor.go`
  - 检查项新增：
    - `loginctl`
    - `linger`
    - `unit 漂移`
    - `托管 services`
  - `doctorCheck` 增加 `Repair` 字段
  - 失败项输出统一带 `修复=...`，避免只报错不指路
- `src/internal/scheduler/systemd_inspect.go`
  - 新增 `InspectLingerStatus()`
  - 新增 `DetectManagedUnitDrift()`
  - 新增 `ListManagedServices()`
  - 补 `loginctl` 调用，统一带超时

本轮新增的 systemd 深诊断语义：

- `linger`
  - 明确判断当前用户退出登录后 user timer 是否还能继续跑
  - 未启用时直接给 `loginctl enable-linger <user>` 修复建议
- `unit 漂移`
  - 用当前配置重新生成期望 unit，再与 `systemd_user_dir` 下实物逐个比对
  - 区分 `missing=` 与 `drift=`
- `托管 services`
  - 汇总最近失败或异常退出的托管 service
  - 输出 `active/sub/result/status` 摘要
  - 修复建议直接落到 `systemctl --user status ...` 与 `journalctl --user -u ...`
- `托管 timers`
  - 默认只把真正的“未启用/未运行”视为失败
  - 对“已启用但尚未首次触发”的 timer 仅做提示，不误报故障

### 2. 收口 `scheduler timers` 多视图

- `src/internal/scheduler/timers_view.go`
  - 新增四种输出视图：
    - 默认 `human`
    - `--wide`
    - `--raw`
    - `--json`
  - `ResolveTimersView()` 统一处理冲突 flag
  - `RenderManagedTimers()` 下沉视图渲染，避免 CLI 层拼业务表格
- `src/internal/scheduler/systemd.go`
  - `TimerStatus` 补 `HasNext` / `HasLast`
  - 抽出通用 `showUnitProperties()`
- `src/cmd/ccclaw/main.go`
  - `scheduler timers` 新增：
    - `--wide`
    - `--raw`
    - `--json`

四类视图边界如下：

- 默认视图
  - 面向人看
  - 仅保留 `TASK / ACTIVE / ENABLED / NEXT[cfg_tz] / LAST[cfg_tz]`
  - 解决窄终端爆宽
- `--wide`
  - 面向运维排障
  - 展示 `CAL_RAW / CAL_CFG / NEXT[host] / NEXT[cfg] / LAST[host] / LAST[cfg]`
- `--raw`
  - 面向人工复制、比对和 issue 贴片
  - 使用稳定的 `key=value` 字段名
- `--json`
  - 面向脚本消费
  - 输出 `host_timezone / config_timezone / items[]`

### 3. 文档与术语补齐

- `README.md`
  - 增加 `scheduler timers --wide|--raw|--json`
  - 增加推荐排障路径
- `src/dist/README.md`
  - 增加新视图说明
  - 明确 `scheduler doctor` 新诊断项
- `src/ops/examples/app-readme.md`
- `src/dist/ops/examples/app-readme.md`
  - 补典型查看路径与视图选择说明

## 验证

执行：

```bash
cd src
gofmt -w cmd/ccclaw/main.go cmd/ccclaw/main_test.go internal/scheduler/doctor.go internal/scheduler/doctor_test.go internal/scheduler/systemd.go internal/scheduler/systemd_inspect.go internal/scheduler/timers_view.go
go test ./internal/scheduler
go test ./cmd/ccclaw
go test ./...
```

本轮新增回归覆盖：

- `scheduler timers`
  - 默认视图
  - `--wide`
  - `--raw`
  - `--json`
- `scheduler doctor`
  - linger
  - unit 漂移
  - service 摘要
- `ResolveTimersView()`
- `DetectManagedUnitDrift()`

## 结果

Issue #26 本轮范围已落地：

- `scheduler doctor` 能显式识别 linger、unit 漂移和最近失败 service
- `scheduler timers` 具备人类默认视图、完整视图、原始视图和 JSON 视图
- 默认终端视图明显收窄，不再把完整 unit 名、双时区和原始表达式全塞进一张表
- 文档已给出更清楚的排障顺序

## 剩余风险

1. `scheduler status` 仍保持单行 `key=value` 摘要，没有再拆成独立 `--json`
2. service 异常摘要目前基于 `systemctl show` 属性，还没有下沉到 journal 内容级别
3. `--raw` 视图适合人工排障，但字段集合仍可能随后续 systemd 观测项增加而扩展
