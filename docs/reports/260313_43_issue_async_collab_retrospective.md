# 260313_43_issue_async_collab_retrospective

## 背景

本轮目标不是修代码，而是回溯 Issue [#43](https://github.com/41490/ccclaw/issues/43) 被 `ccclaw` 感知、执行、反馈的真实过程，并结合 [#42](https://github.com/41490/ccclaw/issues/42) 已完成改造，判断当前“通过 Issue 进行异步协作”的流程到底到了哪一步。

结论先行：

- `#42` 修掉的“批准后要再等一个 `run` 周期才能发射”的问题，本轮已经被 `#43` 现场验证为已收口
- 但 `#43` 也同时暴露出新的收口缺口：`tmux` 会话在极早期消失后，`patrol` / `ingest` 都会因 `no server running` 直接失败
- 因此，当前流程只能确认“感知 + 入队 + 发射 + RUNNING 回帖”已可用，不能确认“执行完成后的自动收口与最终反馈”已可用

## 现场时间线

### GitHub 侧

- `2026-03-13 16:12:30 UTC`：普通成员 `DU4DAMA` 创建 Issue [#43](https://github.com/41490/ccclaw/issues/43)，带 `ccclaw` 标签
- `2026-03-13 16:13:35 UTC`：成员 `ZoomQuiet` 评论 `/ccclaw approve`
- `2026-03-13 16:15:17 UTC`：Issue 自动回帖：
  - `任务已开始执行`
  - `状态: RUNNING`
  - `tmux 会话: ccclaw-41490_ccclaw_43_body-445b`

这一步已经证明：

- 审批门禁工作正常
- `ingest` 在批准后同轮就能发射任务
- `#42` 推进的“按仓 ingest cycle”已经替代旧的 `ingest -> run` 分离模式

### 本机事件流

`~/.ccclaw/var/events-2026-W11.jsonl` 中可见：

- `2026-03-13T16:15:17.441341047Z`
  - `CREATED`
  - `任务入队，author=DU4DAMA permission=read approved=true class=general target=41490/ccclaw`
- `2026-03-13T16:15:17.44459516Z`
  - `STARTED`
  - `开始执行任务`
- `2026-03-13T16:15:17.468341503Z`
  - `UPDATED`
  - `任务已挂入 tmux 会话 ccclaw-41490_ccclaw_43_body-445b`

这说明：

- 感知 -> 入队 -> 发射 -> 回帖 这段链路是闭合的
- `#42` 中“状态主写入口收敛到 ingest cycle”的方向至少在发射前半段已经生效

### 运行态快照

`2026-03-13 12:17:40 EDT` 执行 `~/.ccclaw/bin/ccclaw status --json` 的结果显示：

- `tasks.counts.RUNNING = 1`
- `tasks.items[0] = 41490/ccclaw#43#body`
- `sessions.running_tasks = 1`
- `sessions.total_sessions = 0`
- `sessions.missing_sessions = 1`
- `slots.items[0].phase = running`
- `slots.items[0].current_step = execute`

同时：

- `~/.ccclaw/var/runtime/41490_ccclaw.json` 仍停在：
  - `phase = running`
  - `current_step = execute`
- `~/.ccclaw/log/41490_ccclaw_43_body.log` 为空

这说明：

- 主状态和 sidecar 仍认为任务在运行
- 但 tmux 会话实际上已经不存在
- 执行器没有留下 stdout/诊断输出，至少在当前现场中无法还原 Claude 真正执行到了哪一步

## 关键失败点

### 1. `patrol` 无法把 `no server running` 视为“会话已不存在”

`journalctl --user -u ccclaw-ingest.service -u ccclaw-patrol.service` 显示：

- `2026-03-13 12:16:15 EDT`
  - `patrol` 执行 `tmux list-panes -t ccclaw-41490_ccclaw_43_body-445b ...`
  - 返回：`no server running on /tmp/tmux-1000/default`
  - `ccclaw-patrol.service` 直接失败退出
- `2026-03-13 12:18:15 EDT`
  - 同样错误再次出现
- `2026-03-13 12:20:17 EDT`
  - `ingest` 自己在推进 repo slot 时也遇到相同错误
  - `ccclaw-ingest.service` 同样失败退出

也就是说：

- 这不是一次性的巡查误报
- 同一个 `tmux` 错误会同时打崩 `patrol` 与 `ingest`
- 任务因此停在 `RUNNING`，但没有继续收口到 `pane_dead` / `FINALIZING` / `DEAD`

### 2. `tmux` server 级消失没有被统一映射为 `ErrSessionNotFound`

从代码上可以直接看出：

- [src/internal/tmux/manager.go](/opt/src/ccclaw/src/internal/tmux/manager.go) 中：
  - `List()` 已把 `no server running` 视为“无会话”，返回 `nil, nil`
  - 但 `Status()` 只把 `can't find session` 视为 `ErrSessionNotFound`
- [src/internal/app/runtime_cycle.go](/opt/src/ccclaw/src/internal/app/runtime_cycle.go) 中：
  - `refreshRunningRepoSlot()` 与 `patrolRepoSlots()` 都只处理 `ErrSessionNotFound`
  - 其它错误会直接返回并打断整轮执行

因此，当前根因之一已经比较明确：

- `tmux server` 不存在时，运行态逻辑没有把它归一化成“该 slot 的 session 已消失，进入 `pane_dead` 等待收口”

### 3. `tmux` 会话可能在 `remain-on-exit` 生效前就已结束

这点目前是基于代码的推断，不是直接证据：

- [src/internal/tmux/manager.go](/opt/src/ccclaw/src/internal/tmux/manager.go) 的 `Launch()` 先执行：
  - `tmux new-session -d -s ... <command>`
- 然后才执行：
  - `tmux set-option remain-on-exit on`

如果 Claude 启动命令在极早期就退出，那么可能出现：

- 会话已结束
- `remain-on-exit` 还未来得及设置
- 整个 tmux server 因无会话而退出

本轮现场中：

- `#43` 的 log/stdout/diagnostic 都为空
- `tmux ls` 也看不到 `ccclaw-*` 会话

这与上述推断是一致的，但还需要在下一轮单独验证。

## 与 #42 的关系

Issue #42 已完成的改造，本轮能确认这几项是真正生效的：

1. `run.timer` 退出主链路，批准后不再需要等下一轮 `run`
2. `Issue -> ingest cycle -> ReportStarted` 的即时反馈已经可见
3. `state.json + slot sidecar` 已能稳定保留 `RUNNING` / `slot.phase=running`，没有再出现 `#41` 那种被 `NEW` 覆盖的现象

但 `#42` 还没有被完整验证闭环的，是这几项：

1. slot 从 `running` 自动转 `pane_dead`
2. `ingest` 在下一轮读取结果并进入 `FINALIZING`
3. 最终成功/失败评论能自动回到 Issue
4. `status --json.slots` 与服务健康之间的一致性在 tmux server 消失场景下仍然成立

所以本轮不能下“通过 Issue 的异步协作流程已经完全可用”的结论。  
更准确的表述应是：

- **异步入口与发射链已可用**
- **自动收口与最终反馈链仍存在明确阻断**

## 建议下一轮优先优化

### 1. 把 `no server running` 统一降级为 session 丢失语义

优先级最高。

建议：

- 在 `tmux.Status()` 中把 `isNoServer(err)` 也映射为 `ErrSessionNotFound`
- 或在 `refreshRunningRepoSlot()` / `patrolRepoSlots()` 中显式把该错误转义为 `pane_dead`

目标：

- 不能因为 tmux server 已退出，就把 `ingest` / `patrol` 整轮打崩

### 2. 给“发射后瞬时退出”补最早期诊断落点

当前 `#43` 最大的问题不是失败，而是失败后没有证据。

建议：

- 在 tmux 发射前后立即记录：
  - 启动命令
  - tmux `new-session` 返回结果
  - session 首次探测结果
- 若首次探测已不存在：
  - 立刻补一条 `WARNING/DEAD` 事件
  - 补抓一次诊断信息

目标：

- 不再出现“只有 RUNNING 回帖，但日志、stdout、diagnostic 都为空”的黑洞

### 3. 重新审视 `remain-on-exit` 的设置时机

这是基于推断的优化建议。

建议评估两种方案：

- 方案 A：用 `new-session` 时直接确保 pane 退出后仍保留可观测现场
- 方案 B：发射后立刻做一次 `Status()` 自检；若会话不存在，直接按失败链收口，而不是等 patrol

目标：

- 把“极短命会话”从黑盒变成可诊断失败

### 4. 把 slot 健康接入 `doctor`

Issue #42 已补 `status --json.slots`，但当前服务失败时，外部还要自己拼状态。

建议：

- `doctor` 或单独 `runtime doctor` 直接输出：
  - 哪个 repo slot 卡住
  - 当前 phase / step
  - 最近推进时间
  - 服务是否正在连续失败
  - 建议人工动作

目标：

- 避免出现 “status 看见 RUNNING，systemd 实际连着失败” 的理解错位

## 本轮结论

本轮对 `#43` 的回溯可以形成 3 条明确判断：

1. `#42` 解决的“批准后即时入队并发射”已经通过真实 Issue 验证，可认为这段流程可用
2. 当前“执行后自动收口并反馈”仍未验证通过，且现场已经坐实存在 `tmux no server running` 的阻断
3. 因此，当前不能把“通过 Issue 进行异步协作的流程”判定为完全可用，只能判定为：
   - 入口可用
   - 发射可用
   - 收口仍待修复
