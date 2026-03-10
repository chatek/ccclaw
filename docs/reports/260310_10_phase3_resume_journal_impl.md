# 260310_10_phase3_resume_journal_impl

## 背景

Issue #10 的 Phase 1/2 已经完成 Claude JSON 统计、tmux 巡查与基础 stats/patrol 能力，但 Phase 3 仍缺以下闭环：

- 失败任务未优先复用 `last_session_id`
- 完整 prompt 未归档，token 记录无法回溯到提示词
- `ccclaw journal` 与每日 systemd 定时生成链路未落地
- `rtk_enabled` 已预留但没有形成对比统计输出

## 实施

### 1. executor 补齐 prompt/resume 元数据链

- 新增 `executor.RunOptions`，支持 `Prompt` 与 `ResumeSessionID`
- 执行前将完整 prompt 归档到 `~/.ccclaw/var/prompts/`
- 为每次执行写入 sidecar 元数据文件，记录：
  - `prompt_file`
  - `rtk_enabled`
  - `resume_session_id`
- tmux 启动命令补 `--resume`
- tmux 路径显式注入 `.env` secrets，避免后台会话继续依赖调用环境变量

### 2. SQLite 增强统计与 journal 查询

- `token_usage` 新增 `prompt_file`
- 新增按时间范围聚合 token 统计
- 新增 RTK 对比统计查询
- 新增 journal 日汇总、任务汇总、事件列表查询

### 3. runtime/CLI 落地 Phase 3

- `run` 对失败任务优先走 resume
- 新增 `ccclaw journal`
- `ccclaw stats` 增加 `--rtk-comparison`
- doctor/systemd 检查范围扩展到 `ccclaw-journal.timer`

### 4. 发布资产补齐

- 新增：
  - `src/ops/systemd/ccclaw-journal.service`
  - `src/ops/systemd/ccclaw-journal.timer`
  - `src/dist/ops/systemd/ccclaw-journal.service`
  - `src/dist/ops/systemd/ccclaw-journal.timer`
- `src/dist/install.sh` 与 `src/dist/upgrade.sh` 更新 journal timer 的启用/重启提示

## 结果

- Phase 3 的四个核心目标都已落地到主线：
  - `--resume` 重试逻辑
  - prompt 归档
  - `journal` 日报生成
  - rtk 对比统计
- `journal` 生成路径与 `kb/journal` 规范对齐，输出到 `年/月/yyyy.mm.dd.{user}.ccclaw_log.md`
- 发布资产已包含每日 23:50 的 journal timer，安装链路与 doctor 能识别该能力

## 验证

- `go test ./...`
- `bash src/tests/install_regression.sh`

## 风险与后续

- 当前 `rtk_enabled` 仍属于执行路径推断，不是 Claude/rtk 返回的显式埋点；后续若 wrapper 暴露确定性标记，可再从“推断”升级为“实测”
- `journal` 当前以当日任务/事件/成本汇总为主，后续可继续追加月度 `summary.md` 的自动汇编
