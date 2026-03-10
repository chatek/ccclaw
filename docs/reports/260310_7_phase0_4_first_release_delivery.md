# 260310_7_phase0_4_first_release_delivery

## 背景

对应子 Issue：#7 `phase0.4: 首个可正确下载安装 release 的交付收口`

父 Issue：#6 `phase0.3: 项目状态清查、治理文件同步与统一 release 发布流`

本轮直接承接 #6 最后一次回复中的最新决策：

1. release 校验从 `gpg + minisign` 简化为 `SHA256SUMS`
2. 安装后的程序树固定在 `~/.ccclaw`，`upgrade.sh` 继续负责程序树升级
3. 本体仓库安装必须支持 `init|remote|local`
4. 任务仓库安装绑定只允许 `remote|local`
5. 安装完成后必须明确打印本体仓库来源、任务仓库绑定结果与后续迁移说明

## 本轮变更

### 1. 发布链路改为 HASH 校验

调整 `src/Makefile`：

- `make sign` 退化为 `make checksum` 的兼容别名
- `make archive` 只归档安装包与 `SHA256SUMS`
- `make release` 创建 release 前强制检查：
  - `gh auth status`
  - git 工作树必须干净

这样避免了两个问题：

1. 继续依赖 `minisign` 与最新决策冲突
2. 在脏工作树上发 release 会让 tag 指向旧提交、资产却来自未提交代码

### 2. 安装脚本支持三类本体仓库来源

重写 `src/dist/install.sh` 的仓库处理逻辑，新增：

- `--home-repo-mode init|remote|local`
- `--home-repo-remote`

对应行为：

- `init`：在目标目录初始化 git 仓库并写入 `dist/kb`
- `remote`：自动 clone 到目标目录，再补齐 `kb/` 初始树并提交
- `local`：接管本地已有 git 仓库，再补齐 `kb/` 初始树并提交

同时补充：

- `/opt/*` 等路径的权限修正
- 本体仓库 `README.md` / `CLAUDE.md` / `.gitignore` 初始化
- 安装摘要中的 upstream 配置指引

### 3. 安装脚本支持两类任务仓库绑定

新增：

- `--task-repo-mode none|remote|local`
- `--task-repo-remote`
- `--task-repo-local`
- `--task-repo`
- `--task-repo-path`
- `--task-repo-kb-path`

对应行为：

- `remote`：clone 到本地目标目录，默认落到 `~/repo-name`
- `local`：直接接管已有本地仓库
- 两种模式都会自动写入 `config.toml` 的 `targets`

### 4. 程序树安装内容补齐

安装脚本现在会把以下文件同步到 `~/.ccclaw`：

- `bin/ccclaw`
- `install.sh`
- `upgrade.sh`
- `ops/config/*`
- `ops/systemd/*`
- `ops/scripts/*`

从而让程序目录本身具备后续自解释与升级入口。

### 5. 治理文件与 README 同步

已同步更新：

- `AGENTS.md`
- `CLAUDE.md`
- `README.md`
- `src/dist/CLAUDE.md`
- `src/dist/README.md`
- `src/ops/examples/install-flow.md`
- `src/dist/ops/examples/install-flow.md`

统一后的口径：

- release 资产至少包含安装包与 `SHA256SUMS`
- 本体仓库支持 `init|remote|local`
- 任务仓库支持 `remote|local`
- `make release` 只能在干净工作树上执行

### 6. 关键记忆约定进入 release，并支持无损升级

按 Issue #7 最后一条提醒，已在 `src/dist/kb/` 下补齐关键 `CLAUDE.md`：

- `kb/CLAUDE.md`
- `kb/journal/CLAUDE.md`
- `kb/designs/CLAUDE.md`
- `kb/assay/CLAUDE.md`
- `kb/skills/CLAUDE.md`
- `kb/skills/L1/CLAUDE.md`
- `kb/skills/L2/CLAUDE.md`

这些模板已明确约定：

