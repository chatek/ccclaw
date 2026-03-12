# 260312_32_phase1_duckdb_archive_integration

## 背景

在 `260312_32_phase1_jsonl_state_store_foundation.md` 中，`Phase 1` 已完成 SQLite 主路径替换：

- `state.json` 承担任务快照
- `events/token` 改为周级 JSONL
- `Store` API 保持兼容

但当时仍缺少 `Phase 1` 下半段：

1. `DuckDB CLI` 适配
2. `ccclaw archive` 子命令
3. 每周 archive systemd 定时器

本轮继续把这三块补齐。

## 本轮实现

### 1. 新增 DuckDB CLI 适配器

新增：

- `src/internal/adapters/query/duckdb.go`
- `src/internal/adapters/query/duckdb_test.go`

提供能力：

- `Query(ctx, sql)`：执行 `duckdb :memory: -json -c <sql>`
- `Execute(ctx, sql)`：执行非 JSON 输出 SQL
- `CopyJSONLToParquet(ctx, source, target)`：把 JSONL 导出为 Parquet(zstd)

约束：

- 所有外部命令都带超时
- 不引入 CGO
- 错误直接回传，不吞 CLI stderr

### 2. 新增 `ccclaw archive` 子命令

新增：

- `src/internal/archive/archive.go`
- `src/internal/archive/archive_test.go`
- `src/cmd/ccclaw/archive.go`

行为：

- 扫描 `var/` 下的 `events-*.jsonl` / `token-*.jsonl`
- 跳过当前周活动文件
- 对历史周文件：
  - 先校验哈希链
    - `events` 走 `VerifyChain`
    - `token` 走 `VerifyTokenChain`
  - 再导出到 `var/archive/*.parquet`

当前策略刻意保守：

- **只导出，不删除旧 JSONL**

原因：

- 当前 `Store` 统计与 journal 仍直接扫描 JSONL
- 如果本轮就把旧 JSONL 轮转走，会立刻导致现有统计丢历史

因此，这一版 archive 是“安全导出版”，先把 Parquet 资产生成出来，为后续读取切换做准备。

### 3. 新增 archive systemd 单元并接入安装链路

新增 release 资产：

- `src/dist/ops/systemd/ccclaw-archive.service`
- `src/dist/ops/systemd/ccclaw-archive.timer`

同步修改：

- `src/internal/scheduler/systemd.go`
- `src/internal/scheduler/systemd_units.go`
- `src/dist/install.sh`

效果：

- `ManagedSystemdTimers()` 现在包含 `ccclaw-archive.timer`
- `GenerateSystemdUnitContents()` 会生成 archive service/timer
- `install.sh` 在 systemd 模式下会：
  - 写入 archive 单元
  - `enable --now` archive timer
  - 重装时一并 `restart`

固定 schedule：

```text
Mon 02:00:00 Asia/Shanghai
```

这是按 Issue #32 已定规格硬编码的周归档时间，不额外引入配置项。

### 4. 同步文档与回归

更新：

- `src/dist/README.md`
- `src/ops/examples/app-readme.md`
- `src/dist/ops/examples/app-readme.md`
- `src/tests/install_regression.sh`
- `src/cmd/ccclaw/main_test.go`
- `src/internal/scheduler/systemd_test.go`

新增覆盖：

- fake `duckdb` 场景下的 query/copy 测试
- `ccclaw archive` 命令测试
- archive timer 生成测试
- install.sh 自动启用/重启 timer 列表含 archive timer

## 验证

执行：

```bash
cd src
go test ./...
cd ..
bash src/tests/install_regression.sh
```

结果：

- 全部通过

## 当前结论

Issue #32 的 `Phase 1` 已完成到一个可部署状态：

- 运行时主存储已不依赖 SQLite
- 历史周 JSONL 已可导出为 Parquet
- archive 已具备命令入口与 systemd 定时器

## 后续建议

后续如果继续优化，建议按以下顺序推进：

1. 让 `stats/journal` 逐步优先读 Parquet / DuckDB，而不是只扫 JSONL
2. 完成“导出后轮转旧 JSONL”的真正归档闭环
3. 把 `doctor`、README、配置示例中的 `state_db` 语义逐步收口为 `varDir + archive`
4. 再评估是否需要把 archive schedule 从硬编码提升为配置项
