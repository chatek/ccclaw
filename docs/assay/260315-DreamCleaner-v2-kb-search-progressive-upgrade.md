# DreamCleaner v2：kb 检索层三层渐进式升级调研

> 调研日期：2026-03-15
> 调研模型：Claude Sonnet 4.6
> 状态：待审核
> 关联参考文档：
> - [260307-opus-DreamCleaner：CCClaw 的"睡眠记忆整合"系统设计](/opt/src/DAMA/docs/researchs/DevOps/VibeCoding/CCClaw/260307-opus-DreamCleaner：CCClaw的"睡眠记忆整合"系统设计.md)
> - [260307-opus-CCClaw DreamCleaner 基础设施选型](/opt/src/DAMA/docs/researchs/DevOps/VibeCoding/CCClaw/260307-opus-CCClaw%20DreamCleaner%20基础设施选型.md)

---

## 1. 正确理解问题与需求

DreamCleaner v1（2026-03-07）是一个完整的夜间记忆整合系统，依赖本地 Ollama + Qwen3-8B，七阶段流水线，涉及知识提取、去重、反思、分层晋升。这是一个长期目标架构，实现成本高：需要 GPU、Python 环境、Ollama 服务常驻。

本次调研聚焦的是 **DreamCleaner v2**——一个更务实的切入方向，核心问题不是"如何夜间整合记忆"，而是：

**当前 `ccclaw` 的 kb 检索能力太弱，随着 kb 文档积累，检索召回率和相关性会持续退化。如何用最低外部依赖、最高安全性的方式，分阶段把 kb 检索升级到工程可用水准？**

### 1.1 当前 kb 检索实现（现状）

当前 `src/internal/memory/index.go` 实现的是纯内存关键词匹配：

- **建索引**：WalkDir 遍历所有 `.md` 文件，解析 frontmatter + 标题 + 摘要行
- **检索**：对每个文档做 `strings.Contains()` 关键词匹配，计数为分数
- **中文支持**：通过 Unicode CJK 范围（`0x4e00-0x9fff`）做字符级切分
- **无 BM25**：分数 = 命中关键词个数，没有词频、文档频率、文档长度归一化
- **无向量**：完全没有语义检索能力

随着 `kb/skills/`、`kb/designs/`、`kb/assay/` 文档增多，这个实现会面临：
1. **相关性退化**：命中多个低权重词的噪声文档会排在真正相关的文档之前
2. **中文精度有限**：字符级切分对多字词（"调度器"、"状态机"）无法正确区分边界
3. **扩展性极限**：全量扫描，文档增多时 CPU 和内存线性增长

### 1.2 现有 DuckDB 使用情况

DuckDB 在 ccclaw 中目前只用于事件归档分析（`src/cmd/ccclaw/archive.go`），不涉及 kb 检索。`src/internal/adapters/storage/` 层使用 JSONL + state.json，无 SQLite。

### 1.3 v2 目标

用三层渐进式方案，把 kb 检索从"关键词计数"升级到"BM25 + 可选向量"，同时把事件 JSONL 冷层归档到 Parquet/DuckDB，整体保持：

- **零 LLM 依赖**（与 v1 的 Ollama 需求不同）
- **零额外常驻进程**
- **可逐步落地，每一层独立可回滚**

---

## 2. 外部事实核验

所有关键技术数据均已通过 DuckDuckGo 搜索核实，以下是逐项结论：

### 2.1 第一层：SQLite FTS5 + BM25 + 中文分词

**BM25 支持：已确认**
- SQLite FTS5 原生内置 BM25 排序函数，参数为 k1=1.2、b=0.75（硬编码）
- 查询语法：`SELECT * FROM fts WHERE fts MATCH ? ORDER BY rank`
- BM25 相比纯关键词计数，考虑词频、逆文档频率、文档长度归一化
- FTS5 采用增量实例列表加载，峰值内存远低于 FTS3/FTS4

