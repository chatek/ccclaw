# 260312_24_observer_runtime_logging_followup

## 背景

按仓库执行约束，先使用 `gh` 读取了 Issue #24：

- 议题：`plan: scheduler 日志级别产品化与运行态统一输出`
- 地址：<https://github.com/41490/ccclaw/issues/24>

Issue 最新评论已明确前一轮主实现完成，并给出 4 条后续优化建议。本轮未发现新的拍板评论，因此按“最小风险、直接延续既有方向”的顺序，继续推进：

1. 把 `status/doctor/config` 等观测型命令接入统一运行态 logger
2. 给运行态 logger 增加固定字段约束，降低后续日志消费成本

本轮不触碰 JSON 日志模式与归档格式扩展，避免把一次跟进扩成新的设计决策。

## 本轮实现

### 1. 观测型命令补齐统一运行态日志

已在以下入口补充内部运行日志，且继续保持面向用户的主结果输出不变：

- `src/internal/app/runtime.go`
  - `Status(...)` 新增“开始生成运行态快照 / 快照已输出”日志
  - `Status(...)` 对任务列表、token 统计、RTK 对比、最近事件读取失败补 error 日志
  - `Status(...)` 对 tmux 会话采集结果补 debug 日志
  - `ShowConfig(...)` 新增“开始导出配置快照 / 配置快照已输出”日志
  - `ShowConfig(...)` 对配置序列化和输出失败补 error 日志
  - `Doctor(...)` 在每个诊断项前后补 debug 日志，在失败项与最终失败收口时补 warning 日志

这样 `status/config/doctor` 的内部执行轨迹已经和 `ingest/run/patrol/journal` 共用同一套 logger 入口，不再继续散写运行态信息。

### 2. 固定字段顺序与键名约束

已在 `src/internal/app/runtime_logging.go` 增加统一字段归一逻辑：

- 固定字段集合收口为：
  - `entry`
  - `task_id`
  - `issue`
  - `target_repo`
  - `session_id`
- 以上字段即使调用侧传参顺序不同，最终输出仍按固定顺序落盘
- 非固定字段继续原样追加，不改变现有业务语义

这次没有强行要求所有日志都必须携带完整固定字段，但已经先把字段名和顺序的约束底座补齐，后续要继续推广时不必再重复改 handler 逻辑。

## 测试

执行：

```bash
cd src
go test ./...
```

新增覆盖：

- `src/internal/app/runtime_logging_test.go`
  - `TestStatusLogsRespectRuntimeLevel`
  - `TestConfigLogsRespectRuntimeLevel`
  - `TestRuntimeLogsNormalizeFixedFieldOrder`

验证点：

- `status/config` 运行态日志也遵守 `debug|info|warning|error` 阈值
- 观测型命令日志会带上统一 `entry`
- 固定字段即使乱序传入，也会按约定顺序输出

## 结果

本轮完成后，Issue #24 评论里第 1 条建议已落下主体实现，第 2 条建议完成了“统一字段约束底座”这一关键前置。

当前仍待后续拍板或继续推进的内容：

1. 是否把 `scheduler doctor`、`scheduler status` 等调度子命令也接入同一运行态 logger
2. 是否把固定字段从“有则规范输出”进一步升级为“关键入口必须带齐”
3. 是否增加 `json` 日志输出模式
4. 日志归档是否直接复用固定字段语义，而不只保留 journald 文本
