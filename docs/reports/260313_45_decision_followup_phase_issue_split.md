# 260313_45_decision_followup_phase_issue_split

## 背景

用户提供 Issue #45 决策评论链接：

- <https://github.com/41490/ccclaw/issues/45#issuecomment-4058421715>

决策内容为：

1. 前 5 项均同意；
2. 第 6 项改为“前述阶段完成后，立即切换”。

## 本轮执行

### 1. 决策核查

通过 `gh issue view 45 --comments --json ...` 读取并确认最新拍板评论，结论与用户提供链接一致。

### 2. 拆分可执行子 Issue

已按 Phase 0~3 拆分四个执行入口：

1. #46 `phase0: stream-json 事件契约冻结与解析基线`
   - <https://github.com/41490/ccclaw/issues/46>
2. #47 `phase1: tmux 下切 stream-json 影子对账`
   - <https://github.com/41490/ccclaw/issues/47>
3. #48 `phase2: daemon 执行内核与 repo 级灰度切流`
   - <https://github.com/41490/ccclaw/issues/48>
4. #49 `phase3: 默认切换 daemon 与 tmux 职责收缩`
   - <https://github.com/41490/ccclaw/issues/49>

以上 Issue 均带 `ccclaw` 标签，便于继续进入自动执行链路。

### 3. 回帖 #45 绑定追踪链路

已在 #45 回帖汇总子 Issue 并确认执行顺序：

- 回帖链接：<https://github.com/41490/ccclaw/issues/45#issuecomment-4058430217>
- 执行顺序：`46 -> 47 -> 48 -> 49`

同时在回帖中已显式更新切换闸门：

- 不再把“单仓 7 天 + 全局 14 天观察窗口”作为硬门槛；
- 改为“前述阶段验收完成后立即切换默认 daemon”。

## 产出清单

1. 新建执行子 Issue：#46/#47/#48/#49。
2. 更新 #45 跟踪回帖并同步拍板口径。
3. 新增本工程报告。
