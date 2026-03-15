# Issue #66 `state_db` 废弃收口与自动回写

## 背景

Issue #66 已明确：`state_db` 已废弃，运行态统一以 `var_dir` 承载 `state.json + *.jsonl`，并要求把历史残留统一清掉，同时追加自动回写。

本轮核查发现，底层存储实际上早已按 `var_dir -> state.json + JSONL` 工作，但配置层、CLI 调用层、安装脚本、示例文档与测试仍在沿用 `state_db` 这个旧名字，再靠 `filepath.Dir(...)` 或 `resolveVarDir(...)` 做兼容。

这会带来两个问题：

1. 公开接口继续暴露错误心智模型，误导用户把运行态事实源理解为单个 SQLite 文件。
2. 各层重复携带“旧名词转目录”的兼容逻辑，导致废弃语义持续扩散。

## 本轮变更

### 1. 配置模型正式切换到 `paths.var_dir`

- `src/internal/config/config.go`
  - `PathsConfig` 移除 `StateDB`，改为 `VarDir`
  - `Validate()` 改为校验 `paths.var_dir`
  - 注释化配置渲染改为输出 `var_dir`
  - 路径归一化改为统一收敛到目录语义

### 2. 追加旧配置自动回写

- 在 `config.Load()` 中新增 `rewriteLegacyStateDBPaths()`
- 若检测到 `[paths] state_db = ".../state.db"`：
  - 先改写为 `var_dir = ".../var"`
  - 立即回写原配置文件
  - 再按新模型继续解析

这意味着老配置首次被任一命令加载时，就会自动完成一次无交互迁移，不再要求用户手工先跑 `config migrate`。

### 3. 运行链路统一使用 `VarDir`

- `src/internal/app/runtime_logging.go`
- `src/internal/app/runtime.go`
- `src/cmd/ccclaw/archive.go`
- `src/cmd/ccclaw/sevolver.go`
- `src/internal/sevolver/sevolver.go`
- `src/internal/sevolver/task_event_scanner.go`

以上调用点不再透传 `StateDB/StateDBPath`，统一改为 `VarDir`。

### 4. 安装与文档收口

- `src/dist/install.sh`
- `src/ops/config/config.example.toml`
- `src/dist/ops/config/config.example.toml`
- `src/ops/examples/install-flow.md`
- `src/dist/ops/examples/install-flow.md`
- `README.md`
- `README_en.md`
- `CLAUDE.md`
- `docs/plans/phase0_1_install_flow.md`

安装器生成的新配置、示例配置、README 与流程文档全部改为 `var_dir`。

### 5. 测试与回归基线同步

- 单元测试与 CLI 测试统一切到 `var_dir`
- 保留少量 legacy 测试输入，用于验证：
  - 老 `state_db` 配置可被识别
  - 加载时会自动回写为 `var_dir`
  - 迁移后配置中不再残留 `state_db`

## 自动回写策略

本轮自动回写只针对 `paths.state_db`：

- 触发点：`config.Load()`
- 行为：检测到旧键后立即原地回写
- 结果：后续命令都只看到 `var_dir`

未对 `approval.command` 复用同一策略，仍保持原有显式迁移门禁，因为该项属于审批语义变更，不宜在日常加载时静默重写。

## 验证

已执行：

```bash
cd /opt/src/ccclaw/src && go test ./...
```

结果：通过。

另做残留扫描后确认，仓库中剩余 `state_db` 仅存在于两类位置：

1. `src/internal/config/config.go` 的废弃兼容与错误提示逻辑
2. `src/internal/config/config_test.go`、`src/cmd/ccclaw/main_test.go` 中用于覆盖旧配置迁移场景的测试输入

## 结论

Issue #66 这轮已完成三件关键事：

1. 公开配置语义从 `state_db` 收口到 `var_dir`
2. 老配置在首次加载时自动回写，不再长期漂着旧键
3. 运行、安装、示例、测试与文档口径统一到 `state.json + JSONL`

后续若继续清理，可考虑在未来版本移除 `config.Load()` 对 `state_db` 的兼容解析，仅保留一个短期过渡窗口。
