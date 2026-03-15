# 260315_69_phase1_home_repo_seed_version_ahead

## 背景

Issue #69 的 Phase 1 目标是完成两项低风险优化：

1. `home_repo` 的自动 seed commit 需要携带版本号，便于通过 `git log` 直接审计升级来源。
2. `home_repo` 已绑定 remote 时，需要明确提示当前分支相对远端的 `ahead` 数，减少用户忘记 push 的情况。

相关收口点集中在安装器 `src/dist/install.sh` 与升级入口 `src/dist/upgrade.sh`。

## 实现

### 1. seed commit 消息注入版本号

- 在 `src/dist/install.sh` 新增 `CCCLAW_VERSION` 环境变量入口。
- `commit_home_repo_seed()` 生成 commit message 时：
  - 未注入版本号，保持 `seed ccclaw home repo`
  - 注入版本号时，输出 `seed ccclaw home repo (v<version>)`

这样可以同时兼容：

- 本地直接执行安装器但未显式传版号的场景
- release 升级路径中由外层注入 tag 的场景

### 2. upgrade.sh 传递 release tag

- `src/dist/upgrade.sh` 在下载最新 release 后，把 tag 保存为 `RELEASE_TAG`
- 调用 release tree 中的 `install.sh` 时，显式传递：

```bash
CCCLAW_VERSION="$RELEASE_TAG" "$RELEASE_DIR/install.sh" ...
```

这样升级路径产生的 `home_repo` seed commit 可以稳定带上 release 版本号。

### 3. ahead 提示改为基于真实 git 状态

- `commit_home_repo_seed()` 在自动提交后新增 remote 探查
- 若检测到 remote，则读取 `git status --porcelain=v1 -b` 的首行
- 当状态中含有 `ahead N` 时，输出：

```text
[home_repo ahead N，建议执行：git -C ".../home-repo" push]
```

- 若检测到 remote 但当前分支尚未建立 upstream，则改为提示首次 `push -u`
- 若完全没有 remote，则明确说明跳过 ahead 提示，避免继续打印误导性的 `git push`

## 回归

在 `src/tests/install_regression.sh` 新增与扩展两类验证：

1. `test_home_repo_seed_commit_includes_version_and_ahead_hint`
   - 预置带 `origin` 与 upstream 的本地 `home_repo`
   - 传入 `CCCLAW_VERSION=26.03.15.0746`
   - 验证最新 commit 为 `seed ccclaw home repo (v26.03.15.0746)`
   - 验证安装日志包含 `home_repo ahead 1`

2. 扩展 `test_upgrade_downloads_release_and_migrates_config`
   - 验证 `upgrade.sh` 下载 release 后，最终 `home_repo` 最新提交带有 release tag 版本号

## 验证记录

执行命令：

```bash
bash -n src/dist/install.sh src/dist/upgrade.sh src/tests/install_regression.sh
cd src && make build
cd src && bash tests/install_regression.sh
```

结果：

- 语法检查通过
- 安装回归测试全部通过
- Phase 1 的版本化 seed commit 与 ahead 提示行为已落地

## 结论

Issue #69 Phase 1 已完成，后续可进入：

- Phase 2：`--simulate` 输出将要 add 的完整文件列表
- Phase 3：升级路径与首装路径分离设计与实现
