# 260312_32_phase0_skill_frontmatter_upgrade_protection

## 背景

Issue #32 已锁定实施顺序为 `Phase 0 -> 1 -> 2 -> 3`。

Phase 0 的目标是：当安装/升级流程刷新 `kb/skills` 下的受管 Markdown 时，保留由 `sevolver` 自动维护的 Skill frontmatter 字段，避免升级把运行期元数据冲回模板默认值。

在实际代码中核对后发现，计划文档写的是 `upgrade.sh`，但真正执行受管 Markdown 合并的是 `src/dist/install.sh` 内的 `merge_managed_markdown()`；`upgrade.sh` 只是下载 release 并转调安装器。因此本轮修订落在安装器真实入口，而不是表面包装层。

## 本轮实现

### 1. 在 `install.sh` 补齐 Skill frontmatter 保留链路

新增函数：

- `has_yaml_frontmatter()`
- `is_skill_markdown_path()`
- `extract_frontmatter_field()`
- `remove_frontmatter_field()`
- `append_frontmatter_blocks()`
- `preserve_skill_meta_fields()`

实现策略：

- 仅对 `kb/skills/**/*.md` 生效
- 仅在旧文件与新模板都带 YAML frontmatter 时执行
- 保留字段固定为：
  - `last_used`
  - `use_count`
  - `status`
  - `gap_signals`
- 若模板内已有这些字段，也以旧文件中的真实值覆盖，避免模板默认值把运行态元数据洗掉

挂接点：

- 在 `merge_managed_markdown()` 完成 managed/user 区块合成后，写回目标文件前执行 `preserve_skill_meta_fields`

额外补充：

- 增加 `CCCLAW_INSTALL_LIB_ONLY=1` 保护分支，允许回归测试安全 `source install.sh` 调用内部函数，而不触发完整安装流程

### 2. 同步 Skill 规范文档

更新文件：

- `src/dist/kb/skills/CLAUDE.md`
- `src/dist/kb/skills/L1/CLAUDE.md`
- `src/dist/kb/skills/L2/CLAUDE.md`

补充内容：

- `sevolver` 自动维护字段说明
- 升级必须保留这些字段原值
- `dormant` / `deprecated` 的语义约束
- `deprecated` 移入 `kb/skills/deprecated/` 后默认不再加载

### 3. 回归测试补齐

在 `src/tests/install_regression.sh` 新增一条场景：

- 构造 `kb/skills/L1/demo/CLAUDE.md` 模板与旧文件
- 模板含默认 `status: active` 与 `gap_signals: []`
- 旧文件含：
  - `last_used`
  - `use_count`
  - `status: dormant`
  - 多行 `gap_signals`
- 调用 `merge_managed_markdown()`
- 校验：
  - 受管区更新为新版内容
  - 用户区保留
  - `sevolver` 字段保留旧值且不重复
  - 模板默认 `status: active` / `gap_signals: []` 不会残留

## 验证

执行：

```bash
bash -n src/dist/install.sh
bash -n src/tests/install_regression.sh
bash src/tests/install_regression.sh
```

结果：

- shell 语法检查通过
- install 回归全量通过
- 新增 Skill frontmatter 保护场景通过

## 结论

Issue #32 的 `Phase 0` 已完成，且真实收口点已经从“计划中的 `upgrade.sh`”纠正为“实际生效的 `install.sh` 合并链路”。

这为后续 `Phase 2 sevolver` 自动维护 Skill 生命周期打下了升级保护基础；下一阶段应按 Issue 已定顺序进入 `Phase 1`，处理 JSONL + DuckDB 替代 SQLite `state_db`。
