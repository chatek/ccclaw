# 260311_10_stats_range_daily_impl

## 背景

Issue #10 在 Phase 3 收尾评论里已经明确提出两个后续补强点：

- 为 `stats` 增加日期范围筛选
- 为 `stats` 增加按天聚合视图

在 Issue #12 已确认 `status` 负责当前快照、`stats` 负责历史聚合后，这两个能力不再存在语义分歧，可以直接实现。

## 本轮实现

### 1. `ccclaw stats` 新增时间窗口参数

CLI 新增：

- `--from YYYY-MM-DD`
- `--to YYYY-MM-DD`
- `--daily`

约束：

- `--from` 表示起始日期，含当日
- `--to` 表示截止日期，含当日；内部按次日零点转为右开区间
- `--from` 不得晚于 `--to`
- `--rtk-comparison` 继续可用，但统计口径自动收敛到同一时间窗口

### 2. 存储层补齐范围查询与 daily 聚合

SQLite 存储层新增：

- `TaskTokenStatsBetween(start, end, limit)`
- `DailyTokenStatsBetween(start, end)`

其中 daily 聚合没有继续依赖 SQLite 内建日期函数，而是：

1. 先按时间窗口取出 `token_usage`
2. 再在 Go 层统一解析时间
3. 按本地日界线聚合

这样可以避开不同时间格式、时区串行化方式不一致时带来的分桶误差。

### 3. 顺手修复了任务事件测试的时间戳脆点

在补范围查询测试时暴露出旧问题：

- `AppendEvent` 始终使用实时 `CURRENT_TIMESTAMP`
- 但 journal / event 相关测试使用固定日期断言

此前只是恰好未踩中边界。本轮补了：

- `AppendEventAt(...)`

默认运行逻辑仍走 `AppendEvent`，不改变生产行为；测试和需要固定时间写入的场景可使用显式时间接口，避免假阳性/假阴性。

## 验证

已执行：

- `go test ./...`

新增覆盖：

- `stats` 的 `--from/--to` 范围解析
- `stats` 的按天聚合输出
- 存储层的范围统计与 daily 聚合
- 任务事件显式时间写入

## 结果

本轮完成后：

- `status` 继续聚焦当前快照
- `stats` 现已支持历史聚合的时间范围与 daily 视图
- `--rtk-comparison` 与范围筛选口径一致

## 后续建议

- 为 `stats` 增加 `--limit`，控制任务表和日聚合的输出规模
- 为 `stats --daily --rtk-comparison` 补每日 RTK/plain 分桶视图
- 为 `journal` 继续衔接月度 / 年度 `summary.md` 自动汇编