**中文分词 "simple" 扩展：已确认（需外部扩展）**
- **重要修正**：SQLite 内置的 "simple" tokenizer 是英文标点切分，不原生支持中文字符级切分
- 用户方案中的 "simple 中文分词" 实际指 `wangfenjin/simple` 第三方扩展（GitHub），专为 FTS5 设计，支持中文字符级切分和拼音检索
- 替代方案：FTS5 + unicode61 tokenizer 对中文有基础支持（按 Unicode 类别切分），或自行实现 Go 端分词后存入 FTS5
- **推荐路径**：以 unicode61 tokenizer 作为起步（零外部依赖），需要精细中文支持时再引入 `wangfenjin/simple` `.so`

**检索准确率基线：已确认**
- 来自 Hybrid Search 实测数据（159 个多语言文档测试集）：
  - 纯 BM25：**58% 准确率**（基线已核实）
  - 纯向量搜索：略高于 BM25，但缺少精确关键词匹配
  - 混合 BM25 + 向量（RRF 融合）：**79% 准确率**（已核实）
  - 加 rerank：91%（需额外推理，超出当前目标范围）

**内存占用：已确认**
- SQLite FTS5 + BM25 整库加载 RAM < 100MB（对于 KB 级别的知识库）
- FTS5 增量实例列表设计确保不会全量加载到内存

### 2.2 第二层：sqlite-vec 向量检索

**存储计算修正：已确认（重要）**

用户构思中"100K × 1536 维仅需 ~19MB"需要补充说明：

- **Float32 存储**：100K × 1536 × 4B = **614MB**（不是 19MB）
- **Bit 量化存储（binary quantization）**：100K × 192B（1536 bits = 192 bytes）= **19.2MB** ✓
- **结论**：19MB 只在使用 **bit 量化**（`vec_bit` 类型）时成立；ccclaw kb 知识库文档量远小于 100K，实际占用更小
- 对于 1000 个文档（典型 kb 规模）：Float32 ≈ 6.1MB，Bit 量化 ≈ 192KB

**扩展加载方式：已确认**
- sqlite-vec 以 `.so`（Linux）/ `.dll`（Windows）形式加载
- 安装：`curl` 下载预编译包，或从 GitHub Releases 安装脚本
- SQLite CLI 中：`.load ./vec0`；Go 代码中通过 `sqlite3.RegisterExtension` 或构建时链接

**Go 集成路径：已确认**
- CGO 路径：使用 `github.com/mattn/go-sqlite3` + `github.com/asg017/sqlite-vec-go-bindings/cgo`（需 C 编译器，构建慢）
- WASM 路径（推荐）：使用 `github.com/ncruces/go-sqlite3` + `github.com/asg017/sqlite-vec-go-bindings/ncruces`（纯 Go，预编译 WASM sqlite-vec，无 C 编译器）
- **注意**：WASM 路径与 `modernc.org/sqlite` 不兼容；如 ccclaw 当前无 SQLite Go 依赖，引入时需选定一个

### 2.3 第三层：DuckDB 归档引擎

**无常驻进程：已确认**
- DuckDB 是内嵌式列存引擎，CLI 子进程按需启动，查询结束后进程退出
- 可直接查询 Parquet 文件，无需先导入（零拷贝读取，projection/filter 下推）

**资源限制配置：已确认**
- 语法验证：
  ```sql
  SET memory_limit = '400MB';
  SET threads = 1;
  ```
- DuckDB 默认占用 80% 物理内存；建议在资源受限环境下显式设置
- DuckDB 每线程最低需要 125MB；`threads=1` + `memory_limit=400MB` 是资源受限环境合理配置

**JSONL → Parquet 转换：已确认**
- 转换命令：
  ```sql
  COPY (SELECT * FROM read_json_auto('events.jsonl'))
    TO 'events.parquet' (FORMAT 'PARQUET', CODEC 'SNAPPY');
  ```
- 支持通配符批量转换：`read_json_auto('events-*.jsonl')`
- DuckDB 对 Parquet 查询有行组级过滤（zoneMap），适合时间范围查询场景

**混合检索 BM25 + 向量 RRF 融合：已确认（见 2.1）**
- BM25 58% → RRF 混合 79%，数据来源真实测试，非推测

---

