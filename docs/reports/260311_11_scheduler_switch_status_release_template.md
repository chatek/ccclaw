# 260311_11_scheduler_switch_status_release_template

## 背景

在 Issue #11 最后一轮收口完成后，仍有 3 个后续优化点需要继续推进：

1. 增加独立的 `scheduler status`
2. 提供统一的 `scheduler use systemd|cron|none` 切换入口
3. 扩展 release notes 模板，固定加入升级影响 / 迁移动作 / 回滚命令区块

本轮目标是把这三点做成正式能力，而不是停留在建议层。

## 实现

### 1. 新增 `ccclaw scheduler status`

- `src/cmd/ccclaw/main.go` 新增 `scheduler status`
- 该命令复用运行时调度诊断输出，单独打印：
  - `request`
  - `effective`
  - `reason`
  - `repair`
  - 必要时的 `systemd=` / `cron=` 上下文
- 安装后无需再依赖 `doctor` 全量输出，即可独立确认调度后端状态

### 2. 新增统一切换入口 `ccclaw scheduler use`

- `src/internal/scheduler/use.go` 新增统一切换逻辑
- `src/internal/scheduler/systemd.go` 新增 user systemd 管理能力
- 当前支持：
  - `ccclaw scheduler use systemd`
  - `ccclaw scheduler use cron`
  - `ccclaw scheduler use none`

切换语义：

- `use systemd`
  - 先清理受控 cron
  - 再把 `ops/systemd/*.service|*.timer` 安装到 `systemd_user_dir`
  - 执行 `systemctl --user daemon-reload`
  - 执行 `systemctl --user enable --now ...`
  - 最后写回 `config.toml`

- `use cron`
  - 先停用并清理 user systemd 定时器与 unit 文件
  - 再写入 / 更新受控 crontab 块
  - 最后写回 `config.toml`

- `use none`
  - 同时清理 user systemd 与受控 cron
  - 最后写回 `config.toml`

额外处理：

- 若当前会话缺少 `crontab`，`disable-cron` 不再报错，而是返回“无需清理”
- 若当前会话未直连 user bus，`DisableSystemd()` 会优先清理本地 unit 文件并提示登录会话中复核，避免从 `systemd` 切到 `cron/none` 时被软性卡死

### 3. release notes 模板扩展

- `src/ops/examples/release-notes-template.md`
- `src/dist/ops/examples/release-notes-template.md`

新增固定区块：

- `升级影响`
- `迁移动作`
- `回滚命令`

这样 `make release-notes` 生成的说明不再只有术语对齐和资产信息，还能直接作为发布页执行指引。

### 4. 文档同步

- `README.md` 新增：
  - `scheduler status`
  - `scheduler use cron|systemd|none`
- `src/dist/README.md` 同步新增统一切换说明

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
- 新增命令级测试已覆盖：
  - `scheduler status`
  - `scheduler use cron`
  - `scheduler use systemd`
- `install_regression.sh` 全量通过
- `.release/RELEASE_NOTES.md` 已按新模板成功渲染

## 结论

本轮完成后，调度后端的日常维护入口已从“分散在 install.sh、doctor、手工命令”推进到“可查询、可统一切换、可发布说明落地”的状态，后续若继续补 `scheduler use auto` 或更强的 systemd 自检，也有了稳定落点。
