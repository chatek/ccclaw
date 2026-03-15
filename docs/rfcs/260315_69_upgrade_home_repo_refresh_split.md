# 260315_69_upgrade_home_repo_refresh_split

## 背景

Issue #69 Phase 3 要求把升级路径与首次安装路径分离，避免 `upgrade.sh` 继续复用 `init_or_attach_home_repo()` 的首装语义。

当前实现的问题不是“函数名不准确”这么简单，而是升级阶段实际上只掌握：

- `paths.app_dir`
- `paths.home_repo`

并不知道历史安装时到底走过：

- `init`
- `remote`
- `local`

因为这些模式信息当前并未持久化进 `config.toml`。

## 现状问题

`upgrade.sh` 当前只是下载 release 后转调 `install.sh`，而 `install.sh` 默认会进入：

- `init_or_attach_home_repo()`
- `seed_home_repo_tree()`
- `commit_home_repo_seed()`

这会把升级路径继续当成“再做一次首装”，从而带来两个概念混淆：

1. 升级时仍会触发首装专属逻辑
   - 首次写入本体 `README.md`
   - 首次写入本体 `CLAUDE.md`
   - 首次初始化 `.gitignore`
2. 升级时对 `home_repo` 的操作边界不清
   - 看起来像“重新初始化本体仓库”
   - 实际目标却只是“无损刷新受管模板”

## 约束与事实

### 1. upgrade 阶段无法可靠还原历史 mode

当前配置只记录 `paths.home_repo`，没有记录：

- `home_repo_mode`
- `home_repo_remote`

因此 `upgrade.sh` 无法在不引入额外 schema 迁移的前提下，可靠区分“这个目录最初是 init 还是 remote 还是 local”。

### 2. 升级阶段真正稳定的事实只有一个

升级时唯一可信的不变量是：

- `home_repo` 已经存在，并且它就是本机当前使用的本体仓库目录

因此 upgrade 的正确抽象不应再是“重复执行 init|remote|local 三选一”，而应收敛成：

- 对现有 `home_repo` 执行原位受管刷新

### 3. 三种 mode 的 upgrade 差异应如何理解

虽然升级时无法再复原首装来源，但三种 mode 在升级阶段仍可整理出清晰边界：

- `init`
  - 历史上由安装器 `git init`
  - 升级时只需原位刷新受管模板
- `remote`
  - 历史上由安装器 `git clone`
  - 升级时只需原位刷新受管模板
  - 若已有 upstream，则继续输出 ahead 提示
- `local`
  - 历史上接管用户已有仓库
  - 升级时同样只应原位刷新受管模板

结论是：

- 三种 mode 在升级阶段的行为应统一收敛
- 差异体现在仓库自身是否带 remote / upstream
- 不体现在“是否再次执行 init/clone/首装根文件初始化”

## 设计决策

### 决策 1：新增 upgrade 专用开关

在 `install.sh` 新增：

```text
--upgrade-home-repo
```

由 `upgrade.sh` 显式传入。

目的：

- 让 upgrade 路径在入口层就与首装路径分叉
- 避免继续借用默认 `HOME_REPO_MODE=init` 的语义

### 决策 2：upgrade 路径只刷新 `kb/**/CLAUDE.md`

upgrade 专用函数只处理：

- `dist/kb/**/CLAUDE.md`

不再触碰首装专属根文件：

- `README.md`
- `CLAUDE.md`
- `.gitignore`

理由：

- 这些根文件并不是“升级必然需要刷新的受管模板”
- 它们属于首次初始化边界，继续在 upgrade 中写入只会放大概念混淆

### 决策 3：升级要求 `home_repo` 已存在且为 git 仓库

upgrade 不再兜底创建新目录、执行 `git init` 或 `git clone`。

如果 `home_repo` 不存在，或不是 git 仓库，则直接失败。

理由：

- 升级阶段不存在“重新初始化本体仓库”这一合法语义
- 一旦 upgrade 自动新建目录，用户会误以为原有记忆被安全继承，风险更大

### 决策 4：保持 Phase 1 的版本化提交审计约定

为兼容 Phase 1 已落地的审计语义，upgrade 路径的自动提交仍沿用：

```text
seed ccclaw home repo (v<version>)
```

但提交范围收缩为：

- `kb/**/CLAUDE.md`

这样可以保持：

- 版本审计口径稳定
- 升级写入边界变窄

## 预期结果

升级后将形成两条明确路径：

### 首次安装路径

- `init_or_attach_home_repo()`
- `seed_home_repo_tree()`
- `commit_home_repo_seed()`

负责：

- 仓库初始化/接管
- 首次根文件建立
- 初始受管模板落盘

### 升级路径

- `refresh_managed_home_repo_templates()`
- `merge_kb_contracts()`
- `commit_home_repo_refresh()`

负责：

- 原位刷新受管 `kb/**/CLAUDE.md`
- 保持用户已有记忆内容不被首装逻辑再次触碰

## 验收标准

1. `upgrade.sh` 调用 `install.sh` 时显式走 upgrade 专用分支
2. 升级回归中，`home_repo` 缺少 `README.md` / `CLAUDE.md` / `.gitignore` 时，不会被自动补建
3. `kb/**/CLAUDE.md` 仍可正常刷新并提交
4. 已有 remote/upstream 的仓库仍可继续看到 ahead 提示
