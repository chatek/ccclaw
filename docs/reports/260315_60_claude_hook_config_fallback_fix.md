# 260315_60_claude_hook_config_fallback_fix

## 背景

Issue [#60](https://github.com/41490/ccclaw/issues/60) 追踪的第二个缺陷指出：`ccclaw claude-hook stop` 在用户交互式 Claude Code session 中被触发时，会因为缺少 `CCCLAW_HOOK_STATE_DIR` / `CCCLAW_APP_DIR` 环境变量而报错。

本轮用户进一步明确要求：

1. 这类路径不应只依赖 executor 注入环境变量
2. 常驻配置应优先来自 `config.toml`
3. 对尚未修订到位的部分直接开始修订

## 根因复核

变更前 `src/cmd/ccclaw/main.go` 中的 `claude-hook` 入口存在两个结构性问题：

1. `stateDir` 只通过以下环境变量解析：
   - `CCCLAW_HOOK_STATE_DIR`
   - `CCCLAW_APP_DIR`
2. 即使是用户交互式 session，也会进入受管任务分支并在缺少这些环境变量时报错

但仓库当前配置基线里，本来已经有稳定的 `paths.app_dir`：

- 配置示例：`src/ops/config/config.example.toml`
- 运行时统一配置加载：`src/internal/config/config.go`

因此问题不在“缺配置”，而在于 `claude-hook` 入口没有优先使用配置兜底。

## 修订内容

本轮在 `src/cmd/ccclaw/main.go` 中完成以下收口：

1. 将 `claude-hook` 入口逻辑抽成可测试 helper
   - `handleClaudeHook`
   - `parseClaudeHookEvent`
   - `resolveClaudeHookStateDir`

2. 调整 hook 上下文判定
   - 若 `CCCLAW_TASK_ID` 为空，视为非受管任务上下文
   - 交互式 Claude Code session 的 hook 直接静默退出，不再报错

3. 调整 hook 状态目录解析优先级
   - 优先：`CCCLAW_HOOK_STATE_DIR`
   - 次优：`CCCLAW_APP_DIR`
   - 兜底：`config.toml` 中 `paths.app_dir` 推导出的 `var/claude-hooks`

4. 保留运行态覆盖能力
   - executor 注入的环境变量仍然有效
   - 这样不会破坏受管任务已有运行时上下文

## 测试补充

在 `src/cmd/ccclaw/main_test.go` 中新增三组回归测试：

1. `TestHandleClaudeHookSkipsWhenTaskContextMissing`
   - 无 `CCCLAW_TASK_ID` 时静默跳过

2. `TestHandleClaudeHookUsesConfigAppDirWhenEnvMissing`
   - 缺少 hook 相关环境变量时，能从 `config.toml` 的 `paths.app_dir` 推导状态目录并落盘

3. `TestHandleClaudeHookPrefersExplicitHookStateDir`
   - 若显式设置 `CCCLAW_HOOK_STATE_DIR`，仍优先使用该目录而不是配置默认值

## 验证结果

执行：

```bash
cd /opt/src/ccclaw/src
go test ./cmd/ccclaw
```

结果：

```text
ok  	github.com/41490/ccclaw/cmd/ccclaw	0.620s
```

## 结论

本轮已将 `claude-hook` 的状态目录解析从“只依赖环境变量”修订为：

1. 受管任务：环境变量覆盖
2. 常规调用：配置文件兜底
3. 无 task 上下文：静默退出

这样既满足 `config.toml` 作为常驻配置事实来源的要求，也避免继续对交互式 Claude Code session 产生无意义告警。