- `journal/` 使用 `年/月/` 目录树
- 每日日志命名为 `yyyy.mm.dd.{用户名}.ccclaw_log.md`
- 日记只允许名词、动词、副词，避免形容词
- `summary.md` 采用目录内逐级链接、逐级总结的整理方式
- 回忆优先从根 `summary.md`、年份 `summary.md`、月份 `summary.md` 渐进式下钻

同时，`src/dist/install.sh` 新增了关键契约文件的受管合并逻辑：

- 模板中的受管区块始终按 release 刷新
- 用户在保留区块中的本地增补会原样保留
- 如果本地旧文件还没有受管标记，会整体迁入保留区块，避免升级丢失

为保证这条升级链路可执行，本轮还顺手修正了两个根因：

- `.env` 写入时把 `umask 177` 限定在局部，不再污染后续 `kb/` 文件权限
- 本体仓库 `init` 模式改为 `git -C <dir> init`，避免目标目录已存在时初始化失败

## 验证

已执行：

```bash
bash -n src/dist/install.sh src/dist/upgrade.sh
cd src && go test ./...
cd src && make checksum
cd src && make sign
cd src && make archive
cd src && ./dist/bin/ccclaw -V
cd src && ./dist/bin/ccclaw target list --config ./ops/config/config.example.toml
cd src && bash ./dist/install.sh --simulate --yes --skip-deps --home-repo-mode init
cd src && bash ./dist/install.sh --simulate --yes --skip-deps --home-repo-mode remote --home-repo-remote 41490/ccclaw --task-repo-remote 41490/ccclaw
cd src && bash ./dist/install.sh --simulate --yes --skip-deps --home-repo-mode local --home-repo /tmp/ccclaw-local-home --task-repo-local /tmp/ccclaw-local-task
cd src && XDG_CONFIG_HOME=/opt/src/ccclaw/.ccclaw-verify-xxxx/xdg BIN_LINK=/opt/src/ccclaw/.ccclaw-verify-xxxx/bin/ccclaw \
  bash ./dist/install.sh --yes --skip-deps --app-dir /opt/src/ccclaw/.ccclaw-verify-xxxx/app --home-repo /opt/src/ccclaw/.ccclaw-verify-xxxx/home --home-repo-mode init
# 再次执行同一安装命令前，先向 /opt/src/ccclaw/.ccclaw-verify-xxxx/home/kb/journal/CLAUDE.md 的用户区块插入一行本机补充文本
gh auth status
cd src && make release VERSION=26.3.10.1142
```

验证结果：

- shell 语法检查通过
- Go 单元测试通过
- `make checksum` 可生成 `tar.gz + SHA256SUMS`
- `make sign` 可作为兼容别名正常执行
- `make archive` 可将产物归档到 `/ops/logs/ccclaw/<version>/`
- `ccclaw -V` 输出正确版本号
- 三种本体仓库模式与两种任务仓库模式在模拟安装中均能输出正确摘要
- 同一 `home_repo` 上连续执行两次 `init` 模式安装后，`kb/journal/CLAUDE.md` 的受管区块保持最新模板，本地插入的用户补充内容仍被保留
- `gh` 登录态正常
- `make release` 在当前脏工作树上按预期拒绝执行
- `make release VERSION=26.3.10.1142` 的前置条件已验证齐全，等待干净工作树执行

## 当前状态

本轮已经把“首个可正确下载安装 release”与“关键记忆契约可随升级无损演进”两部分都收口到位：

1. release 资产生成没问题
2. 本地归档没问题
3. 安装脚本已支持最新决策要求的仓库绑定模型
4. `kb/**/CLAUDE.md` 已进入 release，并具备受管刷新 + 用户保留的升级策略
5. 发布前会阻止脏工作树误发 release

## 后续建议

1. 提交并推送本轮变更后，在干净工作树执行一次 `make release`
2. release 创建成功后，自动回帖父 Issue #6：
   - tag
   - 资产名
   - `SHA256SUMS`
   - 下载与安装入口
3. 后续可继续补：
   - 远程本体仓库的 upstream 自动配置开关
   - 任务仓库迁移辅助命令
   - release 说明模板
