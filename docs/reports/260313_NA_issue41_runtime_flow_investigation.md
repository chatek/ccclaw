# 260313_NA_issue41_runtime_flow_investigation

## 背景

2026-03-13 按仓库约束对 [Issue #41](https://github.com/41490/ccclaw/issues/41) 做现场追查，目标有三项：

- 核对普通成员 Issue 在人工批准后的巡查与执行状态
- 梳理当前 `ccclaw` 的整体运转链路与事实源
- 在证据充足后创建新的讨论 Issue，和 `sysNOTA` 明确优化方向

本轮不改运行代码，只做现场取证、机制分析和讨论入口创建。

## 现场对象

- 控制仓库：`41490/ccclaw`
- 当前分支：`main`
- 运行版本：`26.03.13.1547`
- 调度模式：`systemd`
- 调查时间：`2026-03-13 04:10 EDT` 至 `04:23 EDT`

## #41 时间线

### GitHub 侧

- `2026-03-13 08:08:28 UTC`
  - `DU4DAMA` 创建 [#41](https://github.com/41490/ccclaw/issues/41)
  - 标题：`测试,普通用户再发愿`
  - 标签：`ccclaw`
- `2026-03-13 08:09:36 UTC`
  - `ZoomQuiet` 评论 `/ccclaw approve`

### 本机运行态

- `2026-03-13 04:10:15 EDT`
  - `ccclaw-run.service` 启动并输出 `暂无待执行任务`
- `2026-03-13 04:10:17 EDT`
  - `ccclaw-ingest.service` 才记录 `Issue 已入队`
  - `task_id=41490/ccclaw#41#body`
  - `approved=true`
  - `state=NEW`
- `2026-03-13 04:20:17 EDT`
  - 下一轮 `run` 真正开始执行 `#41`
  - 事件流写入：
    - `STARTED: 开始执行任务`
    - `UPDATED: 任务已挂入 tmux 会话 ccclaw-41490_ccclaw_41_body-443d`
  - GitHub Issue 也收到一条 `任务已开始执行` 评论
- `2026-03-13 04:22:15 EDT`
  - `ccclaw-patrol.service` 再次启动
  - 日志显示 `running_tasks=0 sessions=0`
- `2026-03-13 04:22:50 EDT`
  - `ccclaw status --json` 里，`#41` 又显示为 `NEW`
  - 现场不存在任何 `ccclaw-*` tmux 会话
  - `results/41490_ccclaw_41_body.*` 文件已创建，但内容为空

## 当前运转流程梳理

结合 README、源码和现场日志，当前 `ccclaw` 主链路可概括为：

1. `ingest`
   - 周期扫描 control repo 与启用的 target repo
   - 只处理带 `ccclaw` 标签的 open Issue
   - 校验发起者权限与审批评论
   - 将任务快照写入 `~/.ccclaw/var/state.json`
   - 将任务事件追加到 `~/.ccclaw/var/events-YYYY-WNN.jsonl`

2. `run`
   - 从 `state.json` 中挑选 `NEW/FAILED`
   - 再次 `syncIssue`
   - 将任务状态改为 `RUNNING`
   - 启动 tmux 托管执行
   - 回帖 `任务已开始执行`

3. `patrol`
   - 巡查 tmux 会话
   - 对退出/超时/缺失 session 做收口
   - 末尾顺手同步 target repo 与知识仓库

4. `status/doctor`
   - 主要展示调度状态、执行器解析结果与任务快照
   - 当前未把 repo sync 告警与状态面一致性校验纳入主视图

## 已坐实的问题

### 1. 调度存在整点竞态窗口

当前 timer 配置为：

- `ingest = *:0/5`
- `run = *:0/10`
- `patrol = *:0/2`

`04:10` 这一轮里，`run` 先查库后退出，`ingest` 两秒后才把 `#41` 写入 `NEW`。  
直接后果是：新批准任务可能平白多等一个 `run` 周期。`#41` 现场实测就因此延迟了约 10 分钟。

### 2. 状态快照与事件流已经发生并发覆盖

这是本轮最关键的发现。

现场同时成立的事实有：

- `events-2026-W11.jsonl` 已记录 `#41` 的 `STARTED` 与“挂入 tmux 会话”事件
- GitHub 页面上也已有 `RUNNING` 评论
- 但 `state.json` 中 `#41` 仍是 `State=NEW`
- `LastSessionID` 为空
- `UpdatedAt` 仍是零值
- `04:22` 的 `patrol` 看到 `running_tasks=0 sessions=0`

这说明 `run` 与 `ingest` 至少存在一次“各自读到旧快照，再由后写者覆盖前写者”的情况。  
也就是说，当前不是只有调度延迟，而是已经出现了：

- 用户评论、事件流、状态快照三方不一致
- `RUNNING` 状态被覆盖回 `NEW`
- `patrol` 无法接管真实刚发射过的任务
- 后续 `run` 有机会再次重复发射同一任务

### 3. 巡查链路长期带噪声运行

`patrol` 每 2 分钟都会出现同类告警：

- `jj git fetch --remote origin 执行失败`
- `Git does not recognize required option: porcelain`

现场版本为：

- `jj 0.39.0`
- `git 2.39.5`

但与此同时：

- `ccclaw scheduler doctor --json` 为 `13/13 passed`
- `ccclaw status --json` 的 `alerts` 为空

这说明持续发生的巡查级告警还没有被收入口径统一的健康视图。

### 4. 运行态事实源已经切换，但旧 `state.db` 仍在现场

当前真正被 `ccclaw status` 读取的是：

- `state.json`
- `events-*.jsonl`
- `token-*.jsonl`

而 `~/.ccclaw/var/state.db` 里保留的是旧任务和旧 schema。  
如果排障人员按旧习惯直接查 SQLite，会得出和当前真实运行态不同的结论。

### 5. 状态元数据维护不完整

`#41` 在 `state.json` 中的 `UpdatedAt` 为零值，这说明当前快照并没有稳定维护“最后更新时间”。  
这会继续影响：

- 任务排序
- journal 汇总
- 人工排障时序判断

### 6. `DEAD` 终态的 rerun 语义仍不完整

历史样本 [Issue #22](https://github.com/41490/ccclaw/issues/22) 已经证明：

- 任务可以进入 `DEAD`
- 人工可以评论 `/ccclaw rerun`
- 但现版本不会把 `DEAD` 正式恢复到 `NEW/FAILED`

因此，“失败后再试一次”仍缺少稳定入口。

## 当前对 #41 的结论

截至 `2026-03-13 04:23 EDT`，`#41` 的状态不是简单的“未执行”，而是：

- 已通过审批
- 已被 `ingest` 发现
- 首轮执行被调度竞态延迟
- 第二轮 `run` 已经发射并回帖 `RUNNING`
- 但运行态快照被并发覆盖回 `NEW`
- `patrol` 没有看到对应运行中任务
- tmux 会话也未在现场留下可见实例

因此，`#41` 暴露出的核心问题已经从“时序窗口”升级为“状态面并发覆盖”。

## 新建讨论入口

本轮已创建新的讨论 Issue：

- [Issue #42: investigate: #41 批准后延迟执行，且运行态状态面存在并发覆盖与巡查噪声](https://github.com/41490/ccclaw/issues/42)

创建时刻：

- `2026-03-13`

该 Issue 当前未打 `ccclaw` 标签，仅用于和 `sysNOTA` 拍板优化方向，避免误触发自动执行。

## 建议的优化优先级

基于本轮证据，建议按以下顺序讨论和落地：

1. 先收 `state.json` 并发覆盖与状态一致性问题
2. 再收 `ingest/run` 的因果保证
3. 然后拆解或修复 `patrol` 末尾的 repo sync 噪声
4. 同步把运行态 backend 明示到 `README/status/doctor`
5. 最后补齐 `/ccclaw rerun` 的正式语义

## 本轮产物

- 工程报告：本文
- 讨论 Issue：[Issue #42](https://github.com/41490/ccclaw/issues/42)