## 3. 三层渐进式升级方案（设计）

### 3.1 第一层：SQLite FTS5 + BM25（零成本基线）

**目标**：替换 `src/internal/memory/index.go` 的关键词计数检索，升级为 BM25 排序

**技术方案**：
- 使用 `modernc.org/sqlite`（纯 Go，无 CGO，ccclaw 现有 Go 工具链可直接引入）
- 建立 FTS5 虚拟表，tokenizer 选 `unicode61`（内置，支持中文按 Unicode 类别分割）
- 每次 `ccclaw` 启动或 kb 有变更时，重建 FTS5 索引（内存中或落盘到 `~/.ccclaw/kb.db`）

**schema**：
```sql
CREATE VIRTUAL TABLE IF NOT EXISTS kb_fts
  USING fts5(path, title, summary, body, tokenize='unicode61');
```

**查询**：
```sql
SELECT path, title, summary, bm25(kb_fts) as score
FROM kb_fts WHERE kb_fts MATCH ?
ORDER BY rank LIMIT 10;
```

**与现有代码的关系**：
- `src/internal/memory/index.go` 的 `Build()` 和 `Match()` 接口保持不变（签名兼容）
- 内部实现替换：从文件 WalkDir → SQLite FTS5 查询
- 可选：FTS5 索引持久化到 `~/.ccclaw/kb_index.db`，避免每次重建

**回滚**：删除 `~/.ccclaw/kb_index.db`，回退到旧版关键词匹配

### 3.2 第二层：sqlite-vec 向量检索（按需升级）

**触发条件**：FTS5 BM25 检索准确率不足以满足需求（典型场景：语义近似但词汇不重叠，如"cron 调度"与"定时任务"）

**技术方案**：
- 在同一个 `kb_index.db` 中新增 `vec0` 虚拟表
- Embedding 模型选型：
  - **方案 A（无外部依赖）**：LLM 辅助查询扩展（不生成 embedding，而是用 LLM 扩充查询词，再走 FTS5）
  - **方案 B（本地 embedding）**：调用 Ollama API 生成 embedding（需 Ollama 运行）
  - **推荐当前阶段**：方案 A（零依赖），方案 B 作为下一步

- **Bit 量化存储**（方案 B 时）：
  ```sql
  CREATE VIRTUAL TABLE kb_vec USING vec0(embedding bit[768]);
  ```
  1000 文档 × 768 维 bit 量化 ≈ 96KB（可忽略）

- **RRF 融合**：BM25 结果 + 向量结果各自排名，通过 `1/(rank + 60)` 加权融合

### 3.3 第三层：DuckDB 冷层归档（事件 JSONL 归档）

**目标**：把超过 N 天的事件 JSONL 导出为 Parquet 归档，减少活跃 JSONL 扫描量

**现状**：`src/internal/adapters/storage/jsonl_store.go` 全量扫描 JSONL；历史数据积累后，`stats/journal/sevolver` 查询变慢

**技术方案**：
- 定期（每周/每月）把超过 30 天的 JSONL 记录导出到 Parquet
- 归档后从活跃 JSONL 中删除对应记录
- DuckDB CLI 按需查询归档数据，不常驻

**Bash 脚本骨架**：
```bash
archive_jsonl() {
  local cutoff_date="$1"   # 如 "2026-02-15"
  local var_dir="${2:-$HOME/.ccclaw/var}"

  # JSONL → Parquet（DuckDB CLI 子进程）
  duckdb -c "
    SET memory_limit='400MB'; SET threads=1;
    COPY (
      SELECT * FROM read_json_auto('$var_dir/events-*.jsonl')
      WHERE ts < '$cutoff_date'::TIMESTAMP
    ) TO '$var_dir/archive/events_before_${cutoff_date//-/}.parquet'
    (FORMAT 'PARQUET', CODEC 'SNAPPY');
  "

  echo "[done] 已归档 $cutoff_date 前的事件记录"
}

query_archive() {
  local var_dir="${1:-$HOME/.ccclaw/var}"
  duckdb -c "
    SET memory_limit='400MB'; SET threads=1;
    SELECT * FROM '$var_dir/archive/*.parquet'
    WHERE \$2
    ORDER BY ts DESC LIMIT \$3;
  "
}
```

