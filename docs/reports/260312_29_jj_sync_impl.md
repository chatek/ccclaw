# 260312_29_jj_sync_impl

## 背景

Issue #29 最终计划要求把本地仓库同步从“只写文件/只等 Claude 提交”推进到可执行的 `jj` 流程，覆盖以下链路：

- `journal` 写完知识仓库后自动提交并尝试推送
- `run` 任务完成后同步对应任务仓库
- `patrol` 作为兜底，对任务仓库和知识仓库执行全量同步
- 安装流程强制具备 `jj`，并把 Git 仓库初始化为 colocated 模式
- `journal` 日报按 `target_repo` 拆分文件名与内容

## 实施内容

### 1. 新增 `internal/vcs/jj.go`

实现统一的 `vcs.SyncRepo()`：

- 缺少 `.jj/` 时自动执行 `jj git init --colocate`
- 有 `origin` 时执行 `fetch -> rebase -> 冲突检查 -> commit -> bookmark set -> push`
- push 最多重试 3 次
- 无远端时退化为本地 `track + commit`
- 暴露 `ErrJJNotAvailable`、`ErrConflict`、`ErrPushFailed`

同时新增 `internal/vcs/jj_test.go`，覆盖：

- 指定路径提交
- push 重试
- 冲突中止
- 无远端仅本地提交

### 2. 存储层支持按 `target_repo` 过滤

在 `storage.Store` 中新增：

- `JournalDaySummaryByTarget()`
- `JournalTaskSummariesByTarget()`
- `ListTaskEventsBetweenByTarget()`
- `RTKComparisonBetweenByTarget()`

这样 `journal` 不再先查全量再在运行时二次筛选。

### 3. `journal` 改为按 target 生成日报

`Runtime.Journal()` 现在会：

- 遍历启用 target，为每个 target 生成独立日报
- 文件名改为 `YYYY.MM.DD.<user>.<owner-repo>.ccclaw_log.md`
- 标题改为 `# ccclaw journal YYYY-MM-DD [owner/repo]`
- 月/年/根汇总仍保持全量聚合
- 日报及三个 summary 文件写完后，调用 `vcs.SyncRepo()` 仅同步本次 journal 写入文件

兼容上补了一步保守迁移：

- 若当前仅有一个 target，且同日存在旧文件名日报，则在首次写入新规则文件前自动迁移

### 4. `run` / `patrol` 接入同步

- `finishTaskExecution()` 在任务成功完成后同步目标任务仓库
- 同步失败不打断任务完成状态，而是追加 `WARNING` 事件
- `Patrol()` 在巡查收口后，对全部启用 target 执行 `patrol: sync`
- `Patrol()` 同时对知识仓库执行 `patrol: sync home`

### 5. 安装与交付更新

`src/dist/install.sh` 已补：

- 自动检测 `jj`
- 缺失时按 Linux 预编译包安装到 `/usr/local/bin/jj`
- 本体仓库与任务仓库绑定后自动执行 `jj git init --colocate`
- 安装时同步 `src/dist/jj.md` 到 `~/.ccclaw/jj.md`

`src/dist/CLAUDE.md` 也补充了 `jj.md` 必须交付的约束。

## 验证

已执行：

- `gofmt -w src/internal/vcs/jj.go src/internal/vcs/jj_test.go src/internal/adapters/storage/sqlite.go src/internal/adapters/storage/sqlite_test.go src/internal/app/runtime.go src/internal/app/runtime_test.go`
- `go test ./internal/vcs ./internal/adapters/storage ./internal/app`
- `go test ./...`
- `bash -n src/dist/install.sh`

结果：

- 全部通过

## 当前边界

本轮只实现了“单 target 下同日旧日报文件”的保守迁移。

对历史月份中已经存在的旧命名日报，尚未做批量回填脚本；目前它们仍会继续被月汇总索引到，不影响读取，但文件树不会自动一次性重写。
