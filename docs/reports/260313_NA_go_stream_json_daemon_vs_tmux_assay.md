# 260313_NA_go_stream_json_daemon_vs_tmux_assay

## 背景

用户要求基于外部参考文档：

- `/opt/src/DAMA/docs/researchs/DevOps/VibeCoding/CCClaw/260314-opus-Claude_Code_Session_Daemon_Guide_zh.md`

对比 `ccclaw` 当前 tmux 包装方案与 Go stream-json daemon 方案，给出更简洁、合理、稳定、有效的推荐，并且：

- 先建 GitHub Issue
- 不修改运行代码
- 调研结果输出到 `docs/assay`
- 外部事实按 DuckDuckGo 检索链路核验

## 本轮动作

1. 检查当前 open issues 与既有拍板上下文（含 #42/#44）。
2. 新建讨论 Issue：
   - [#45](https://github.com/41490/ccclaw/issues/45)
   - 标题：`assay: Go stream-json daemon 与当前 tmux 包装方案对比与选型建议`
3. 梳理当前仓库实现：
   - 执行器、tmux 管理器、ingest/patrol 周期、slot/finalizing 收口、context restart 链路。
4. 外部事实核验（DuckDuckGo 检索入口 + 一手文档）。
5. 产出调研文档：
   - `docs/assay/260313-go-stream-json-daemon-vs-tmux.md`

## 结论摘要

- 推荐方向：Go stream-json daemon 作为主执行内核，tmux 降级为可选排障工具。
- 理由：减少轮询与文件中转链路，采用结构化事件流原位推进状态机，整体复杂度更低。
- 风险：需要补人工接管替代路径与双轨回退开关。
- 落地建议：影子观测 -> 单仓灰度 -> 双轨可回滚 -> 默认切换。

## 约束遵循

- 未修改任何运行代码
- 已按要求创建 Issue
- 已输出 `docs/assay` 调研文档
- 已标注调研模型版本与参考链接

