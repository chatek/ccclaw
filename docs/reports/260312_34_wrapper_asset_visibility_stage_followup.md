# 260312_34_wrapper_asset_visibility_stage_followup

## 背景

在 Issue #34 第一轮修复完成后，又根据回帖里的后续优化建议继续推进三项收口：

1. 将 `ccclaude` wrapper 从安装脚本内联模板抽成独立发布资产
2. 在 `doctor` / `status` 中显式展示 `claude` / `rtk` 的最终解析路径
3. 给 Issue 失败回帖补“启动阶段 / 执行阶段 / 收口阶段”分类

本轮只做已明确的工程化收紧，不改动任务门禁和执行语义。

## 改动

### 1. `ccclaude` 资产化

新增发布文件：

- [ccclaude](/opt/src/ccclaw/src/dist/ops/scripts/ccclaude)

同步修改：

- [install.sh](/opt/src/ccclaw/src/dist/install.sh)
- [README.md](/opt/src/ccclaw/src/dist/README.md)

调整后：

- release 树中显式包含 `bin/ccclaude`
- 安装时不再用 heredoc 现场拼 wrapper
- `install.sh` 只负责复制发布资产并设置权限

这样安装脚本与 wrapper 本体解耦，后续升级 wrapper 时不会再把模板逻辑埋在安装脚本内部。

### 2. `status` / `doctor` 输出解析路径

修改 [runtime.go](/opt/src/ccclaw/src/internal/app/runtime.go)：

- `status` 新增“执行器快照”区块
  - 入口配置
  - 入口解析
  - Claude 解析
  - RTK 解析
- `doctor` 将原“claude CLI”入口探查更名为“执行器入口”
- `doctor` 的 `claude CLI` / `rtk CLI` 现在会输出：
  - requested
  - resolved
  - version

这样现场排障时，不再只知道“命令可用/不可用”，还能直接看到最终命中了哪个二进制路径。

### 3. Issue 失败回帖阶段分类

修改 [reporter.go](/opt/src/ccclaw/src/internal/adapters/reporter/reporter.go)：

- 失败回帖新增 `阶段`
- 目前按错误特征划分为：
  - 启动阶段
  - 执行阶段
  - 收口阶段
- 同时把 `错误` 与 `诊断` 拆成两个字段

例如：

- `tmux 会话退出码=127` + `claude: not found` -> 启动阶段
- 超时/执行中断 -> 执行阶段
- JSON 解析失败 / 结果文件异常 -> 收口阶段

这样 Issue 上看到的失败信息更接近真实阶段，不再把所有问题都压成同一类描述。

## 测试

本轮新增/更新测试：

- [runtime_test.go](/opt/src/ccclaw/src/internal/app/runtime_test.go)
  - `status` 输出新增执行器快照断言
  - `describeExecutor()` 解析路径断言
- [reporter_test.go](/opt/src/ccclaw/src/internal/adapters/reporter/reporter_test.go)
  - 失败阶段分类断言
  - 失败回帖正文包含阶段/诊断断言

## 验证

已执行：

```bash
gofmt -w src/internal/app/runtime.go src/internal/app/runtime_test.go src/internal/adapters/reporter/reporter.go src/internal/adapters/reporter/reporter_test.go src/internal/executor/executor.go
bash -n src/dist/install.sh src/dist/bin/ccclaude
go test ./internal/app ./internal/adapters/reporter ./internal/executor
```

结果均通过。

## 结果

本轮完成后：

- wrapper 已成为 release 树中的显式资产，而不是安装脚本私有模板
- `status` / `doctor` 可以直接回答“最终跑的是哪个 claude/rtk”
- Issue 失败回帖首次具备阶段分类能力，能区分启动失败、执行失败和收口失败
