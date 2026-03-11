# 260310_10_stats_limit_journal_summary_resume_rtk

## 背景

根据 Issue #10 最新回复中保留的后续项，本轮继续补齐以下优化：

- `stats --limit`
- `stats --daily --rtk-comparison` 的每日 RTK/plain 分桶
- `rtk_enabled` 从执行路径推断升级为 wrapper 显式埋点
- `journal` 月度/年度/总览 `summary.md`
- resume 失败后降级为新执行的显式事件

同时参考了 Issue #12 对 `status/stats/journal` 观测闭环的上层语义约束，保持 `status` 看当前、`stats` 看历史、`journal` 负责沉淀。

## 实现

### 1. stats 输出规模与每日 RTK 对比

- `ccclaw stats` 新增 `--limit`
  - 限制任务明细表输出条数
  - 限制 `--daily` 与 `--daily --rtk-comparison` 的天数输出规模
  - 保持最近记录优先
- 存储层新增 `DailyRTKComparisonBetween(...)`
  - 按天分桶统计 RTK/plain runs
  - 输出各自平均成本、平均 token 与估算节省比例
- `Runtime.StatsWithOptions(...)` 统一使用 `StatsOptions.Limit`

### 2. journal 自动汇总

- `ccclaw journal` 在生成日报后，继续自动刷新：
  - `kb/journal/YYYY/MM/summary.md`
  - `kb/journal/YYYY/summary.md`
  - `kb/journal/summary.md`
- summary 文件使用受控 managed/user 区块
  - 自动更新 managed 区块
  - 保留 user 区块人工补充空间
- 汇总内容包含：
  - 任务触达/完成/失败/僵死聚合
  - token/cost 聚合
  - RTK/plain 聚合概览
  - 日报/月报/年报入口链接

### 3. resume 降级显式事件

- 当任务基于 `last_session_id` 发起恢复且执行失败时：
  - 追加 `WARNING` 事件，明确“后续重试将降级为新执行”
  - 清空 `last_session_id`
- 这样下一次重试不会继续复用损坏 session，避免同一坏 session 反复恢复

### 4. wrapper 显式 RTK 埋点

- executor 为每次执行注入 `CCCLAW_RTK_MARKER_FILE`
- `ccclaude` wrapper 在真正 `exec` 前写入标记：
  - 走 `rtk proxy claude` 时写 `1`
  - 直连 `claude` 时写 `0`
- executor 优先读取 marker，再回退到旧推断逻辑
- 统计链路因此不再依赖 wrapper 路径名是否碰巧包含 `rtk`

## 验证

- `go test ./...`
- `bash src/tests/install_regression.sh`

## 结果

- `stats` 已具备可控输出规模和每日 RTK/plain 趋势视图
- `journal` 已从“仅日报”升级为“日报 + 月汇总 + 年汇总 + 总览”
- resume 失败后的降级行为有了显式事件留痕
- RTK 统计口径从路径猜测收紧到 wrapper 实际执行结果
