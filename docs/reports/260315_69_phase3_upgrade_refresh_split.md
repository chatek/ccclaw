# 260315_69_phase3_upgrade_refresh_split

## 背景

Issue #69 Phase 3 的目标是把升级路径与首装路径分离，避免 `upgrade.sh` 继续复用 `install.sh` 中的首装语义。

本轮先产出设计文档：

- `docs/rfcs/260315_69_upgrade_home_repo_refresh_split.md`

随后再按设计落代码。

## 设计结论

设计阶段确认了一个关键事实：

- 当前 `config.toml` 只持久化 `paths.home_repo`
- 并没有记录 `home_repo_mode` / `home_repo_remote`

因此升级阶段无法可靠还原历史上的 `init|remote|local` 选择。

基于这个事实，本轮没有去补一层“伪装成还能识别首装 mode”的逻辑，而是直接把 upgrade 抽象收敛为：

- 对现有 `home_repo` 做原位受管刷新

也就是说：

- 首装阶段仍保留 `init|remote|local`
- 升级阶段统一收敛为 `upgrade-refresh`

## 实现

### 1. 新增 upgrade 专用入口

`src/dist/install.sh` 新增：

- `--upgrade-home-repo`

`src/dist/upgrade.sh` 调用 release tree 安装器时，显式带上该参数：

```bash
install.sh --yes --app-dir ... --home-repo ... --upgrade-home-repo
```

这样升级链路在入口层就与首装路径分叉，不再继续借用默认的 `HOME_REPO_MODE=init`。

### 2. 新增 upgrade 专用刷新函数

在 `src/dist/install.sh` 新增：

- `list_home_repo_refresh_relative_files()`
- `commit_home_repo_refresh()`
- `refresh_managed_home_repo_templates()`
- `preview_home_repo_upgrade_simulation()`

行为边界：

- 只刷新 `dist/kb/**/CLAUDE.md`
- 只提交 `kb/**/CLAUDE.md`
- 不再写入首装根文件：
  - `README.md`
  - `CLAUDE.md`
  - `.gitignore`

### 3. upgrade 路径改为强约束现有仓库

`refresh_managed_home_repo_templates()` 现在要求：

- `HOME_REPO` 目录必须已存在
- `HOME_REPO/.git` 必须存在

否则直接失败，不再尝试：

- 自动 `git init`
- 自动 `git clone`
- 自动补做首装初始化

这样可以避免升级阶段悄悄变成“重新创建一个新本体仓库”。

### 4. 首装与升级的用户输出已区分

`print_flow()` / `print_summary()` 在 upgrade 分支下会显示：

- `本体仓库模式: upgrade-refresh`
- `本体仓库来源: upgrade-refresh -> <path>`

不再继续把升级流程说成 `init|remote|local` 之一。

### 5. 保持 Phase 1 的版本化提交审计约定

为了兼容 Phase 1 已落地的审计口径，upgrade 刷新提交仍沿用：

```text
seed ccclaw home repo (v<version>)
```

但提交范围已经收缩为：

- `kb/**/CLAUDE.md`

## 回归

更新 `src/tests/install_regression.sh` 中的升级回归：

- 预先创建一个已存在的 `home_repo` git 仓库
- 故意不放 `README.md` / `CLAUDE.md` / `.gitignore`
- 执行 `upgrade.sh`

新增验证点：

1. 升级日志中出现 `upgrade-refresh -> <home_repo>`
2. `home_repo/kb/CLAUDE.md` 被正常刷新
3. 根目录 `README.md` / `CLAUDE.md` / `.gitignore` 仍然不存在
4. 最新提交仍携带 release 版本号
5. 最新提交文件列表中不包含首装根文件

## 验证记录

执行命令：

```bash
bash -n src/dist/install.sh src/dist/upgrade.sh src/tests/install_regression.sh
cd src && bash tests/install_regression.sh
```

结果：

- 语法检查通过
- install / upgrade 回归测试全部通过
- 升级路径已与首装路径完成明确分离

## 结论

Issue #69 Phase 3 已完成。

至此，本 Issue 三个阶段均已落地：

1. Phase 1：seed commit 版本号 + ahead 提示
2. Phase 2：simulate 输出将要 add 的文件清单
3. Phase 3：upgrade 与首装路径分离，升级仅刷新受管模板
