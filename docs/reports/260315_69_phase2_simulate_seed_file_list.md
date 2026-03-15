# 260315_69_phase2_simulate_seed_file_list

## 背景

Issue #69 的 Phase 2 要求补齐 `install.sh --simulate` 在 `home_repo` seed 场景下的预检信息。

此前 simulate 模式虽然会说明“将执行 seed/commit”，但不会展开实际将要 `git add` 的文件列表，用户无法在真实执行前确认受影响范围。

## 根因

核查 `src/dist/install.sh` 后确认：

1. `commit_home_repo_seed()` 的 simulate 分支原先只有一行总括日志
2. 更关键的是，`main()` 在 `SIMULATE=1` 时会在真正安装阶段之前直接 `print_summary` 并退出

这意味着即便在 `commit_home_repo_seed()` 内补打印逻辑，也不会被 simulate 主路径触发。

## 实现

### 1. 抽取统一的 seed 文件清单函数

新增 `list_home_repo_seed_relative_files()`：

- 收集 `dist/kb/**/CLAUDE.md`
- 收集当前 `HOME_REPO/kb/**/CLAUDE.md`
- 追加根目录受管文件：`README.md`、`CLAUDE.md`、`.gitignore`
- 统一去重、排序

这样可以同时覆盖：

- 首装时即将从 release 模板写入的受管文件
- 本地已有 `home_repo` 中额外存在的 `kb/**/CLAUDE.md`

从而让 simulate 输出与真实 `git add` 逻辑保持一致。

### 2. simulate 模式前移预览逻辑

新增 `preview_home_repo_simulation()`，在 `main()` 的 simulate 早退分支中显式调用：

- `init`：打印初始化本体仓库
- `remote`：打印 clone 动作
- `local`：打印 attach 动作
- 之后统一调用：
  - `seed_home_repo_tree()`
  - `commit_home_repo_seed()`
  - `init_jj_colocate_repo()`

这样 simulate 路径无需真正落盘，也能输出完整的 `home_repo` seed 预期动作与文件清单。

### 3. commit_home_repo_seed simulate 输出补全

`commit_home_repo_seed()` 在 simulate 模式下现在会输出：

```text
[simulate] 将要 add 的文件列表：
[simulate]   - .gitignore
[simulate]   - CLAUDE.md
[simulate]   - README.md
...
```

同时，真实执行路径也改为复用同一个文件清单函数，避免 simulate 与真实 `git add` 再次漂移。

## 回归

在 `src/tests/install_regression.sh` 新增：

- `test_simulate_lists_seed_files_to_add`

覆盖点：

1. 本地已有 git `home_repo`
2. 额外预置自定义文件 `kb/custom/CLAUDE.md`
3. 运行 `--simulate`
4. 验证日志中同时出现：
   - 根目录受管文件
   - release 模板内的 `kb/**/CLAUDE.md`
   - 本地已有的 `kb/custom/CLAUDE.md`

## 验证记录

执行命令：

```bash
bash -n src/dist/install.sh src/tests/install_regression.sh
cd src && bash tests/install_regression.sh
```

结果：

- 语法检查通过
- install 回归测试全部通过
- simulate 现在可完整预览 `home_repo` seed add 列表

## 结论

Issue #69 Phase 2 已完成。

下一阶段进入 Phase 3：

- 先产出升级路径与首装路径分离设计
- 再把 `upgrade.sh` / `install.sh` 的 `home_repo` 刷新链路拆分为独立的 upgrade 专用入口
