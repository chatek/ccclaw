# 260312_25_scheduler_logs_archive_retention_impl

## 背景

Issue #25 要求把现有 `ccclaw scheduler logs --archive` 从“只会落盘”推进到“可长期维护”的归档策略，避免 `scheduler.logs.archive_dir` 无限增长。

本轮实现遵循以下边界：

- 先解决根因：把归档文件纳入受管命名与策略执行，而不是依赖外部手工清理
- 不误删人工文件：清理与压缩只作用于 ccclaw 受管归档
- 保持安装树、配置样板、README 与 doctor 一致，不做源码侧孤岛能力

## 本轮拍板

### 默认策略

- `retention_days = 30`
- `max_files = 200`
- `compress = true`

选择理由：

- 30 天足够覆盖常见近一月运维回溯
- 200 个文件为长期使用提供数量上界，避免高频归档把目录拖大
- 默认压缩历史归档，优先压缩空间而不影响最新一次明文查看

### 受管边界

- 新归档统一命名为 `ccclaw_scheduler_YYYYMMDD_HHMMSS_scope.log`
- 旧版 `YYMMDD_HHMMSS_scope.log` 只要头部仍带 `# ccclaw scheduler logs archive`，也纳入受管兼容范围
- 目录中的人工文件，即使文件名形似旧格式，只要没有 ccclaw 归档头，也不会被压缩或删除

## 实现内容

### 配置模型

在 `[scheduler.logs]` 下新增：

- `retention_days`
- `max_files`
- `compress`

并补齐：

- `Load()` 默认值
- `NormalizePaths()` / `validateSchedulerConfig()` / `Config.Validate()` 校验
- `renderAnnotatedConfig()` 与 `UpdateSchedulerSection()` 输出
- 安装模板与配置样板注释

### 归档治理

在 `src/internal/scheduler/logs.go` 中补齐归档后处理链路：

1. 写入当前归档文件
2. 枚举受管归档
3. 压缩历史 `.log` 为 `.log.gz`，保留当前最新文件为明文
4. 删除超过 `retention_days` 的受管旧文件
5. 若受管文件数仍超过 `max_files`，按时间从旧到新裁剪

失败时会返回可操作提示，明确提示检查目录权限，或临时关闭 `scheduler.logs.compress`。

### 诊断与文档

- `scheduler doctor` 新增“日志归档策略”检查项
- 输出当前 `retention_days/max_files/compress` 与目录内受管文件数量、压缩数量
- README、程序目录运维手册、安装配置样板同步补充保留策略说明

## 验证

已执行：

```bash
cd src
go test ./...
make test-install
```

覆盖点包括：

- 配置默认值与注释渲染
- `scheduler doctor` 新增策略检查
- 归档压缩、过期删除、超额裁剪
- 旧受管文件兼容
- 人工文件不受影响
- 安装树样板与升级回归

## 结果

Issue #25 的 Phase 1~3 已形成一条闭环：

- 有默认可解释的保留策略
- 有只作用于受管文件的压缩/裁剪实现
- 有 doctor 与 README 的可观测说明

后续若要继续深化，可再拆：

- 压缩级别与策略参数细化
- `scheduler logs` 归档策略执行结果的结构化输出
- 面向脚本消费的归档统计视图
