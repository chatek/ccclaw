# 260312_32_phase2_sevolver_skill_lifecycle

## 背景

Issue #32 已锁定顺序为 `Phase 0 -> 1 -> 2 -> 3`。

其中 `Phase 2` 的目标是补齐 `sevolver` 自我成长服务，把前两阶段已经铺好的基础能力串起来：

- `Phase 0` 已保证 Skill frontmatter 中的 `last_used/use_count/status/gap_signals` 在升级时不会被模板回刷
- `Phase 1` 已把运行态主存储切到 JSONL/`state.json`

本轮聚焦 `Phase 2` 的最小可部署闭环：

1. 扫描 journal 中显式出现的 `kb/skills/...` 路径
2. 维护 Skill 生命周期字段
3. 将超时未触发的 Skill 归档到 `kb/skills/deprecated/`
4. 记录缺口信号到 `kb/assay/gap-signals.md`
5. 生成 `sevolver` 日报并接入 CLI/systemd/install 链路

## 本轮实现

### 1. 新增 `internal/sevolver` 核心包

新增文件：

- `src/internal/sevolver/scanner.go`
- `src/internal/sevolver/skill_updater.go`
- `src/internal/sevolver/gap_recorder.go`
- `src/internal/sevolver/reporter.go`
- `src/internal/sevolver/sevolver.go`

落地能力：

- `ScanJournal()`：
  - 扫描 `journal/` 下最近 7 天 Markdown
  - 仅识别明确写出的 `kb/skills/.../*.md` 路径
  - 命中后规范化为相对 `kb/` 根的 `skills/...` 路径
- `ScanJournalForGaps()`：
  - 默认使用内建失败/缺口关键词
  - 若存在 `kb/assay/signal-rules.md`，则按其中 `关键词:` 行动态装载
- `UpdateSkillMeta()`：
  - 更新 `last_used`
  - 递增 `use_count`
  - 激活 `status: active`
  - 缺失时补 `gap_signals: []`
- `processSkillLifecycle()`：
  - 超过 14 个自然日未命中 => `dormant`
  - 超过 28 个自然日未命中 => `deprecated` 并物理迁移
- `AppendGapSignals()`：
  - 仅更新 `gap-signals.md` 的 managed 区块
  - 自动去重
  - 仅保留最近 30 天
- `WriteDailyReport()`：
  - 输出 `kb/journal/sevolver/YYYY/MM/YYYY.MM.DD.log.md`

### 2. Skill 归档改为“保留相对目录结构”

`Issue` 规格只说“移入 `deprecated/`”，但真实仓库里的 Skill 入口统一叫 `CLAUDE.md`。

如果简单按文件名平铺迁移，会立刻发生冲突：

- `skills/L1/foo/CLAUDE.md`
- `skills/L2/bar/CLAUDE.md`

都会撞成 `deprecated/CLAUDE.md`。

因此本轮实现改为保留 `skills/` 下的相对路径，例如：

- `skills/L2/deploy-flow/CLAUDE.md`
- `=> skills/deprecated/L2/deploy-flow/CLAUDE.md`

这样既满足“废弃后默认不加载”，也保留原始层级语义与历史上下文。

### 3. 生命周期按“自然日”而不是“小时差”计算

直接用 `time.Duration.Hours()/24` 计算 14 天，会在夏令时切换周出现 13.x 天误差。

本轮改为：

- 仅取 `YYYY-MM-DD`
- 转成 UTC 零点日期
- 再按整日差值计算

这样 `2026-02-26 -> 2026-03-12` 不会因为 DST 少 1 小时而误判成 13 天。

### 4. memory 索引排除 `kb/skills/deprecated/`

修改：

- `src/internal/memory/index.go`
- `src/internal/memory/index_test.go`

现在 `memory.Build()` 在遍历时遇到 `skills/deprecated` 会直接 `SkipDir`。

结果：

- 被归档的 Skill 仍保留在知识仓库内
- 但默认不会再进入运行态检索与 prompt 注入

### 5. 接入 CLI、systemd、日志与安装链路

新增：

- `src/cmd/ccclaw/sevolver.go`
- `src/dist/ops/systemd/ccclaw-sevolver.service`
- `src/dist/ops/systemd/ccclaw-sevolver.timer`

修改：

- `src/cmd/ccclaw/main.go`
- `src/internal/scheduler/systemd.go`
- `src/internal/scheduler/systemd_units.go`
- `src/internal/scheduler/status.go`
- `src/internal/scheduler/logs.go`
- `src/internal/app/runtime.go`
- `src/dist/install.sh`
- `src/tests/install_regression.sh`

结果：

- 新增命令：`ccclaw sevolver`
- `systemd --user` 托管 timer 变为：
  - `ingest`
  - `run`
  - `patrol`
  - `journal`
  - `archive`
  - `sevolver`
- `scheduler logs all` 现在也覆盖 `archive` 与 `sevolver`
- 安装器自动启用/重启的 timer 列表同步加入 `ccclaw-sevolver.timer`

### 6. 新增 KB 模板资产

新增：

- `src/dist/kb/assay/signal-rules.md`
- `src/dist/kb/assay/gap-signals.md`
- `src/dist/kb/skills/deprecated/.gitkeep`

目的：

- 让 `sevolver` 的规则文件、缺口台账和废弃 Skill 目录在 release 中具备稳定落点
- 升级时继续沿用现有 managed/user 分区保留策略

## 验证

执行：

```bash
cd src && go test ./...
cd ..
bash src/tests/install_regression.sh
```

结果：

- `go test ./...` 全量通过
- `install_regression.sh` 全量通过

专项覆盖：

- `internal/sevolver`：
  - journal 命中扫描
  - gap 关键词装载
  - frontmatter 更新
  - dormant / deprecated 生命周期
  - gap-signals 去重与清理
- `internal/memory`：
  - `deprecated/` 排除
- `internal/scheduler`：
  - `ccclaw-sevolver.timer` unit 生成
- 安装器回归：
  - systemd enable/restart 列表含 `ccclaw-sevolver.timer`

## 当前结论

Issue #32 的 `Phase 2` 已完成到可部署状态：

- `sevolver` 已具备日常扫描、Skill 生命周期维护、缺口记录与日报输出能力
- 废弃 Skill 会物理迁入 `kb/skills/deprecated/`，并自动退出默认 memory 索引
- `systemd/install/logs` 入口已全部接通

## 仍未进入本轮范围

以下仍属于 `Phase 3` 或后续优化，不在本轮混入：

- 按阈值触发 Claude Code 深度分析
- 将 `gap_signals` 精确回填到具体 Skill frontmatter
- 基于 `task_events` 的更强语义缺口判断
- 自动把 `sevolver` 产物同步提交到知识仓库

## 建议下一步

若继续按已定顺序推进，下一步应进入 `Phase 3`：

- 为重复性缺口信号补阈值聚合
- 满足阈值后自动创建 GitHub Issue，触发 Claude Code 深度分析闭环