---

## 4. 与 ccclaw 现有架构的适配

### 4.1 变更范围最小化原则

三层升级均不修改：
- 运行态事实来源（state.json + events JSONL + hashchain）
- `sevolver` 的 gap 聚合与 Issue 升级逻辑
- `stats/status/journal` 的计算口径
- 任何 systemd timer / cron 配置

**第一层（FTS5）**只影响 `src/internal/memory/index.go`，该模块仅被 `sevolver` 和技能检索路径调用，接口保持不变。

**第三层（DuckDB 归档）**通过新增 `ccclaw archive` 子命令（`src/cmd/ccclaw/archive.go` 已存在）实现，不修改现有存储读写路径。

### 4.2 与现有 DuckDB 使用的关系

现有 `archive.go` 已有 DuckDB 子进程调用先例。第三层归档脚本复用同一模式，参数约束（`memory_limit=400MB; threads=1`）直接沿用。

### 4.3 升级后的目录结构影响

```
~/.ccclaw/
├── var/
│   ├── events-YYYY-WW.jsonl        # 活跃事件（近 30 天）
│   ├── archive/
│   │   └── events_before_YYYYMMDD.parquet  # 冷层归档（第三层新增）
│   └── state.json
├── kb_index.db                     # FTS5 + 可选 vec0（第一/二层新增）
└── ...
```

---

## 5. 安全升级流程

### 5.1 总原则

- **每一层独立上线**，不绑定其他层
- **每一层可独立回滚**，回滚代价为删除新增文件或切换配置开关
- **不修改已有数据格式**：JSONL/hashchain 结构不变，Parquet 是独立副本（删除不影响原始 JSONL）
- **无缝降级**：FTS5 索引缺失时，`memory/index.go` 应自动降级到现有关键词匹配

### 5.2 推荐分阶段计划

```
阶段 0（准备）：
  - 在 staging/dev 环境测试 FTS5 查询效果
  - 确认 modernc.org/sqlite 引入后 Go 构建可通过
  - 拍板 tokenizer 选型（unicode61 还是 wangfenjin/simple .so）

阶段 1（第一层上线）：
  - 新增 FTS5 索引构建逻辑（build_kb_index）
  - memory.Index.Match() 内部切换为 FTS5 查询，降级保底保留
  - 上线后对比 sevolver 检索结果质量

阶段 2（评估 + 可选第二层）：
  - 收集 2 周真实检索日志，评估 BM25 是否足够
  - 若足够：暂停在第一层
  - 若不够：评估 LLM 查询扩展（零依赖）或 sqlite-vec（需选定 embedding 方案）

阶段 3（可选第三层）：
  - 当 JSONL 扫描时长超过 500ms（stats/journal 响应慢时）启动
  - 新增 archive 子命令，支持手动归档和查询归档
  - 配置 sevolver 定期触发归档（或手工 cron）
```

### 5.3 回滚步骤

| 层级 | 回滚操作 | 影响范围 |
|------|---------|---------|
| 第一层（FTS5） | 删除 `~/.ccclaw/kb_index.db` + 禁用 FTS 标志 | 仅检索质量，无数据丢失 |
| 第二层（vec） | 同上，或不初始化 vec0 表 | 同上 |
| 第三层（DuckDB 归档） | 保留原始 JSONL（归档为独立副本） | 完全可回滚，删除 parquet 即可 |

---

## 6. 脚本构思

### 6.1 kb_index_build.sh（第一层索引构建）

