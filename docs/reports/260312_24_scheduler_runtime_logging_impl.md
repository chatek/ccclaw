# 260312_24_scheduler_runtime_logging_impl

## 背景

Issue #24 要求把 `scheduler.logs.level` 从“`scheduler logs` 的查看过滤”提升为“运行态产品能力”，至少覆盖：

- `ingest`
- `run`
- `patrol`
- `journal`

此前仓库内存在两个根因问题：

1. 运行态没有统一日志入口，多个路径直接散写 stdout/stderr
2. `scheduler.logs.level` 只影响 `journalctl -p` 默认参数，不能控制程序内部输出密度

## 本轮实现

### 1. 新增统一运行态日志组件

- 新增 `src/internal/logging/logger.go`
- 统一收口四级语义：`debug|info|warning|error`
- 兼容历史别名：
  - `notice -> info`
  - `err|crit|alert|emerg -> error`
- 统一输出到命令 `stderr`，避免污染 CLI 面向用户的核心 `stdout`

### 2. 让配置真正接通运行态

- `app.NewRuntimeWithOptions(...)` 在启动时读取：
  - `scheduler.logs.level`
  - 命令行 `--log-level` 临时覆盖
- `ingest/run/patrol/journal` 统一通过 logger 输出运行态日志
- `run` 去掉内部直接 `fmt.Println`，改为显式 writer 传入，消除一处散写 stdout 的根因

### 3. 保持 journald 查看链路兼容

- `scheduler logs --level error` 现在会自动映射为 `journalctl -p err`
- 配置仍兼容旧值 `notice/err/crit/alert/emerg`
- `scheduler.logs.level` 既作为运行态阈值，也作为 `scheduler logs` 默认查看过滤

### 4. 诊断与文档补齐

- `scheduler doctor` 新增“运行态日志级别”检查项
- README、dist README、`config.example.toml` 补充：
  - 运行态阈值与查看过滤的关系
  - `--log-level` 临时覆盖能力
  - `error -> err` 的 journald 兼容说明

## 测试

执行：

```bash
cd src
go test ./...
```

新增覆盖：

- `internal/app/runtime_logging_test.go`
  - 验证 `ingest/run/patrol/journal` 在 `info` 与 `warning` 级别下的日志输出差异
- `cmd/ccclaw/main_test.go`
  - 验证 `--log-level` 临时覆盖
  - 验证 `scheduler logs --level error` 到 `journalctl -p err` 的映射

## 结果

本轮后：

- `scheduler.logs.level` 不再只是查看层参数
- 运行态四个关键入口已经共享同一套日志级别语义
- CLI 核心结果输出与运行态调试日志已分流为 `stdout/stderr`
- 手工排障可用 `ccclaw --log-level debug <command>` 临时放大日志，不需要改配置文件
