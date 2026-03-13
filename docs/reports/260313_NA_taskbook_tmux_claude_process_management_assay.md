# 260313_NA_taskbook_tmux_claude_process_management_assay

## 背景

2026-03-13，按 `sysNOTA` 要求开展纯调研，不改任何源码，目标是：

- 深入学习 `taskbook-sh/taskbook`
- 与 `tmux` 对比
- 评估哪种工具更适合作为 Claude 进程的稳定、友好、一致的管理载体
- 将最终方案写入 `docs/assay`

同时遵守额外要求：

- 先核对仓库内既有 Issue 决策
- 所有外部事实先经 DuckDuckGo 检索入口核验

## 过程

本轮先核对了：

- `AGENTS.md`
- `CLAUDE.md`
- [docs/reports/260313_NA_issue41_runtime_flow_investigation.md](/opt/src/ccclaw/docs/reports/260313_NA_issue41_runtime_flow_investigation.md)
- [Issue #42](https://github.com/41490/ccclaw/issues/42) 的拍板讨论

随后经 DuckDuckGo 检索定位并回查官方资料：

- `taskbook-sh/taskbook` README、CLI Reference、Server Setup、Releases
- `tmux` README、man page、Releases

最后形成正式调研稿：

- [docs/assay/260313-tmux-vs-taskbook-for-claude-process-management.md](/opt/src/ccclaw/docs/assay/260313-tmux-vs-taskbook-for-claude-process-management.md)

## 结论摘要

本轮结论明确：

- `taskbook-sh/taskbook` 不是终端复用器，而是任务/看板/笔记管理工具
- `tmux` 才是与 Claude 长时执行场景直接匹配的 PTY/session 托管层
- 对 `ccclaw` 而言，正确方向不是“tmux 不稳所以换 taskbook”，而是继续采用 `tmux`，同时把状态一致性收回 `systemd + ingest/patrol + state.json/sidecar`

## 交付物

- 调研主稿：
  - [docs/assay/260313-tmux-vs-taskbook-for-claude-process-management.md](/opt/src/ccclaw/docs/assay/260313-tmux-vs-taskbook-for-claude-process-management.md)
- 工程报告：
  - 本文

## 说明

- 本轮未修改任何源码
- 本轮未运行测试；原因是只产出文档，不涉及实现变更