```bash
#!/usr/bin/env bash
# 构建或更新 kb FTS5 索引
# 依赖: sqlite3 CLI（系统自带）或 ccclaw 二进制内建

set -euo pipefail

KB_DIR="${1:-$(ccclaw config kb-dir 2>/dev/null || echo $HOME/.ccclaw/kb)}"
INDEX_DB="${2:-$HOME/.ccclaw/kb_index.db}"

# 初始化 FTS5 虚拟表
sqlite3 "$INDEX_DB" <<'SQL'
CREATE VIRTUAL TABLE IF NOT EXISTS kb_fts
  USING fts5(path UNINDEXED, title, summary, body, tokenize='unicode61');
SQL

# 重建索引
sqlite3 "$INDEX_DB" "DELETE FROM kb_fts;"

find "$KB_DIR" -name "*.md" | while read -r f; do
  title=$(grep -m1 '^# ' "$f" 2>/dev/null | sed 's/^# //' || basename "$f" .md)
  body=$(cat "$f")
  sqlite3 "$INDEX_DB" "INSERT INTO kb_fts(path, title, body) VALUES ('$f', '$title', '$body');"
done

echo "[done] kb FTS5 索引构建完成: $(sqlite3 "$INDEX_DB" "SELECT count(*) FROM kb_fts;") 条文档"
```

### 6.2 kb_search.sh（FTS5 + BM25 查询）

```bash
#!/usr/bin/env bash
# 查询 kb FTS5 索引（BM25 排序）

INDEX_DB="${HOME}/.ccclaw/kb_index.db"
QUERY="${*:?'用法: kb_search.sh <关键词>'}"

sqlite3 -json "$INDEX_DB" \
  "SELECT path, title, bm25(kb_fts) as score
   FROM kb_fts WHERE kb_fts MATCH '$QUERY'
   ORDER BY rank LIMIT 10;"
```

### 6.3 archive_events.sh（第三层 JSONL 冷归档）

```bash
#!/usr/bin/env bash
# 将 N 天前的 JSONL 事件归档为 Parquet
# 依赖: duckdb CLI (~/.local/bin/duckdb)

set -euo pipefail

DAYS="${1:-30}"
VAR_DIR="${2:-$HOME/.ccclaw/var}"
CUTOFF=$(date -d "-${DAYS} days" +%Y-%m-%d)
ARCHIVE_DIR="$VAR_DIR/archive"
DUCKDB="${DUCKDB_BIN:-$HOME/.local/bin/duckdb}"

mkdir -p "$ARCHIVE_DIR"

"$DUCKDB" -c "
  SET memory_limit='400MB';
  SET threads=1;
  COPY (
    SELECT * FROM read_json_auto('$VAR_DIR/events-*.jsonl')
    WHERE ts < '$CUTOFF'::TIMESTAMP
  ) TO '$ARCHIVE_DIR/events_before_${CUTOFF//-/}.parquet'
  (FORMAT 'PARQUET', CODEC 'SNAPPY');
"

echo "[done] 已归档 $CUTOFF 前事件到 $ARCHIVE_DIR/events_before_${CUTOFF//-/}.parquet"
echo "提示: 确认归档无误后，手工从活跃 JSONL 中删除对应记录"
```

---

## 7. 结论与建议

### 7.1 推荐实施顺序

**立即可做**（第一层，低风险，高价值）：
- 引入 `modernc.org/sqlite`，在 `memory/index.go` 中新增 FTS5 路径
- tokenizer 先用 `unicode61`（内置，无额外依赖），后续可切换
- 这一步不需要 Ollama、不需要 GPU、不需要新 systemd 服务

**按需评估**（第二层，中等成本）：
- 在第一层运行 2 周后，根据检索质量决定是否启用向量
- 优先考虑 LLM 查询扩展（无 embedding，让 Claude 扩充查询词后再走 FTS5）
- 若需 embedding：bit 量化可控制存储，WASM 路径可避免 CGO 编译问题

**配合现有 DuckDB**（第三层，中等成本，明确收益点）：
- JSONL 扫描时长超过阈值时再做，不提前过度工程化
- 归档脚本独立，不影响主流程

### 7.2 关键风险与注意事项

| 风险 | 说明 | 缓解措施 |
|------|------|---------|
| FTS5 中文准确率 | unicode61 按 Unicode 类别分割，多字词可能拆散 | 初期可接受，后续可引入 wangfenjin/simple .so |
| sqlite-vec Go 集成 | CGO vs WASM 选型影响构建系统 | 先用方案 A（LLM 查询扩展），有需要再引入 sqlite-vec |
| DuckDB 内存峰值 | 大 JSONL 转换时可能超过 400MB | 按时间分批转换（每次仅处理一个月） |
| FTS5 索引与 JSONL 来源不同 | FTS5 管 kb 检索，DuckDB 管事件归档，不能混用 | 明确文档和代码注释，两条路径独立 |

