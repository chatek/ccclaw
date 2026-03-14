# 260314_40_sevolver_stats_status_task_class_metrics

## 背景

Issue #40 需要在 `stats/status` 中补齐 sevolver 自进化链路可观测性，重点是：

- 按 `task_class` 展示 `sevolver` 与 `sevolver_deep_analysis`
- 增加升级、收敛、backlog、关闭时长、成功/失败率、资源成本指标
- 明确窗口口径（日/周/指定范围）
- 保持 human readable 与 JSON 输出兼容

## 口径定义

本次统一口径由 `storage` 层单点提供，`stats` 与 `status` 仅消费，不再各自实现并行统计逻辑。

- 升级数：窗口内 `CREATED` 事件去重任务计数
- 收敛数：窗口内 `DONE` 终态任务计数
- 未收敛 backlog：当前状态不在 `DONE/DEAD` 的任务数
- 平均关闭时长：窗口内终态任务的 `(终态事件时间 - 任务创建时间)` 平均值
- 成功率/失败率：窗口内终态任务中 `DONE/DEAD` 占比
- 资源成本：窗口内 token runs、input/output/cache 与 USD 累计

## 主要变更

### 1) storage 统一聚合能力

文件：`src/internal/adapters/storage/store.go`

新增：

- `TaskClassFlowStat`
- `TaskClassFlowSnapshot`
- `TaskClassFlowStatsBetween(start, end, classes)`

实现方式：

- 复用现有 `state + events + tokens` 数据
- 先按 `task_class` 初始化聚合桶，再汇总 backlog/token/event/终态时长
- 返回按调用方指定 class 顺序排列的结果

### 2) stats 接入 task_class 聚合

文件：`src/internal/app/runtime.go`

新增能力：

- 在 `stats` 输出末尾新增 `Sevolver 自进化链路聚合` 区块
- 固定展示三种窗口：
  - `day`：最近 24 小时
  - `week`：最近 7 天
  - `range`：`--from/--to` 指定范围（未指定时显示全量）
- 聚合维度固定为：
  - `sevolver`
  - `sevolver_deep_analysis`

兼容处理：

- 即便当前无 token 记录，也会输出 task_class 聚合区块，避免自进化链路观测被 token 视图短路

### 3) status 接入 task_class 聚合（human + json）

文件：`src/internal/app/runtime.go`

新增：

- human：`Sevolver 自进化链路聚合` 区块
- json：根级新增 `task_class` 字段（窗口数组），包含窗口名、标签、边界与 class 聚合数据

保持兼容：

- 原有 `status --json` 字段不变，仅新增字段

## 测试

### 新增/更新测试

- `src/internal/adapters/storage/sqlite_test.go`
  - `TestTaskClassFlowStatsBetween`
- `src/internal/app/runtime_test.go`
  - `TestStatsWithDateRangeAndDailyRendersSections` 增加新聚合区块断言
  - `TestStatusRendersRuntimeSnapshot` 增加人类可读聚合区块断言
  - `TestStatusJSONRendersRuntimeSnapshot` 增加 `task_class` JSON 字段断言

### 执行结果

在 `src/` 下执行：

- `go test ./internal/app ./internal/adapters/storage ./cmd/ccclaw`

结果：全部通过。

## 影响评估

- 对外命令行为：增强型兼容，未移除旧字段/旧输出
- 数据来源：仅复用既有状态库与事件/token 记录，无新增外部依赖
- 风险点：历史数据若缺失 `CreatedAt`，平均关闭时长会回退为 `-`（已做容错）
