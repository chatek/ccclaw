# 260311_23_scheduler_doctor_logs_followup

## 背景

承接 Issue #23 最后一条回复里提出的下一轮建议：

1. 增补 `scheduler doctor`
2. 补日志级别与输出归档策略
3. 让 `scheduler timers` 更明确展示“配置原文 / 生效表达式 / 主机时区 / 配置时区”

本轮目标是不再只停留在 `status/timers/logs` 的基础能力，而是把调度运维闭环继续补到“可诊断、可筛选、可归档”。

## 实施

### 1. `ccclaw scheduler doctor`

- 新增 `src/internal/scheduler/doctor.go`
- 新增命令入口：`ccclaw scheduler doctor`
- 检查项收敛为调度相关能力，不再混入总 `doctor`：
  - 调度配置
  - 当前调度状态
  - `journalctl`
  - 日志归档目录
  - `systemctl`
  - `user bus`
  - 托管 timer 健康
  - `crontab`（仅在 cron 相关场景）
- 输出格式与总 `doctor` 对齐，统一使用 `[ OK ] / [FAIL]`

### 2. 日志级别与归档

- 配置新增 `[scheduler.logs]`
  - `level`
    - 作为 `ccclaw scheduler logs` 的默认 journal 优先级过滤
    - 允许值：`emerg|alert|crit|err|warning|notice|info|debug`
  - `archive_dir`
    - 作为 `ccclaw scheduler logs --archive` 的默认归档目录
- `scheduler logs` 新增：
  - `--level`
  - `--archive`
- 归档策略：
  - 默认落到 `scheduler.logs.archive_dir`
  - 文件名按时间戳 + scope 生成
  - 文件头写入归档时间、scope、level、since 元信息
  - 归档同时保留终端输出，不强制二选一

### 3. timer 视图细化

- `ccclaw scheduler timers` 现改为明确输出：
  - `CAL_RAW`
    - 配置原文
  - `CAL_CFG`
    - 追加 `calendar_timezone` 后的生效表达式
  - `NEXT[host_tz] / LAST[host_tz]`
  - `NEXT[cfg_tz] / LAST[cfg_tz]`
- 命令顶部增加中文说明，避免把“原始配置表达式”和“生效表达式”混看

### 4. 安装模板与运维手册同步

- `src/ops/config/config.example.toml`
  - 新增 `[scheduler.logs]` 注释与默认值
- `src/ops/examples/app-readme.md`
  - 新增 `scheduler doctor`
  - 新增 `scheduler logs --level ... --archive` 示例
- `src/dist/install.sh`
  - 安装时生成的 `config.toml` 同步带出 `[scheduler.logs]`
- `make dist-sync`
  - 已把 `ops/` 新模板同步进 `src/dist/ops/`

## 验证

执行：

```bash
cd src
go test ./cmd/ccclaw ./internal/config ./internal/scheduler
go test ./...
make dist-sync
bash tests/install_regression.sh
```

结果通过。

补充回归覆盖：

- `scheduler doctor` 命令输出
- `scheduler logs --level --archive`
- 归档文件头与落盘
- `scheduler timers` 新列输出
- `config.Load()` 对 `scheduler.logs` 默认值补全
- 安装回归校验 `README.md` 与 `config.toml` 新段落

## 结论

Issue #23 建议中的第二阶段最小闭环已补齐：

- 调度后端已有独立 `doctor`
- 日志现在可以按优先级查看，并能一键归档
- timer 展示不再混淆“配置原文”和“生效表达式”
- 安装/升级生成的运维文档与模板配置已同步更新

## 未覆盖项

本轮仍未实现：

1. 应用内部结构化日志与真正的运行时 log level 控制
2. journald 之外的长期自动轮转/压缩归档
3. timer 视图中更细的跨时区格式化样式