---

# refer. 参考

## A. 调研关键词（DuckDuckGo 验证入口）

- `SQLite FTS5 BM25 scoring` → 官方文档 https://www.sqlite.org/fts5.html
- `wangfenjin simple sqlite fts5 chinese` → https://github.com/wangfenjin/simple
- `sqlite-vec binary quantization storage` → https://alexgarcia.xyz/blog/2024/sqlite-vec-stable-release/index.html
- `asg017 sqlite-vec go bindings ncruces` → https://github.com/asg017/sqlite-vec-go-bindings
- `hybrid BM25 dense vector RRF accuracy 58 79` → https://medium.com/@pbronck/better-rag-accuracy-with-hybrid-bm25-dense-vector-search-ea99d48cba93
- `DuckDB memory_limit threads configuration` → https://duckdb.org/docs/stable/configuration/overview
- `DuckDB JSONL to Parquet COPY` → https://duckdb.org/docs/stable/guides/file_formats/query_parquet

## B. 官方/原始资料

- SQLite FTS5 官方文档（BM25 参数 k1=1.2, b=0.75 出处）
  https://www.sqlite.org/fts5.html

- wangfenjin/simple：FTS5 中文分词扩展
  https://github.com/wangfenjin/simple

- sqlite-vec 官方博客（v0.1.0 稳定版，bit 量化存储说明）
  https://alexgarcia.xyz/blog/2024/sqlite-vec-stable-release/index.html

- sqlite-vec binary quantization 文档
  https://alexgarcia.xyz/sqlite-vec/guides/binary-quant.html

- sqlite-vec Go 集成指南（CGO + WASM 两条路径）
  https://alexgarcia.xyz/sqlite-vec/go.html

- sqlite-vec-go-bindings（官方 ncruces WASM 路径）
  https://github.com/asg017/sqlite-vec-go-bindings

- ncruces/go-sqlite3（纯 Go WASM SQLite，支持 sqlite-vec）
  https://github.com/ncruces/go-sqlite3

- 混合检索 BM25 + Dense Vector RRF（58% → 79% 数据来源）
  https://medium.com/@pbronck/better-rag-accuracy-with-hybrid-bm25-dense-vector-search-ea99d48cba93

- Weaviate: Hybrid Search Explained（RRF 算法详解）
  https://weaviate.io/blog/hybrid-search-explained

- DuckDB Configuration（memory_limit, threads）
  https://duckdb.org/docs/stable/configuration/overview

- DuckDB Memory Management
  https://duckdb.org/2024/07/09/memory-management

- DuckDB Parquet 查询指南（零拷贝，projection pushdown）
  https://duckdb.org/docs/stable/guides/file_formats/query_parquet

- DuckDB JSONL → Parquet 转换（COPY 语句）
  https://github.com/duckdb/duckdb/discussions/6478

## C. 仓库内关联资料

- `src/internal/memory/index.go`（当前 kb 检索实现）
- `src/cmd/ccclaw/archive.go`（现有 DuckDB 归档子命令）
- `docs/superpowers/plans/2026-03-12-memory-architecture.md`（Phase 1 JSONL + DuckDB 架构计划）
- [260307 DreamCleaner v1 设计](/opt/src/DAMA/docs/researchs/DevOps/VibeCoding/CCClaw/260307-opus-DreamCleaner：CCClaw的"睡眠记忆整合"系统设计.md)
- [260307 DreamCleaner 基础设施选型](/opt/src/DAMA/docs/researchs/DevOps/VibeCoding/CCClaw/)

---

*调研完成时间：2026-03-15*
*调研模型：Claude Sonnet 4.6 (claude-sonnet-4-6)*
*所有技术数据已通过 DuckDuckGo 搜索核验*
