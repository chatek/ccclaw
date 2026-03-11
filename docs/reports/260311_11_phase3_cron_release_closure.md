# 260311_11_phase3_cron_release_closure

## 背景

Issue #11 在最后一轮回复中已明确拍板：

- `auto` 在 `systemd --user` 不可部署时，应继续尝试降级到 `cron`
- `cron` 采用完整任务集：`ingest/run/patrol/journal`
- `cron` 需要提供受控写入、更新与清理入口，且不能覆盖用户其它 `crontab` 规则
- `dist/README` 需要补齐控制仓库 / 本体仓库 / 任务仓库术语
- `make release` 需要补 release notes 术语模板

本轮目标是完成 Issue #11 的最后阶段收口。

## 实现

### 1. 受控 cron 落地

- 新增 `src/internal/scheduler/cron.go`
- 提供统一的受控 `cron` 规则生成、块清理、幂等写入与删除能力
- 受控块使用：
  - `# >>> ccclaw managed cron >>>`
  - `# <<< ccclaw managed cron <<<`
- 周期任务已扩为 4 条：
  - `ingest`：每 5 分钟
  - `run`：每 10 分钟
  - `patrol`：每 2 分钟
  - `journal`：每日 23:50

### 2. CLI 与安装器入口

- `ccclaw` 新增：
  - `scheduler enable-cron`
  - `scheduler disable-cron`
  - `config set-scheduler`
- `src/dist/install.sh` 新增：
  - `--remove-cron`
- 安装器行为调整：
  - `auto` 优先探测 `systemd --user`
  - 若 `systemd --user` 不可部署，再探测 `crontab`
  - `crontab` 可用时自动降级为受控 `cron`
  - `cron` 模式下自动写入/更新受控块
  - `systemd|none` 模式下自动尝试清理旧的受控块，降低双重调度风险

### 3. doctor 诊断同步

- `src/internal/app/runtime.go` 的 `detectCronEntries()` 改为识别完整四条受控规则
- `cron` 修复建议从“手工抄样板”改为优先指向：
  - `ccclaw scheduler enable-cron`
  - `ccclaw scheduler disable-cron`

### 4. 文档与发布收口

- `src/dist/README.md` 补齐三类仓库职责与调度边界
- `README.md` 同步更新：
  - `auto -> cron` 降级语义
  - 四个 systemd timer
  - `scheduler enable-cron|disable-cron` 常用命令
- 新增 `src/ops/examples/release-notes-template.md`
- `src/Makefile` 新增 `release-notes` 目标，`make release` 改为使用模板生成 release notes

## 验证

已执行：

```bash
cd src
make test
make test-install
make release-notes
```

结果：

- `go test ./...` 通过
- `install_regression.sh` 通过，新增覆盖：
  - `auto` 自动降级到受控 `cron`
  - `cron` 写入保留用户原有 `crontab` 规则
  - `cron` 幂等更新不重复追加受控块
  - `--remove-cron` 只清理受控块
- `release-notes` 模板成功渲染到 `.release/RELEASE_NOTES.md`

## 结论

Issue #11 的最后阶段要求已完成：

- Phase 3：`cron` 已从“只打印样板”推进到“可受控写入 / 更新 / 清理”
- Phase 4：`dist/README` 与 release notes 模板已完成术语对齐
