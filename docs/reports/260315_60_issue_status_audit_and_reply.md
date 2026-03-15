# 260315_60_issue_status_audit_and_reply

## 背景

用户要求获取 Issue [#60](https://github.com/41490/ccclaw/issues/60)，追查对应代码，确认是否已经解决；若未完全解决，则回帖给出处理方案。

## 前置核查

本轮先按仓库约束执行 Issue 决策核查，再下钻代码：

1. 使用 `gh issue view 60 --repo 41490/ccclaw --json ...` 获取 Issue 主体，确认截至核查时无额外评论与拍板冲突。
2. 复核 `main` 分支最近提交，确认与 #60 直接相关的修复历史。
3. 下钻以下代码路径核对实际状态：
   - `src/internal/executor/executor.go`
   - `src/cmd/ccclaw/main.go`
   - `src/internal/claude/hooks.go`

## 核对结果

截至 `2026-03-15`，在 `main@e7c3bfb` 上核对，Issue #60 属于“部分已解决”：

### 1. `--verbose` 缺失已解决

- `2026-03-14` 的提交 `25c29db`（`fix(#59): 修复 stream-json 缺 --verbose 及汇报型 Issue 豁免执行`）已在 `src/internal/executor/executor.go:656-669` 为 `stream-json` 模式补上 `--verbose`
- 当前代码中可见明确注释：`Claude Code CLI 2.1.76+ 要求 --print 模式下 stream-json 必须附带 --verbose`
- 因此 #60 的第一部分已被 #59 同步收口

### 2. `claude-hook stop` 无上下文报错仍未解决

- `src/cmd/ccclaw/main.go:555-564` 仍在 `stateDir == ""` 时直接返回：
  - `缺少 CCCLAW_HOOK_STATE_DIR 或 CCCLAW_APP_DIR`
- `src/internal/claude/hooks.go:112-116` 仍要求 `taskID` 非空：
  - `缺少 CCCLAW_TASK_ID`
- 现有实现没有区分：
  - `ccclaw executor` 注入环境变量的受管任务上下文
  - 用户交互式 Claude Code session 的无上下文 stop hook

## 最小复现

在当前仓库直接执行：

```bash
cd /opt/src/ccclaw/src
go run ./cmd/ccclaw claude-hook stop
```

现场结果仍为：

```text
缺少 CCCLAW_HOOK_STATE_DIR 或 CCCLAW_APP_DIR
exit status 1
```

说明 #60 的第二部分仍可稳定复现，不是仅存在于静态阅读中的假阳性。

## Issue 回帖内容摘要

已在 Issue #60 发布核对结论与最小修复方案评论：

- 评论链接：<https://github.com/41490/ccclaw/issues/60#issuecomment-4062519551>

回帖要点：

1. 明确标注当前状态为“部分已解决”
2. 说明第一部分已由 `25c29db` 收口
3. 说明第二部分在当前 `main` 仍未修复，并附最小复现结论
4. 给出最小修复方案：
   - 仅在 `stateDir == "" && taskID == ""` 时静默退出
   - 若 `taskID` 已设置但缺少 `stateDir`，继续报错
   - 保持 `claude.HandleHook()` 对真实配置异常的硬错误
5. 给出回归测试边界：
   - 无上下文 stop hook 退出 `0`
   - 半配置异常继续报错
   - 完整 executor 上下文下 `Stop` 事件正常落盘

## 结论

Issue #60 当前不应判定为“已解决”，更准确的结论是：

1. `--verbose` 缺失已解决
2. `claude-hook stop` 无上下文报错未解决

本轮已完成：

1. Issue 状态核对
2. 代码追查与最小复现
3. Issue 回帖给出最小修复方案

本轮未修改运行代码与发布资产。
