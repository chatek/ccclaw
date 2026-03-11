# 260311_20_install_prompt_readonly_claude

## 背景

Issue #20 反馈两个安装问题：

- `install.sh` 在交互模式输入单字母 `l` 后直接因 `input: unbound variable` 退出
- 安装过程会碰触现有 Claude 环境，和“只读探查、不要改现有配置”的预期冲突

Issue 评论最终拍板为：

- 已存在 Claude 配置时仅只读探查，不做后续额外处理
- `rtk init --global` 移出默认安装路径
- 不默认补装 plugins / marketplace / `example-skills`
- 交互模式同时兼容单字母与完整单词输入
- 脚本、测试、文档一起修改

## 根因

### 1. 交互崩溃根因

`src/dist/install.sh` 中：

- `prompt_mode()` 调用 `prompt_default input ...`
- `prompt_default()` 内部又声明了同名局部变量 `input`
- 在 `set -u` 下，调用者作用域里的 `input` 未被正确赋值，随后读取时直接触发 `unbound variable`

这不是单字母映射逻辑本身错误，而是 Bash 动态作用域下的同名局部变量遮蔽。

### 2. Claude 环境被误改的根因

安装脚本此前把“只读探查”和“主动配置”混在一起：

- 前者包括 `claude --version`、`claude auth status --json`、`claude plugin list`
- 后者包括 `claude plugin marketplace add`、`claude plugin install`、`rtk init --global`

这会让安装器在默认路径下越过“探查”边界，触碰现有 `~/.claude` 生态状态。

## 本次修改

### 安装脚本

- 修复 `prompt_default()` 变量遮蔽，把内部临时变量改为 `response`
- `prompt_mode()` 增加输入首尾空白裁剪，统一支持单字母和完整单词
- 删除默认 `configure_claude_assets()` 路径，不再自动：
  - 追加 marketplace
  - 安装 plugins
  - 执行 `rtk init --global`
- 安装流程说明与完成摘要明确标注：
  - Claude 生态默认只读探查
  - 不自动修改 marketplace / plugins / `rtk` 全局配置

### 升级脚本

- `src/dist/upgrade.sh` 不再宣称自动刷新 Claude 资产
- 当外部传入 `REFRESH_CLAUDE_ASSETS=1` 时，仅打印“已停用自动改动”的提示

### 文档

- 更新根 README 的安装行为说明
- 更新 release 树 README
- 更新安装流程样例，明确“探查默认只读”

### 回归测试

在 `src/tests/install_regression.sh` 中新增：

- 交互模式接受单字母输入
- 交互模式接受完整单词输入
- 默认安装仅只读探查 Claude，不触发 `plugin marketplace add` / `plugin install`

同时修复测试辅助函数：

- `run_case_with_input()` 统一补末尾换行，避免 `read` 在 EOF 下误失败
- `grep` 断言统一加 `--`，避免 `--version` 一类样本被误当成 grep 参数

## 验证

已执行：

- `make test-install`
- `make test`

结果：

- install 回归测试全部通过
- Go 单元测试全部通过

## 结果

本次收口后：

- `install.sh` 已不再因单字母输入崩溃
- 默认安装路径不会自动改写现有 Claude marketplace / plugins / `rtk` 全局状态
- 文档、release 源和回归测试已与新决策同步
