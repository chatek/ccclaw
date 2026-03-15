# 260314_NA_residual_followup_issues_57_58

## 背景

在 `26.03.15.0746` 发布与本机升级验收完成后，上一轮报告已确认两个残留点：

1. `ccclaw-run.timer` / `ccclaw-run.service` 旧 systemd 单元仍残留
2. `home_repo` 在升级过程中自动产生 `seed ccclaw home repo` 提交

本轮目标是把这两个残留点拆成独立 Issue，并把分析过程、建议方案与待拍板问题分别沉淀，避免继续混在发布验收记录里。

## 现场补充分析

### 1. 旧 `ccclaw-run.*` systemd 单元残留

核对结果：

- `docs/reports/260313_42_ingest_slot_finalizing_refactor.md` 已明确记录：`run.timer` 已从代码、发布树、cron 规则和安装链路设计中删除
- `src/internal/scheduler/systemd.go` 当前 `ManagedSystemdTimers()` 只包含：
  - `ccclaw-ingest.timer`
  - `ccclaw-patrol.timer`
  - `ccclaw-journal.timer`
  - `ccclaw-archive.timer`
  - `ccclaw-sevolver.timer`
- `src/tests/install_regression.sh` 已断言新安装目录下不存在 `ccclaw-run.service` / `ccclaw-run.timer`

但同时发现：

- 正常升级路径只会安装/启用当前受管 unit
- 不会主动 `disable --now` 历史遗留的 `ccclaw-run.timer`
- `removeManagedSystemdUnits()` 只在 `DisableSystemd()` 中调用，不在正常升级中调用

因此根因已足够明确：

- 不是新版本又把 `run.timer` 生成出来
- 而是升级链路缺少“旧 unit 清理”步骤

### 2. `home_repo` 自动 seed/commit 边界不清

核对结果：

- `src/dist/install.sh` 中：
  - `seed_home_repo_tree` 会同步 `kb/` 初始树并刷新受管内容
  - `commit_home_repo_seed` 会执行 `git add kb README.md CLAUDE.md .gitignore`
  - 只要 staged 非空，就直接 `git commit -m 'seed ccclaw home repo'`
- `init_or_attach_home_repo` 无论 `init|remote|local`，最后都会执行：
  - `seed_home_repo_tree`
  - `commit_home_repo_seed`

本机现场进一步验证：

- `/opt/data/9527` 最新提交确为 `seed ccclaw home repo`
- 提交内容不仅包括 `kb/**/CLAUDE.md`
- 还包含 `kb/journal/**`、`kb/journal/sevolver/**`、`gap-signals.md` 等运行期内容

因此第二个残留点的性质与第一个不同：

- 第一个更接近实现遗漏
- 第二个更接近“策略尚未拍板，当前行为已经超出文档共识边界”

## 已创建的 Issue

### 1. Issue #57

- URL：<https://github.com/41490/ccclaw/issues/57>
- 标题：`bug: 升级后未清理旧 ccclaw-run.timer/run.service`

Issue 内容包含：

- 现场命令与观测结果
- 代码与测试交叉核对
- 根因分析：升级链路缺少 legacy unit 收缩
- 推荐方案：升级时自动 `disable --now` 并删除旧 unit，再执行当前 5 类 timer 的 `daemon-reload + enable/restart`
- 待拍板问题：
  - 是否接受升级器自动清理旧 `ccclaw-run.*`
  - user bus 不可用时是否降级为“删文件 + 打印精确补救命令”
  - 是否在 `doctor` 中提升为 `WARN`

### 2. Issue #58

- URL：<https://github.com/41490/ccclaw/issues/58>
- 标题：`decision: 升级时 home_repo 自动 seed/commit 边界需要拍板`

Issue 内容包含：

- 现场提交事实
- `install.sh` 当前自动 `seed + git add + git commit` 路径说明
- 风险分析：用户未确认即改写 `home_repo` 历史、运行态内容与模板刷新混在同次提交里
- 备选方案：
  - 方案 A：只刷新受管区块，不自动提交
  - 方案 B：允许自动提交，但严格限制在受管模板文件
  - 方案 C：承认现状并补齐文档
- 待拍板问题：
  - 是否允许升级时自动提交
  - 自动提交范围是否排除 journal/sevolver/gap 运行态内容
  - `init|remote|local` 三种模式是否统一策略
  - 若不自动提交，是否接受只打印建议命令由用户自行确认

## 结论

本轮已把两个残留点成功拆分为：

1. 可直接修复的升级缺口：Issue #57
2. 需要先拍板边界的产品/治理决策：Issue #58

后续建议按这个顺序推进：

1. 先在 #58 上拍板 `home_repo` 升级边界
2. 再实现 #57 的旧 unit 自动清理
3. 两者落地后，再做一轮“旧版本升级到新版本”的完整回归验证
