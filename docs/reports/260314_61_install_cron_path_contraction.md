# 260314_61_install_cron_path_contraction

## 背景

- 对应 Issue: `#61 proposal: 将 cron 收缩为手工备选并撤出安装/升级主路径`
- 拍板评论：`issuecomment-4062063420`
- 执行方式：在独立 worktree `/opt/src/ccclaw-issue61`、分支 `issue61-cron-shrink` 中分阶段推进，避免干扰主工作树上的其它修改

本轮目标是把安装/升级主路径中的 `cron` 自动托管能力收缩为专家手工备选，并让代码、回归、文档三侧重新一致。

## 拍板口径

### 1. `auto`

- 统一改为手工提示
- `systemd --user` 不可用时，不再自动托管 `cron`

### 2. `cron` 能力保留边界

- 保留 CLI 专家工具
- 文档必须明确区分：
  - `CCClaw` 自动使用的 `systemd --user` 主路径
  - 专家人工配置 `cron` 的命令参数与样板

### 3. 安装器显式 `cron`

- 直接退出
- 失败指引统一指向 `~/.ccclaw/croncfg.md`

## 实施过程

### 阶段 1：先收缩测试与文档口径

- 修改根 `README`、`src/dist/README.md`、安装流示例、release notes 模板
- 统一改成：
  - `systemd --user` 是自动托管主路径
  - `cron` 是专家手工备选
  - `auto` 不再被描述为“自动降级到受控 cron”
- 修改 `src/tests/install_regression.sh`
  - 移除 `auto -> cron` 作为标准通过路径的断言
  - 移除安装器自动写 `cron` 的标准回归
  - 保留 `--remove-cron` / `disable-cron` 只清理托管块的验证

阶段提交：`0846719 docs: 收缩 cron 主路径口径`

### 阶段 2：再收紧安装器行为

- 修改 `src/dist/install.sh`
  - `auto` 在 `systemd --user` 不可用时统一落到 `none`
  - 输出精确手工 `cron` 样板、`scheduler status` 复核命令与 CLI 专家入口
  - 显式 `--scheduler cron` 直接失败
  - `none` 模式下不再自动改写或清理 `crontab`
  - 仅 `systemd` 主路径保留清理历史托管块，避免双重调度
- 扩充 `src/tests/install_regression.sh`
  - `auto -> none + 手工 cron 指引`
  - 显式 `cron` fail-fast
  - 继承旧 `scheduler.mode = "cron"` 时的非交互 fail-fast

阶段提交：`a7c567c feat: 收紧安装器 cron 路径`

### 阶段 3：补专家文档与安装同步

- 新增 `src/dist/croncfg.md`
  - 单独说明 `cron` 收缩策略
  - 区分自动 `systemd` 主路径与专家手工 `cron` 备选
  - 提供 CLI 专家工具、手工 `crontab` 样板、复核与回切步骤
- 安装/升级同步 `croncfg.md` 到程序目录
- README / release notes 模板统一改为指向 `~/.ccclaw/croncfg.md`
- 首装与升级回归新增 `croncfg.md` 落盘断言

## 关键变更点

### 安装器行为

- `--scheduler cron` 不再是可成功完成安装的分支
- `auto` 不再自动写受控 `cron`
- `none` 只保留提示态，不再暗中改写用户 `crontab`

### 文档与发布树

- 发布树新增 `croncfg.md`
- release notes 模板明确要求先读 `croncfg.md` 再手工启用 `cron`
- `src/dist/CLAUDE.md` 增补同步要求，避免后续发布漏带该文档

### 回归测试

- 旧“主路径包含 cron 自动托管”的断言全部下线
- 迁移清理能力仍保留自动化覆盖
- 新增旧配置 `cron` 继承场景，避免非交互路径卡死或静默回退

## 验证

执行：

```bash
go build -o src/dist/bin/ccclaw ./src/cmd/ccclaw
bash src/tests/install_regression.sh
go test ./cmd/ccclaw
bash -n src/dist/install.sh
```

结果：

- `install_regression` 全部通过
- `cmd/ccclaw` 单测通过
- `install.sh` 语法检查通过

## 结果

- 安装/升级主路径已经不再自动托管 `cron`
- `cron` 保留为专家手工工具，不再与自动安装链路混写
- 代码、测试、README、release tree 文档与 Issue 决策已重新对齐

## 残留事项

- CLI 级 `scheduler use cron` / `enable-cron` 仍保留，这是拍板后的专家手工能力，不在本轮继续收缩

## 后续同步

在本轮收尾阶段，已追加同步 `README_en.md`，将以下内容与中文主文档重新对齐：

- 当前阶段改为 `phase0.5`
- `systemd --user` 是唯一自动托管调度主路径
- `cron` 改为专家手工备选，并显式指向 `~/.ccclaw/croncfg.md`
- 安装器在 `systemd --user` 不可用时进入 `none + manual cron guidance`
