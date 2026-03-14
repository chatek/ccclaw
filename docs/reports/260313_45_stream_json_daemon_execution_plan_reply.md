# 260313_45_stream_json_daemon_execution_plan_reply

## 背景

用户要求基于 `docs/assay/260313-go-stream-json-daemon-vs-tmux.md` 的调研结论，回复 Issue [#45](https://github.com/41490/ccclaw/issues/45)，细化“以 Go stream-json daemon 作为主执行方向”的实施计划，并明确需要拍板的具体决策点。

## 前置核查

本轮先执行 Issue 决策核查，再输出方案：

1. 使用 `gh issue view 45 --comments --json ...` 确认 #45 当前无评论、无新拍板冲突。
2. 复核调研文档 `docs/assay/260313-go-stream-json-daemon-vs-tmux.md` 的推荐结论与风险清单。
3. 下钻现有执行链路关键代码，确保计划与现状对齐：
   - `src/internal/executor/executor.go`
   - `src/internal/tmux/manager.go`
   - `src/internal/app/runtime_cycle.go`
   - `src/internal/app/runtime.go`
   - `src/internal/config/config.go`
   - `src/internal/adapters/storage/slot_store.go`
   - `src/ops/config/config.example.toml`

## Issue 回帖内容摘要

已在 Issue #45 发布细化实施方案评论：

- 评论链接：<https://github.com/41490/ccclaw/issues/45#issuecomment-4056643870>

方案要点：

1. 目标与不变项
   - 主链路迁移为“结构化事件流驱动状态机”。
   - Issue 门禁、`FINALIZING` 语义、`docs/reports` 交付保持不变。
   - 兼容保留 `result/diag/log/meta`，避免一次性破坏运行面。

2. 分阶段实施
   - `Phase 0`：事件契约冻结（事件集、映射规则、落盘格式、解析单测）。
   - `Phase 1`：tmux 载体下启用 `stream-json`（影子对账，不切执行器）。
   - `Phase 2`：daemon 内核单仓灰度（新增 `executor.mode`，仓库级回滚）。
   - `Phase 3`：默认切换与职责收缩（tmux 降级为 debug 能力）。

3. 验收与回滚
   - 每阶段定义门槛（判定一致率、卡槽位指标、观察窗口）。
   - 每阶段保留配置级回滚路径，确保可快速退回 tmux。

4. 风险控制
   - 事件语义偏差、上下文重启回退、人工排障体验下降三类风险均给出控制策略。

## 提醒拍板的具体决策

回帖中已明确提醒需在 Issue 先拍板的 6 项决策：

1. 是否采用 `executor.mode=tmux|daemon` 且支持 repo 级灰度覆盖。
2. 是否接受 Phase 1 先在 tmux 下切 `stream-json` 做对账。
3. Phase 2 是否继续复用现有 hook/transcript 上下文重启判定。
4. Phase 3 是否将 `patrol` 降级为补偿与健康检查。
5. tmux 是否降级为 `debug attach`，退出默认执行载体。
6. 默认切换闸门是否采用“单仓 7 天 + 全局 14 天”观察窗口。

## 本轮产出

1. 完成 #45 实施计划回帖（含阶段路线、验收、回滚、风险、拍板项）。
2. 完成本工程报告归档。
3. 未修改运行代码与发布资产。
