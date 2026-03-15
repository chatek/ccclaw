# 260315_71_claude_hook_manual_session_ignore

## 背景

Issue [#71](https://github.com/41490/ccclaw/issues/71) 追踪的现象是：在部署主机上人工启动 Claude Code 时，经常出现：

```text
Stop hook error: Failed with non-blocking status code: 缺少 CCCLAW_HOOK_STATE_DIR 或 CCCLAW_APP_DIR
```

`2026-03-15` 的 Issue 决策评论已明确三点：

1. 无 `CCCLAW_TASK_ID` 时直接退出
2. 同意受管 hook 命令显式带 `--config`
3. 同意静态安装信息回归 `config.toml`，动态任务态字段继续保留运行时注入

同时补充边界：

- 人工在主机终端中启动的 Claude Code 不属于 `ccclaw` 守护业务
- `ccclaw` 应识别这类人工 Claude session，并完全忽略对应 hook

## 根因

本轮复核确认，问题是入口分层错误，不是 hook 机制本身异常：

1. `src/internal/executor/executor.go`
   - 只有 `ccclaw` 受管任务启动时，才会注入 `CCCLAW_TASK_ID`、`CCCLAW_APP_DIR`、`CCCLAW_HOOK_STATE_DIR`
2. `src/cmd/ccclaw/main.go`
   - `claude-hook` 入口此前会把人工 Claude session 也按受管任务处理
3. `src/internal/claude/hooks.go`
   - managed hook 写入 `.claude/settings.local.json` 时未显式带 `--config`
   - 运行目录变化时只能依赖默认配置探测

因此真正要区分的是两类上下文：

```text
人工 Claude session
  -> 无 CCCLAW_TASK_ID
  -> 不属于受管任务
  -> hook 直接忽略

ccclaw 受管任务
  -> 有 CCCLAW_TASK_ID
  -> 继续写 hook state
  -> 状态目录按 env 覆盖 + config 兜底解析
```

## 变更

### 1. hook 入口忽略人工 Claude session

沿用并确认 `src/cmd/ccclaw/main.go` 中的入口收口：

- 若 `CCCLAW_TASK_ID` 为空，`claude-hook` 直接返回 `nil`
- 不再把人工 Claude session 当成 `ccclaw` 任务
- `CCCLAW_TASK_ID` 已设置时，才继续解析状态目录并调用 `claude.HandleHook`

这使得人工启动的 Claude Code 彻底脱离 `ccclaw` 监察范围。

### 2. managed hook 显式写入 `--config`

在 `src/internal/claude/hooks.go` 中：

- `ManagedHookSettings` 改为同时接收 `ccclawBin` 与 `configPath`
- 生成的 managed hook 命令改为：
  - `ccclaw --config <app>/ops/config/config.toml claude-hook <event>`
- `EnsureManagedHookSettings` 与 `Runtime.ensureClaudeManagedHooks` 同步传入固定配置路径

这样即使 hook 从目标仓库目录触发，也会稳定读取 `~/.ccclaw/ops/config/config.toml`。

### 3. managed hook 替换逻辑兼容新旧命令形态

同一文件中同步放宽 managed hook 识别：

- 旧形态：`ccclaw claude-hook ...`
- 新形态：`ccclaw --config ... claude-hook ...`

避免 `.claude/settings.local.json` 中叠加重复 managed hook。

## 测试

补充并保持以下回归验证：

1. `src/cmd/ccclaw/main_test.go`
   - 无 `CCCLAW_TASK_ID` 时静默跳过
   - 无 hook 目录 env 时，可从 `config.toml` 的 `paths.app_dir` 推导状态目录
   - 显式 `CCCLAW_HOOK_STATE_DIR` 仍优先覆盖
2. `src/internal/claude/hooks_test.go`
   - 校验 managed hook 命令带显式 `--config`
   - 校验旧 managed hook 会被替换，自定义 hook 继续保留

## 验证结果

执行：

```bash
cd /opt/src/ccclaw/src
go test ./cmd/ccclaw ./internal/claude ./internal/app
```

结果：

```text
ok
```

## 结论

本轮完成后，`ccclaw` 对 Claude hook 的边界变为：

1. 人工启动的 Claude Code：完全忽略
2. 受管任务的动态身份：继续由 `CCCLAW_TASK_ID` 运行时注入
3. 静态安装路径：统一由 `config.toml` 提供，并在 managed hook 中显式传入

这样既消除了人工使用 Claude Code 时的误告警，也避免继续把静态配置绑在脆弱的 env 约定上。
