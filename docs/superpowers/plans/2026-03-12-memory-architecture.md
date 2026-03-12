# Memory Architecture Upgrade Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 JSONL+DuckDB 替代 SQLite state_db，并新增 sevolver 自我成长服务，改善 CCClaw 长期记忆结构与自主进化能力。

**Architecture:** state.json 作原子可重建缓存承载 task 可变状态；events/token JSONL 文件（chattr +a + SHA-256 哈希链，按周分区）作不可篡改事件流；DuckDB CLI 子进程用于分析查询。sevolver 每日扫描 journal 维护 skill frontmatter 生命周期，超 2 周未命中则归档到 deprecated/。

**Tech Stack:** Go 1.25 (module `github.com/41490/ccclaw`), DuckDB 1.5 CLI (`~/.local/bin/duckdb`), `encoding/json`, `crypto/sha256`, `os.Rename` (atomic), `exec.Command` (DuckDB subprocess), `gopkg.in/yaml.v3` (已有).

**Reference Issues:** #32 (主设计), #31 (SQLite 锁冲突，Phase 1 解决).

---

## Chunk 1: Phase 0 — upgrade.sh Frontmatter Field Protection

**Scope:** upgrade.sh 在刷新 kb/skills/ 文件时，保留 sevolver 自动维护的 YAML frontmatter 字段。

**Files:**
- Modify: `src/dist/upgrade.sh`
- Modify: `src/dist/kb/skills/CLAUDE.md` — 在 managed section 中记录新字段规范
- Modify: `src/dist/kb/skills/L1/CLAUDE.md`
- Modify: `src/dist/kb/skills/L2/CLAUDE.md`

### Task 0.1: 理解 upgrade.sh 现有 managed-section 机制

- [ ] **Step 1: 阅读 upgrade.sh 中的 `merge_managed_markdown()` 函数**

  ```bash
  grep -n "merge_managed_markdown\|extract_marked_block\|ccclaw:managed\|ccclaw:user" \
    src/dist/upgrade.sh
  ```
  预期：找到 3 个函数定义和 HTML comment 标记模式。

- [ ] **Step 2: 确认当前 skill CLAUDE.md 文件结构**

  ```bash
  head -30 src/dist/kb/skills/CLAUDE.md
  ```
  预期：看到 `<!-- ccclaw:managed:start -->` 和 `<!-- ccclaw:user:start -->` 标记。

### Task 0.2: 新增 frontmatter 字段保留函数

**背景：** 现有 managed-section 机制处理 markdown body 段落；新需求是保留 YAML frontmatter 中的特定字段。

- [ ] **Step 1: 在 upgrade.sh 中新增 `preserve_skill_meta_fields()` 函数**

  在 upgrade.sh 的函数区域末尾（`merge_managed_markdown` 之后）添加：

  ```bash
  # preserve_skill_meta_fields OLD_FILE NEW_FILE
  # 从 OLD_FILE 的 YAML frontmatter 提取 sevolver 维护字段，注入到 NEW_FILE 的 frontmatter
  preserve_skill_meta_fields() {
      local old_file="$1"
      local new_file="$2"
      # 只处理有 frontmatter 的文件
      if ! head -1 "$old_file" | grep -q '^---'; then
          return 0
      fi
      # 提取 sevolver 字段（如有）
      local last_used use_count status gap_signals
      last_used=$(awk '/^---/{n++} n==1 && /^last_used:/{print $2}' "$old_file")
      use_count=$(awk '/^---/{n++} n==1 && /^use_count:/{print $2}' "$old_file")
      status=$(awk '/^---/{n++} n==1 && /^status:/{print $2}' "$old_file")
      # 无旧值则跳过
      if [ -z "$last_used" ] && [ -z "$use_count" ] && [ -z "$status" ]; then
          return 0
      fi
      # 向 new_file 注入（若 frontmatter 中无该字段则追加；已有则不覆盖）
      local tmp
      tmp=$(mktemp)
      awk -v lu="$last_used" -v uc="$use_count" -v st="$status" '
          BEGIN { in_fm=0; fm_done=0; injected=0 }
          /^---/ && !fm_done {
              if (in_fm) {
                  # 关闭 frontmatter：先注入缺失字段，再写 ---
                  if (lu != "" && !has_lu) print "last_used: " lu
                  if (uc != "" && !has_uc) print "use_count: " uc
                  if (st != "" && !has_st) print "status: " st
                  fm_done=1
              }
              in_fm=1; print; next
          }
          in_fm && !fm_done && /^last_used:/ { has_lu=1 }
          in_fm && !fm_done && /^use_count:/ { has_uc=1 }
          in_fm && !fm_done && /^status:/    { has_st=1 }
          { print }
      ' "$new_file" > "$tmp" && mv "$tmp" "$new_file"
  }
  ```

- [ ] **Step 2: 在 kb/skills/ upgrade 循环中调用此函数**

  找到 upgrade.sh 中遍历 kb/skills/ 的循环，在 `merge_managed_markdown` 调用之后添加：

  ```bash
  preserve_skill_meta_fields "$existing_file" "$target_file"
  ```

- [ ] **Step 3: 手工验证函数（不需要自动化测试）**

  创建临时文件验证：
  ```bash
  cat > /tmp/old_skill.md << 'EOF'
  ---
  name: test
  description: test skill
  keywords: [test]
  last_used: 2026-03-10
  use_count: 7
  status: active
  ---
  Content here.
  EOF

  cat > /tmp/new_skill.md << 'EOF'
  ---
  name: test
  description: updated description
  keywords: [test, updated]
  ---
  New content.
  EOF

  # 调用函数（source upgrade.sh 或手动 bash）
  bash -c 'source src/dist/upgrade.sh; preserve_skill_meta_fields /tmp/old_skill.md /tmp/new_skill.md'
  cat /tmp/new_skill.md
  ```
  预期：new_skill.md 的 frontmatter 包含 `last_used: 2026-03-10`、`use_count: 7`、`status: active`。

- [ ] **Step 4: Commit**

  ```bash
  git add src/dist/upgrade.sh
  git commit -m "feat(upgrade): preserve skill frontmatter sevolver fields across upgrades"
  ```

### Task 0.3: 更新 kb/skills/CLAUDE.md managed section 记录新字段规范

- [ ] **Step 1: 在 src/dist/kb/skills/CLAUDE.md 的 managed section 中追加字段文档**

  在 `<!-- ccclaw:managed:end -->` 之前，追加：

  ```markdown
  ## sevolver 自动维护字段

  以下字段由 `ccclaw-sevolver` 服务自动更新，**不得手动修改**：

  ```yaml
  last_used: YYYY-MM-DD      # 最近一次命中日期
  use_count: 0               # 累计命中次数
  status: active             # active | dormant | deprecated
  gap_signals: []            # 关联缺口信号 ID 列表
  ```

  - `status: dormant` — 超过 14 天未命中自动触发
  - `status: deprecated` — dormant 满 14 天后物理移至 `kb/skills/deprecated/`
  ```

- [ ] **Step 2: 同步 L1/CLAUDE.md 和 L2/CLAUDE.md**

  在两个文件的 managed section 中同样追加相同说明。

- [ ] **Step 3: Commit**

  ```bash
  git add src/dist/kb/skills/CLAUDE.md \
          src/dist/kb/skills/L1/CLAUDE.md \
          src/dist/kb/skills/L2/CLAUDE.md
  git commit -m "docs(kb): document sevolver auto-maintained frontmatter fields"
  ```

---

## Chunk 2: Phase 1 — JSONL+DuckDB 替代 SQLite state_db

**Scope:** 新建 JSONLStore（事件写入）、StateStore（task 状态缓存）、HashChain（链验证）、DuckDB 查询层；新增 archive timer；保持 `*storage.Store` 接口兼容，运行时无感切换。

**Files:**
- Create: `src/internal/adapters/storage/jsonl.go`
- Create: `src/internal/adapters/storage/jsonl_test.go`
- Create: `src/internal/adapters/storage/state_store.go`
- Create: `src/internal/adapters/storage/state_store_test.go`
- Create: `src/internal/adapters/storage/hashchain.go`
- Create: `src/internal/adapters/storage/hashchain_test.go`
- Create: `src/internal/adapters/query/duckdb.go`
- Create: `src/internal/adapters/query/duckdb_test.go`
- Create: `src/internal/adapters/storage/store.go` — 新 unified Store，替代 sqlite.go
- Modify: `src/internal/adapters/storage/sqlite.go` → rename to `sqlite_legacy.go` (保留迁移用)
- Modify: `src/internal/app/runtime.go` — 切换到新 Store
- Modify: `src/internal/config/config.go` — 新增 `VarDir` 派生字段
- Create: `src/dist/ops/systemd/ccclaw-archive.timer`
- Create: `src/dist/ops/systemd/ccclaw-archive.service`
- Create: `src/cmd/ccclaw/archive.go` — `ccclaw archive` 子命令

### Task 1.1: hashchain.go — SHA-256 周独立哈希链

- [ ] **Step 1: 写失败测试**

  `src/internal/adapters/storage/hashchain_test.go`:
  ```go
  package storage_test

  import (
      "testing"
      "github.com/41490/ccclaw/internal/adapters/storage"
  )

  func TestGenesisHash(t *testing.T) {
      h := storage.GenesisHash(2026, 11)
      if h != "GENESIS-W2026-11" {
          t.Fatalf("got %q, want GENESIS-W2026-11", h)
      }
  }

  func TestComputeHash_Deterministic(t *testing.T) {
      h1, err := storage.ComputeHash("task1", "CREATED", "2026-03-12T10:00:00Z", "{}", "GENESIS-W2026-11")
      if err != nil { t.Fatal(err) }
      h2, err := storage.ComputeHash("task1", "CREATED", "2026-03-12T10:00:00Z", "{}", "GENESIS-W2026-11")
      if err != nil { t.Fatal(err) }
      if h1 != h2 { t.Fatal("hash not deterministic") }
      if len(h1) != 64 { t.Fatalf("expected 64 hex chars, got %d", len(h1)) }
  }

  func TestVerifyChain_Valid(t *testing.T) {
      dir := t.TempDir()
      path := dir + "/events-2026-W11.jsonl"
      // write 3 records with valid chain
      records := []storage.EventRecord{
          {Seq: 1, TaskID: "t1", EventType: "CREATED", Ts: "2026-03-12T10:00:00Z", Detail: "{}"},
          {Seq: 2, TaskID: "t1", EventType: "RUNNING", Ts: "2026-03-12T10:01:00Z", Detail: "{}"},
          {Seq: 3, TaskID: "t1", EventType: "DONE",    Ts: "2026-03-12T10:05:00Z", Detail: "{}"},
      }
      prevHash := storage.GenesisHash(2026, 11)
      f, _ := os.Create(path)
      enc := json.NewEncoder(f)
      for i, r := range records {
          r.PrevHash = prevHash
          h, _ := storage.ComputeHash(r.TaskID, r.EventType, r.Ts, r.Detail, prevHash)
          r.Hash = h
          records[i] = r
          enc.Encode(r)
          prevHash = h
      }
      f.Close()
      if err := storage.VerifyChain(path); err != nil {
          t.Fatalf("valid chain failed verification: %v", err)
      }
  }

  func TestVerifyChain_Tampered(t *testing.T) {
      // ... write 3 records, tamper with middle record's detail, verify returns error
  }
  ```

  ```bash
  cd src && go test ./internal/adapters/storage/... -run TestGenesisHash -v
  ```
  预期: FAIL — `storage.GenesisHash` undefined.

- [ ] **Step 2: 实现 hashchain.go**

  `src/internal/adapters/storage/hashchain.go`:
  ```go
  package storage

  import (
      "bufio"
      "crypto/sha256"
      "encoding/json"
      "fmt"
      "os"
  )

  // GenesisHash returns the genesis prev_hash for a given ISO year/week.
  func GenesisHash(year, week int) string {
      return fmt.Sprintf("GENESIS-W%d-%02d", year, week)
  }

  // ComputeHash computes SHA-256 over canonical fields (no hash field).
  func ComputeHash(taskID, eventType, ts, detail, prevHash string) (string, error) {
      canonical := fmt.Sprintf("%s|%s|%s|%s|%s", taskID, eventType, ts, detail, prevHash)
      sum := sha256.Sum256([]byte(canonical))
      return fmt.Sprintf("%x", sum), nil
  }

  // ComputeTokenHash computes SHA-256 for a token record.
  func ComputeTokenHash(taskID, sessionID, ts string, inputTokens, outputTokens int, costUSD float64, prevHash string) (string, error) {
      canonical := fmt.Sprintf("%s|%s|%s|%d|%d|%.8f|%s",
          taskID, sessionID, ts, inputTokens, outputTokens, costUSD, prevHash)
      sum := sha256.Sum256([]byte(canonical))
      return fmt.Sprintf("%x", sum), nil
  }

  // VerifyChain reads a JSONL event file and verifies the SHA-256 chain.
  // Returns nil if chain is intact, error describing first broken link otherwise.
  func VerifyChain(path string) error {
      f, err := os.Open(path)
      if err != nil {
          return fmt.Errorf("open %s: %w", path, err)
      }
      defer f.Close()
      sc := bufio.NewScanner(f)
      sc.Buffer(make([]byte, 1<<20), 1<<20) // 1MB per line
      var prev string
      var seq int64
      for sc.Scan() {
          line := sc.Bytes()
          if len(line) == 0 {
              continue
          }
          var r EventRecord
          if err := json.Unmarshal(line, &r); err != nil {
              return fmt.Errorf("seq %d: parse error: %w", seq+1, err)
          }
          seq = r.Seq
          if prev == "" {
              prev = r.PrevHash // first record: accept genesis
          } else if r.PrevHash != prev {
              return fmt.Errorf("seq %d: prev_hash mismatch: got %q, want %q", seq, r.PrevHash, prev)
          }
          want, err := ComputeHash(r.TaskID, r.EventType, r.Ts, r.Detail, r.PrevHash)
          if err != nil {
              return err
          }
          if r.Hash != want {
              return fmt.Errorf("seq %d: hash mismatch (tampered?)", seq)
          }
          prev = r.Hash
      }
      return sc.Err()
  }
  ```

- [ ] **Step 3: 运行测试**

  ```bash
  cd src && go test ./internal/adapters/storage/... -run "TestGenesisHash|TestComputeHash|TestVerifyChain" -v
  ```
  预期: PASS (3 tests).

- [ ] **Step 4: Commit**

  ```bash
  git add src/internal/adapters/storage/hashchain.go \
          src/internal/adapters/storage/hashchain_test.go
  git commit -m "feat(storage): SHA-256 per-week hash chain for JSONL integrity verification"
  ```

### Task 1.2: jsonl.go — JSONL 事件/Token 写入

- [ ] **Step 1: 定义 EventRecord 和 TokenRecord 结构，写失败测试**

  `src/internal/adapters/storage/jsonl_test.go`:
  ```go
  package storage_test

  import (
      "bufio"
      "encoding/json"
      "os"
      "path/filepath"
      "testing"
      "time"
      "github.com/41490/ccclaw/internal/adapters/storage"
  )

  func TestJSONLStore_AppendEvent_WritesHashedRecord(t *testing.T) {
      dir := t.TempDir()
      store, err := storage.OpenJSONLStore(dir)
      if err != nil { t.Fatal(err) }
      defer store.Close()

      now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)
      err = store.AppendEventAt("task1", "CREATED", "{}", now)
      if err != nil { t.Fatal(err) }

      _, week := now.ISOWeek()
      fname := filepath.Join(dir, fmt.Sprintf("events-%d-W%02d.jsonl", now.Year(), week))
      f, _ := os.Open(fname)
      defer f.Close()
      sc := bufio.NewScanner(f)
      sc.Scan()
      var rec storage.EventRecord
      if err := json.Unmarshal(sc.Bytes(), &rec); err != nil { t.Fatal(err) }
      if rec.Seq != 1 { t.Errorf("seq: got %d, want 1", rec.Seq) }
      if rec.TaskID != "task1" { t.Errorf("task_id: got %q", rec.TaskID) }
      if rec.EventType != "CREATED" { t.Error("event_type mismatch") }
      if rec.Hash == "" { t.Error("hash should not be empty") }
      if rec.PrevHash == "" { t.Error("prev_hash should not be empty") }
  }

  func TestJSONLStore_AppendEvent_ChainContinues(t *testing.T) {
      dir := t.TempDir()
      store, _ := storage.OpenJSONLStore(dir)
      defer store.Close()
      now := time.Now()
      store.AppendEventAt("t1", "CREATED", "{}", now)
      store.AppendEventAt("t1", "RUNNING", "{}", now.Add(time.Minute))

      // 验证链完整
      _, week := now.ISOWeek()
      fname := filepath.Join(dir, fmt.Sprintf("events-%d-W%02d.jsonl", now.Year(), week))
      if err := storage.VerifyChain(fname); err != nil {
          t.Fatalf("chain broken: %v", err)
      }
  }

  func TestJSONLStore_AppendToken(t *testing.T) {
      dir := t.TempDir()
      store, _ := storage.OpenJSONLStore(dir)
      defer store.Close()
      rec := storage.TokenRecord{
          TaskID: "t1", SessionID: "s1",
          InputTokens: 1000, OutputTokens: 500, CostUSD: 0.015,
      }
      if err := store.AppendToken(rec); err != nil { t.Fatal(err) }
  }
  ```

  ```bash
  cd src && go test ./internal/adapters/storage/... -run TestJSONLStore -v
  ```
  预期: FAIL — `storage.OpenJSONLStore` undefined.

- [ ] **Step 2: 实现 jsonl.go**

  `src/internal/adapters/storage/jsonl.go`:
  ```go
  package storage

  import (
      "encoding/json"
      "fmt"
      "os"
      "path/filepath"
      "sync"
      "time"
  )

  // EventRecord is one line in events-YYYY-WNN.jsonl.
  type EventRecord struct {
      Seq        int64  `json:"seq"`
      TaskID     string `json:"task_id"`
      EventType  string `json:"event_type"`
      TargetRepo string `json:"target_repo,omitempty"`
      Detail     string `json:"detail"`
      Ts         string `json:"ts"`
      PrevHash   string `json:"prev_hash"`
      Hash       string `json:"hash"`
  }

  // TokenRecord is one line in token-YYYY-WNN.jsonl.
  type TokenRecord struct {
      Seq          int64   `json:"seq"`
      TaskID       string  `json:"task_id"`
      SessionID    string  `json:"session_id"`
      InputTokens  int     `json:"input_tokens"`
      OutputTokens int     `json:"output_tokens"`
      CacheCreate  int     `json:"cache_create"`
      CacheRead    int     `json:"cache_read"`
      CostUSD      float64 `json:"cost_usd"`
      DurationMS   int64   `json:"duration_ms"`
      RTKEnabled   bool    `json:"rtk_enabled"`
      PromptFile   string  `json:"prompt_file,omitempty"`
      Ts           string  `json:"ts"`
      PrevHash     string  `json:"prev_hash"`
      Hash         string  `json:"hash"`
  }

  // JSONLStore appends events and token records to per-week JSONL files.
  type JSONLStore struct {
      dir         string
      mu          sync.Mutex
      eventSeq    int64
      tokenSeq    int64
      eventHash   string // last event hash
      tokenHash   string // last token hash
  }

  // OpenJSONLStore opens (or creates) the JSONL store at varDir.
  // It scans existing files to restore sequence counters and last hashes.
  func OpenJSONLStore(varDir string) (*JSONLStore, error) {
      s := &JSONLStore{dir: varDir}
      // restore event state
      if h, seq, err := s.lastRecordState("events"); err == nil {
          s.eventHash = h
          s.eventSeq = seq
      }
      // restore token state
      if h, seq, err := s.lastRecordState("token"); err == nil {
          s.tokenHash = h
          s.tokenSeq = seq
      }
      return s, nil
  }

  func (s *JSONLStore) Close() error { return nil }

  // AppendEvent writes a new event to the current week's events JSONL file.
  func (s *JSONLStore) AppendEvent(taskID, eventType, detail string) error {
      return s.AppendEventAt(taskID, eventType, detail, time.Now().UTC())
  }

  // AppendEventAt writes an event with explicit timestamp (for testing).
  func (s *JSONLStore) AppendEventAt(taskID, eventType, detail string, ts time.Time) error {
      s.mu.Lock()
      defer s.mu.Unlock()
      year, week := ts.ISOWeek()
      prevHash := s.eventHash
      if prevHash == "" {
          prevHash = GenesisHash(year, week)
      }
      tsStr := ts.Format(time.RFC3339)
      hash, err := ComputeHash(taskID, eventType, tsStr, detail, prevHash)
      if err != nil {
          return err
      }
      s.eventSeq++
      rec := EventRecord{
          Seq: s.eventSeq, TaskID: taskID, EventType: eventType,
          Detail: detail, Ts: tsStr, PrevHash: prevHash, Hash: hash,
      }
      if err := s.appendLine(s.eventFile(year, week), rec); err != nil {
          s.eventSeq-- // rollback
          return err
      }
      s.eventHash = hash
      return nil
  }

  // AppendToken writes a token usage record.
  func (s *JSONLStore) AppendToken(rec TokenRecord) error {
      s.mu.Lock()
      defer s.mu.Unlock()
      now := time.Now().UTC()
      year, week := now.ISOWeek()
      rec.Ts = now.Format(time.RFC3339)
      prevHash := s.tokenHash
      if prevHash == "" {
          prevHash = GenesisHash(year, week)
      }
      hash, err := ComputeTokenHash(rec.TaskID, rec.SessionID, rec.Ts,
          rec.InputTokens, rec.OutputTokens, rec.CostUSD, prevHash)
      if err != nil {
          return err
      }
      s.tokenSeq++
      rec.Seq = s.tokenSeq
      rec.PrevHash = prevHash
      rec.Hash = hash
      if err := s.appendLine(s.tokenFile(year, week), rec); err != nil {
          s.tokenSeq--
          return err
      }
      s.tokenHash = hash
      return nil
  }

  func (s *JSONLStore) eventFile(year, week int) string {
      return filepath.Join(s.dir, fmt.Sprintf("events-%d-W%02d.jsonl", year, week))
  }

  func (s *JSONLStore) tokenFile(year, week int) string {
      return filepath.Join(s.dir, fmt.Sprintf("token-%d-W%02d.jsonl", year, week))
  }

  // appendLine encodes v as JSON and appends it atomically to path.
  func (s *JSONLStore) appendLine(path string, v any) error {
      b, err := json.Marshal(v)
      if err != nil {
          return err
      }
      b = append(b, '\n')
      f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
      if err != nil {
          return fmt.Errorf("open %s: %w", path, err)
      }
      defer f.Close()
      _, err = f.Write(b)
      return err
  }

  // lastRecordState reads the last record from the most recent JSONL file
  // matching prefix (events or token) and returns (lastHash, lastSeq, error).
  func (s *JSONLStore) lastRecordState(prefix string) (string, int64, error) {
      // find most recent file
      pattern := filepath.Join(s.dir, prefix+"-*.jsonl")
      matches, err := filepath.Glob(pattern)
      if err != nil || len(matches) == 0 {
          return "", 0, fmt.Errorf("no files")
      }
      // sort and take last
      latest := matches[len(matches)-1]
      f, err := os.Open(latest)
      if err != nil {
          return "", 0, err
      }
      defer f.Close()
      // read last non-empty line
      var lastLine []byte
      sc := bufio.NewScanner(f)
      sc.Buffer(make([]byte, 1<<20), 1<<20)
      for sc.Scan() {
          if len(sc.Bytes()) > 0 {
              lastLine = make([]byte, len(sc.Bytes()))
              copy(lastLine, sc.Bytes())
          }
      }
      if len(lastLine) == 0 {
          return "", 0, fmt.Errorf("empty file")
      }
      if prefix == "events" {
          var r EventRecord
          if err := json.Unmarshal(lastLine, &r); err != nil {
              return "", 0, err
          }
          return r.Hash, r.Seq, nil
      }
      var r TokenRecord
      if err := json.Unmarshal(lastLine, &r); err != nil {
          return "", 0, err
      }
      return r.Hash, r.Seq, nil
  }
  ```

  需要在 jsonl.go 顶部加 `"bufio"` import.

- [ ] **Step 3: 运行测试**

  ```bash
  cd src && go test ./internal/adapters/storage/... -run TestJSONLStore -v
  ```
  预期: PASS (3 tests).

- [ ] **Step 4: Commit**

  ```bash
  git add src/internal/adapters/storage/jsonl.go \
          src/internal/adapters/storage/jsonl_test.go
  git commit -m "feat(storage): JSONL event and token write layer with hash chain"
  ```

### Task 1.3: state_store.go — task 状态 state.json

- [ ] **Step 1: 写失败测试**

  `src/internal/adapters/storage/state_store_test.go`:
  ```go
  package storage_test

  import (
      "path/filepath"
      "testing"
      "time"
      "github.com/41490/ccclaw/internal/adapters/storage"
      "github.com/41490/ccclaw/internal/core"
  )

  func makeTask(id, ikey string, state core.State) *core.Task {
      return &core.Task{
          TaskID: id, IdempotencyKey: ikey,
          ControlRepo: "ctrl", IssueRepo: "repo", TargetRepo: "target",
          IssueNumber: 1, State: state,
          CreatedAt: time.Now(), UpdatedAt: time.Now(),
      }
  }

  func TestStateStore_UpsertAndGet(t *testing.T) {
      path := filepath.Join(t.TempDir(), "state.json")
      ss, err := storage.OpenStateStore(path)
      if err != nil { t.Fatal(err) }

      task := makeTask("r#1#body", "r#1#body", core.StateNew)
      if err := ss.Upsert(task); err != nil { t.Fatal(err) }

      got, err := ss.GetByIdempotency("r#1#body")
      if err != nil { t.Fatal(err) }
      if got.State != core.StateNew { t.Errorf("state: got %q", got.State) }
  }

  func TestStateStore_ListRunnable(t *testing.T) {
      path := filepath.Join(t.TempDir(), "state.json")
      ss, _ := storage.OpenStateStore(path)
      ss.Upsert(makeTask("t1", "r#1#body", core.StateNew))
      ss.Upsert(makeTask("t2", "r#2#body", core.StateRunning))
      ss.Upsert(makeTask("t3", "r#3#body", core.StateFailed))

      runnable, err := ss.ListRunnable(10)
      if err != nil { t.Fatal(err) }
      if len(runnable) != 2 { t.Errorf("want 2, got %d", len(runnable)) }
      for _, r := range runnable {
          if r.State != core.StateNew && r.State != core.StateFailed {
              t.Errorf("unexpected state: %q", r.State)
          }
      }
  }

  func TestStateStore_PersistsAcrossReopen(t *testing.T) {
      path := filepath.Join(t.TempDir(), "state.json")
      ss, _ := storage.OpenStateStore(path)
      ss.Upsert(makeTask("t1", "r#1#body", core.StateRunning))

      // reopen
      ss2, err := storage.OpenStateStore(path)
      if err != nil { t.Fatal(err) }
      got, err := ss2.GetByIdempotency("r#1#body")
      if err != nil { t.Fatal(err) }
      if got.State != core.StateRunning { t.Error("state not persisted") }
  }
  ```

  ```bash
  cd src && go test ./internal/adapters/storage/... -run TestStateStore -v
  ```
  预期: FAIL.

- [ ] **Step 2: 实现 state_store.go**

  `src/internal/adapters/storage/state_store.go`:
  ```go
  package storage

  import (
      "encoding/json"
      "fmt"
      "os"
      "sort"
      "sync"
      "time"
      "github.com/41490/ccclaw/internal/core"
  )

  // StateStore persists task state in a single JSON file.
  // It is the mutable cache; events JSONL is the source of truth.
  type StateStore struct {
      path string
      mu   sync.RWMutex
      data map[string]*core.Task // keyed by idempotency_key
  }

  // OpenStateStore loads (or creates) state.json at path.
  func OpenStateStore(path string) (*StateStore, error) {
      ss := &StateStore{path: path, data: make(map[string]*core.Task)}
      b, err := os.ReadFile(path)
      if os.IsNotExist(err) {
          return ss, nil
      }
      if err != nil {
          return nil, fmt.Errorf("read state.json: %w", err)
      }
      if err := json.Unmarshal(b, &ss.data); err != nil {
          return nil, fmt.Errorf("parse state.json: %w", err)
      }
      return ss, nil
  }

  func (ss *StateStore) Upsert(task *core.Task) error {
      ss.mu.Lock()
      defer ss.mu.Unlock()
      task.UpdatedAt = time.Now().UTC()
      ss.data[task.IdempotencyKey] = task
      return ss.flush()
  }

  func (ss *StateStore) GetByIdempotency(key string) (*core.Task, error) {
      ss.mu.RLock()
      defer ss.mu.RUnlock()
      t, ok := ss.data[key]
      if !ok {
          return nil, fmt.Errorf("task not found: %q", key)
      }
      cp := *t
      return &cp, nil
  }

  func (ss *StateStore) ListTasks() ([]*core.Task, error) {
      ss.mu.RLock()
      defer ss.mu.RUnlock()
      out := make([]*core.Task, 0, len(ss.data))
      for _, t := range ss.data {
          cp := *t
          out = append(out, &cp)
      }
      sort.Slice(out, func(i, j int) bool {
          return out[i].UpdatedAt.After(out[j].UpdatedAt)
      })
      return out, nil
  }

  func (ss *StateStore) ListRunnable(limit int) ([]*core.Task, error) {
      ss.mu.RLock()
      defer ss.mu.RUnlock()
      var out []*core.Task
      for _, t := range ss.data {
          if t.State == core.StateNew || t.State == core.StateFailed {
              cp := *t
              out = append(out, &cp)
          }
      }
      sort.Slice(out, func(i, j int) bool {
          return out[i].CreatedAt.Before(out[j].CreatedAt)
      })
      if limit > 0 && len(out) > limit {
          out = out[:limit]
      }
      return out, nil
  }

  func (ss *StateStore) ListRunning() ([]*core.Task, error) {
      ss.mu.RLock()
      defer ss.mu.RUnlock()
      var out []*core.Task
      for _, t := range ss.data {
          if t.State == core.StateRunning {
              cp := *t
              out = append(out, &cp)
          }
      }
      return out, nil
  }

  // flush atomically rewrites state.json.
  func (ss *StateStore) flush() error {
      b, err := json.MarshalIndent(ss.data, "", "  ")
      if err != nil {
          return err
      }
      tmp := ss.path + ".tmp"
      if err := os.WriteFile(tmp, b, 0644); err != nil {
          return err
      }
      return os.Rename(tmp, ss.path)
  }
  ```

- [ ] **Step 3: 运行测试**

  ```bash
  cd src && go test ./internal/adapters/storage/... -run TestStateStore -v
  ```
  预期: PASS (3 tests).

- [ ] **Step 4: Commit**

  ```bash
  git add src/internal/adapters/storage/state_store.go \
          src/internal/adapters/storage/state_store_test.go
  git commit -m "feat(storage): state.json task state cache with atomic writes"
  ```

### Task 1.4: duckdb.go — DuckDB CLI 查询层

- [ ] **Step 1: 写失败测试**

  `src/internal/adapters/query/duckdb_test.go`:
  ```go
  package query_test

  import (
      "os/exec"
      "path/filepath"
      "encoding/json"
      "os"
      "testing"
      "github.com/41490/ccclaw/internal/adapters/query"
      "github.com/41490/ccclaw/internal/adapters/storage"
  )

  func skipIfNoDuckDB(t *testing.T) {
      t.Helper()
      if _, err := exec.LookPath("duckdb"); err != nil {
          t.Skip("duckdb not in PATH")
      }
  }

  func TestDuckDB_QueryTokenStats(t *testing.T) {
      skipIfNoDuckDB(t)
      dir := t.TempDir()
      // write minimal token JSONL
      rec := storage.TokenRecord{Seq:1, TaskID:"t1", SessionID:"s1",
          InputTokens:1000, OutputTokens:500, CostUSD:0.015,
          Ts:"2026-03-12T10:00:00Z", PrevHash:"GENESIS-W2026-11", Hash:"abc"}
      b, _ := json.Marshal(rec)
      os.WriteFile(filepath.Join(dir, "token-2026-W11.jsonl"), append(b, '\n'), 0644)

      db := query.New(dir)
      stats, err := db.TokenStats(nil, nil)
      if err != nil { t.Fatal(err) }
      if stats.TotalInputTokens != 1000 { t.Errorf("got %d", stats.TotalInputTokens) }
      if stats.TotalCostUSD < 0.01 { t.Errorf("cost: %f", stats.TotalCostUSD) }
  }
  ```

  ```bash
  cd src && go test ./internal/adapters/query/... -run TestDuckDB -v
  ```
  预期: FAIL (package not found).

- [ ] **Step 2: 实现 duckdb.go**

  `src/internal/adapters/query/duckdb.go`:
  ```go
  package query

  import (
      "bytes"
      "encoding/json"
      "fmt"
      "os/exec"
      "path/filepath"
      "strings"
      "time"
      "github.com/41490/ccclaw/internal/adapters/storage"
  )

  // DuckDB wraps the DuckDB CLI binary for analytics queries.
  type DuckDB struct {
      bin    string
      varDir string
  }

  // New finds duckdb binary in PATH and returns a DuckDB query client.
  func New(varDir string) *DuckDB {
      bin, _ := exec.LookPath("duckdb")
      if bin == "" {
          bin = "duckdb" // will fail at query time with clear error
      }
      return &DuckDB{bin: bin, varDir: varDir}
  }

  // queryJSON runs a DuckDB SQL query and unmarshals the JSON result array.
  func (d *DuckDB) queryJSON(sql string) ([]map[string]any, error) {
      // DuckDB CLI: duckdb :memory: -json -c "<sql>"
      cmd := exec.Command(d.bin, ":memory:", "-json", "-c", sql)
      var out, errBuf bytes.Buffer
      cmd.Stdout = &out
      cmd.Stderr = &errBuf
      if err := cmd.Run(); err != nil {
          return nil, fmt.Errorf("duckdb: %w\n%s", err, errBuf.String())
      }
      if out.Len() == 0 {
          return nil, nil
      }
      var rows []map[string]any
      if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
          return nil, fmt.Errorf("parse duckdb output: %w", err)
      }
      return rows, nil
  }

  func (d *DuckDB) glob(prefix string) string {
      return filepath.Join(d.varDir, prefix+"-*.jsonl") + "','" +
          filepath.Join(d.varDir, "archive", prefix+"-*.parquet")
  }

  // TokenStats returns aggregated token stats, optionally filtered by time range.
  func (d *DuckDB) TokenStats(start, end *time.Time) (*storage.TokenStatsSummary, error) {
      where := d.timeWhere("ts", start, end)
      sql := fmt.Sprintf(`
          SELECT
            COUNT(DISTINCT session_id) AS sessions,
            COUNT(DISTINCT task_id)   AS tasks,
            COALESCE(SUM(input_tokens),0)  AS input_tokens,
            COALESCE(SUM(output_tokens),0) AS output_tokens,
            COALESCE(SUM(cache_create),0)  AS cache_create,
            COALESCE(SUM(cache_read),0)    AS cache_read,
            COALESCE(SUM(cost_usd),0)      AS cost_usd,
            MIN(ts) AS first_ts, MAX(ts) AS last_ts
          FROM read_json_auto('%s')
          %s`,
          filepath.Join(d.varDir, "token-*.jsonl"), where)
      rows, err := d.queryJSON(sql)
      if err != nil || len(rows) == 0 {
          return &storage.TokenStatsSummary{}, err
      }
      r := rows[0]
      return &storage.TokenStatsSummary{
          TotalSessions:       intVal(r, "sessions"),
          TotalRuns:           intVal(r, "tasks"),
          TotalInputTokens:    intVal(r, "input_tokens"),
          TotalOutputTokens:   intVal(r, "output_tokens"),
          TotalCacheCreate:    intVal(r, "cache_create"),
          TotalCacheRead:      intVal(r, "cache_read"),
          TotalCostUSD:        floatVal(r, "cost_usd"),
      }, nil
  }

  // JournalDaySummary returns daily task event counts for a given day.
  func (d *DuckDB) JournalDaySummary(day time.Time) (*storage.JournalDaySummary, error) {
      start := day.Truncate(24 * time.Hour)
      end := start.Add(24 * time.Hour)
      s, e := start, end
      where := d.timeWhere("ts", &s, &e)
      sql := fmt.Sprintf(`
          SELECT
            COUNT(DISTINCT task_id) AS tasks_touched,
            COUNT(CASE WHEN event_type='CREATED' THEN 1 END) AS started,
            COUNT(CASE WHEN event_type='DONE'    THEN 1 END) AS done,
            COUNT(CASE WHEN event_type='FAILED'  THEN 1 END) AS failed,
            COUNT(CASE WHEN event_type='DEAD'    THEN 1 END) AS dead
          FROM read_json_auto('%s') %s`,
          filepath.Join(d.varDir, "events-*.jsonl"), where)
      rows, err := d.queryJSON(sql)
      if err != nil || len(rows) == 0 {
          return &storage.JournalDaySummary{Day: day}, err
      }
      r := rows[0]
      return &storage.JournalDaySummary{
          Day:          day,
          TasksTouched: intVal(r, "tasks_touched"),
          TasksStarted: intVal(r, "started"),
          TasksDone:    intVal(r, "done"),
          TasksFailed:  intVal(r, "failed"),
          TasksDead:    intVal(r, "dead"),
      }, nil
  }

  func (d *DuckDB) timeWhere(col string, start, end *time.Time) string {
      var parts []string
      if start != nil {
          parts = append(parts, fmt.Sprintf("%s >= '%s'", col, start.Format(time.RFC3339)))
      }
      if end != nil {
          parts = append(parts, fmt.Sprintf("%s < '%s'", col, end.Format(time.RFC3339)))
      }
      if len(parts) == 0 {
          return ""
      }
      return "WHERE " + strings.Join(parts, " AND ")
  }

  func intVal(r map[string]any, k string) int {
      if v, ok := r[k]; ok {
          switch x := v.(type) {
          case float64: return int(x)
          case int:     return x
          }
      }
      return 0
  }
  func floatVal(r map[string]any, k string) float64 {
      if v, ok := r[k]; ok {
          if x, ok := v.(float64); ok { return x }
      }
      return 0
  }
  ```

  **注意:** `storage.TokenStatsSummary` 和 `storage.JournalDaySummary` 这些类型目前在 sqlite.go 中定义。需要将它们移到独立的 `types.go` 文件，使 query 包可以引用。

- [ ] **Step 3: 将共享类型移到 storage/types.go**

  新建 `src/internal/adapters/storage/types.go`，将 `TokenStatsSummary`、`JournalDaySummary`、`JournalTaskSummary`、`EventRecord`（旧版，用于兼容）、`TaskTokenStat`、`RTKComparison`、`DailyTokenStat`、`DailyRTKComparison` 从 `sqlite.go` 剪切到 types.go（不改变 package）。

- [ ] **Step 4: 运行测试（duckdb 路径需在 PATH 中）**

  ```bash
  export PATH="$HOME/.local/bin:$PATH"
  cd src && go test ./internal/adapters/query/... -run TestDuckDB -v
  ```
  预期: PASS (1 test) 或 SKIP (若 duckdb 不在 PATH).

- [ ] **Step 5: Commit**

  ```bash
  git add src/internal/adapters/storage/types.go \
          src/internal/adapters/query/duckdb.go \
          src/internal/adapters/query/duckdb_test.go
  git commit -m "feat(query): DuckDB CLI subprocess analytics layer"
  ```

### Task 1.5: 新 unified Store — 替换 sqlite.go

- [ ] **Step 1: 重命名 sqlite.go 为 sqlite_legacy.go（保留迁移用）**

  ```bash
  cd src
  git mv internal/adapters/storage/sqlite.go internal/adapters/storage/sqlite_legacy.go
  git mv internal/adapters/storage/sqlite_test.go internal/adapters/storage/sqlite_legacy_test.go
  # 在 sqlite_legacy.go 顶部加注释: // Deprecated: use store.go (JSONL+DuckDB)
  ```

- [ ] **Step 2: 新建 store.go — unified Store**

  `src/internal/adapters/storage/store.go`:
  ```go
  package storage

  import (
      "time"
      "github.com/41490/ccclaw/internal/adapters/query"
      "github.com/41490/ccclaw/internal/core"
  )

  // Store is the unified storage facade. It replaces the SQLite-backed store.
  // Writes go to JSONLStore (events/tokens) and StateStore (task state).
  // Analytics reads go through DuckDB.
  type Store struct {
      state  *StateStore
      events *JSONLStore
      db     *query.DuckDB
  }

  // Open opens (or creates) the storage layer at varDir.
  // varDir is typically ~/.ccclaw/var/.
  func Open(varDir string) (*Store, error) {
      st, err := OpenStateStore(varDir + "/state.json")
      if err != nil { return nil, err }
      ev, err := OpenJSONLStore(varDir)
      if err != nil { return nil, err }
      return &Store{state: st, events: ev, db: query.New(varDir)}, nil
  }

  func (s *Store) Close() error { return s.events.Close() }
  func (s *Store) Ping() error  { return nil }

  // Task CRUD — delegate to StateStore
  func (s *Store) GetByIdempotency(key string) (*core.Task, error) { return s.state.GetByIdempotency(key) }
  func (s *Store) UpsertTask(task *core.Task) error                { return s.state.Upsert(task) }
  func (s *Store) ListTasks() ([]*core.Task, error)                { return s.state.ListTasks() }
  func (s *Store) ListRunnable(limit int) ([]*core.Task, error)    { return s.state.ListRunnable(limit) }
  func (s *Store) ListRunning() ([]*core.Task, error)              { return s.state.ListRunning() }

  // Event writes — delegate to JSONLStore
  func (s *Store) AppendEvent(taskID, eventType, detail string) error {
      return s.events.AppendEvent(taskID, eventType, detail)
  }
  func (s *Store) AppendEventAt(taskID, eventType, detail string, ts time.Time) error {
      return s.events.AppendEventAt(taskID, eventType, detail, ts)
  }
  func (s *Store) RecordTokenUsage(r TokenUsageRecord) error {
      return s.events.AppendToken(TokenRecord{
          TaskID: r.TaskID, SessionID: r.SessionID,
          InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
          CacheCreate: r.CacheCreate, CacheRead: r.CacheRead,
          CostUSD: r.CostUSD, DurationMS: r.DurationMS,
          RTKEnabled: r.RTKEnabled, PromptFile: r.PromptFile,
      })
  }

  // Analytics reads — delegate to DuckDB
  func (s *Store) TokenStats() (*TokenStatsSummary, error) {
      return s.db.TokenStats(nil, nil)
  }
  func (s *Store) TokenStatsBetween(start, end time.Time) (*TokenStatsSummary, error) {
      return s.db.TokenStats(&start, &end)
  }
  func (s *Store) JournalDaySummary(day time.Time) (*JournalDaySummary, error) {
      return s.db.JournalDaySummary(day)
  }
  // ... 其余 analytics 方法同样 delegate 到 db.*
  ```

- [ ] **Step 3: 修改 runtime.go — 切换 storage.Open 路径**

  在 `NewRuntime()` 中，将:
  ```go
  store, err := storage.Open(cfg.Paths.StateDB)
  ```
  改为:
  ```go
  store, err := storage.Open(filepath.Dir(cfg.Paths.StateDB)) // varDir
  ```
  `StateDB` 之前指向 `~/.ccclaw/var/state.db`，现在 varDir = `~/.ccclaw/var/`。

- [ ] **Step 4: 全量编译确认无错误**

  ```bash
  cd src && go build ./...
  ```
  预期: 0 errors.

- [ ] **Step 5: 运行现有测试套件**

  ```bash
  cd src && go test ./... 2>&1 | tail -30
  ```
  预期: 无 FAIL（可能有部分 sqlite_legacy_test.go 因路径变化需调整）.

- [ ] **Step 6: Commit**

  ```bash
  git add src/internal/adapters/storage/ src/internal/app/runtime.go
  git commit -m "feat(storage): replace SQLite with JSONL+DuckDB unified store"
  ```

### Task 1.6: ccclaw archive 子命令 + systemd 定时器

- [ ] **Step 1: 新增 archive.go 子命令**

  `src/cmd/ccclaw/archive.go`:
  ```go
  package main

  import (
      "fmt"
      "os"
      "path/filepath"
      "time"
      "github.com/spf13/cobra"
      "github.com/41490/ccclaw/internal/adapters/storage"
      "os/exec"
  )

  var archiveCmd = &cobra.Command{
      Use:   "archive",
      Short: "归档上周 JSONL 事件到 Parquet，验证哈希链完整性",
      RunE:  runArchive,
  }

  func init() { rootCmd.AddCommand(archiveCmd) }

  func runArchive(cmd *cobra.Command, _ []string) error {
      cfg, err := loadConfig(cmd)
      if err != nil { return err }
      varDir := filepath.Dir(cfg.Paths.StateDB)
      year, week := time.Now().AddDate(0, 0, -7).ISOWeek()
      eventFile := filepath.Join(varDir, fmt.Sprintf("events-%d-W%02d.jsonl", year, week))
      tokenFile := filepath.Join(varDir, fmt.Sprintf("token-%d-W%02d.jsonl", year, week))

      // 1. Verify hash chains
      for _, f := range []string{eventFile, tokenFile} {
          if _, err := os.Stat(f); os.IsNotExist(err) { continue }
          fmt.Fprintf(os.Stdout, "verifying %s...\n", filepath.Base(f))
          if err := storage.VerifyChain(f); err != nil {
              return fmt.Errorf("chain verification failed for %s: %w", f, err)
          }
          fmt.Println("  ok")
      }

      // 2. Convert JSONL → Parquet via DuckDB CLI
      archDir := filepath.Join(varDir, "archive")
      os.MkdirAll(archDir, 0755)
      duckdb, _ := exec.LookPath("duckdb")
      for _, f := range []string{eventFile, tokenFile} {
          if _, err := os.Stat(f); os.IsNotExist(err) { continue }
          base := filepath.Base(f)
          parquet := filepath.Join(archDir, strings.TrimSuffix(base, ".jsonl")+".parquet")
          sql := fmt.Sprintf(
              "COPY (SELECT * FROM read_json_auto('%s')) TO '%s' (FORMAT parquet, COMPRESSION zstd)",
              f, parquet)
          out, err := exec.Command(duckdb, ":memory:", "-c", sql).CombinedOutput()
          if err != nil {
              return fmt.Errorf("duckdb archive %s: %w\n%s", base, err, out)
          }
          fmt.Printf("  archived %s → %s\n", base, filepath.Base(parquet))
      }

      fmt.Println("archive complete")
      return nil
  }
  ```

- [ ] **Step 2: 新增 systemd 定时器文件**

  `src/dist/ops/systemd/ccclaw-archive.service`:
  ```ini
  [Unit]
  Description=CCClaw Weekly Archive (JSONL→Parquet + chain verify)
  After=network.target

  [Service]
  Type=oneshot
  WorkingDirectory=%h/.ccclaw
  ExecStart=%h/.ccclaw/bin/ccclaw archive --config %h/.ccclaw/ops/config/config.toml --env-file %h/.ccclaw/ops/config/.env
  StandardOutput=journal
  StandardError=journal
  ```

  `src/dist/ops/systemd/ccclaw-archive.timer`:
  ```ini
  [Unit]
  Description=CCClaw Weekly Archive Timer
  Requires=ccclaw-archive.service

  [Timer]
  OnCalendar=Mon *-*-* 02:00:00 Asia/Shanghai
  Persistent=true

  [Install]
  WantedBy=timers.target
  ```

- [ ] **Step 3: 在 install.sh 中注册 archive timer**

  在 install.sh 的 systemd timer 安装循环中，添加 `ccclaw-archive` 到 timer 列表。

- [ ] **Step 4: 编译验证**

  ```bash
  cd src && go build ./cmd/ccclaw/... && echo OK
  ```

- [ ] **Step 5: Commit**

  ```bash
  git add src/cmd/ccclaw/archive.go \
          src/dist/ops/systemd/ccclaw-archive.service \
          src/dist/ops/systemd/ccclaw-archive.timer
  git commit -m "feat(archive): weekly JSONL→Parquet archive command and systemd timer"
  ```

---

## Chunk 3: Phase 2 — sevolver 自我成长服务

**Scope:** 每日 22:00 扫描 journal，维护 skill frontmatter 生命周期（dormant → deprecated → 归档），记录缺口信号，生成日报。

**Files:**
- Create: `src/internal/sevolver/scanner.go` + `scanner_test.go`
- Create: `src/internal/sevolver/skill_updater.go` + `skill_updater_test.go`
- Create: `src/internal/sevolver/gap_recorder.go` + `gap_recorder_test.go`
- Create: `src/internal/sevolver/reporter.go`
- Create: `src/internal/sevolver/sevolver.go` — 主入口
- Create: `src/cmd/ccclaw/sevolver.go` — `ccclaw sevolver` 子命令
- Create: `src/dist/ops/systemd/ccclaw-sevolver.timer`
- Create: `src/dist/ops/systemd/ccclaw-sevolver.service`
- Create: `src/dist/kb/assay/signal-rules.md`
- Create: `src/dist/kb/assay/gap-signals.md` (模板)
- Create: `src/dist/kb/skills/deprecated/.gitkeep`
- Modify: `src/internal/memory/index.go` — 排除 deprecated/ 目录

### Task 2.1: 排除 deprecated/ 目录

- [ ] **Step 1: 写失败测试**

  在 `src/internal/memory/index_test.go` 中添加：
  ```go
  func TestBuild_ExcludesDeprecated(t *testing.T) {
      dir := t.TempDir()
      // create normal skill
      os.MkdirAll(dir+"/skills/L1", 0755)
      os.WriteFile(dir+"/skills/L1/foo.md", []byte("---\nname: foo\nkeywords: [foo]\n---\n"), 0644)
      // create deprecated skill (should be excluded)
      os.MkdirAll(dir+"/skills/deprecated", 0755)
      os.WriteFile(dir+"/skills/deprecated/old.md", []byte("---\nname: old\nkeywords: [old]\n---\n"), 0644)

      idx, err := memory.Build(dir)
      if err != nil { t.Fatal(err) }
      docs := idx.Match([]string{"old"}, 10)
      if len(docs) > 0 {
          t.Error("deprecated skills should not appear in index")
      }
  }
  ```

  ```bash
  cd src && go test ./internal/memory/... -run TestBuild_ExcludesDeprecated -v
  ```
  预期: FAIL.

- [ ] **Step 2: 修改 index.go 的 Build() 函数**

  在 `filepath.WalkDir` 的回调中，添加：
  ```go
  if d.IsDir() && d.Name() == "deprecated" {
      return filepath.SkipDir
  }
  ```

- [ ] **Step 3: 运行测试**

  ```bash
  cd src && go test ./internal/memory/... -v
  ```
  预期: PASS.

- [ ] **Step 4: Commit**

  ```bash
  git add src/internal/memory/index.go src/internal/memory/index_test.go
  git commit -m "feat(memory): exclude kb/skills/deprecated/ from index"
  ```

### Task 2.2: scanner.go — journal 全文扫描

- [ ] **Step 1: 写失败测试**

  `src/internal/sevolver/scanner_test.go`:
  ```go
  package sevolver_test

  import (
      "os"
      "testing"
      "time"
      "github.com/41490/ccclaw/internal/sevolver"
  )

  func TestScanJournal_FindsSkillHits(t *testing.T) {
      dir := t.TempDir()
      os.MkdirAll(dir+"/journal/2026/03", 0755)
      os.WriteFile(dir+"/journal/2026/03/12.log.md", []byte(`
  # 2026-03-12 Journal
  used kb/skills/L1/git-conflict.md to resolve merge
  also used kb/skills/L2/deploy-flow.md for deployment
  `), 0644)

      hits, err := sevolver.ScanJournal(dir, time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC))
      if err != nil { t.Fatal(err) }
      if len(hits) != 2 { t.Fatalf("want 2 hits, got %d", len(hits)) }
      found := map[string]bool{}
      for _, h := range hits { found[h.SkillPath] = true }
      if !found["kb/skills/L1/git-conflict.md"] { t.Error("git-conflict not found") }
      if !found["kb/skills/L2/deploy-flow.md"] { t.Error("deploy-flow not found") }
  }

  func TestScanJournal_FindsGapSignals(t *testing.T) {
      dir := t.TempDir()
      os.MkdirAll(dir+"/journal/2026/03", 0755)
      os.WriteFile(dir+"/journal/2026/03/12.log.md", []byte(`
  失败: 无法完成部署
  重试了 3 次仍然报错
  `), 0644)

      gaps, err := sevolver.ScanJournalForGaps(dir, time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC))
      if err != nil { t.Fatal(err) }
      if len(gaps) == 0 { t.Error("expected at least one gap signal") }
  }
  ```

  ```bash
  cd src && go test ./internal/sevolver/... -run TestScanJournal -v
  ```
  预期: FAIL.

- [ ] **Step 2: 实现 scanner.go**

  `src/internal/sevolver/scanner.go`:
  ```go
  package sevolver

  import (
      "bufio"
      "fmt"
      "os"
      "path/filepath"
      "regexp"
      "strings"
      "time"
  )

  var skillPathRe = regexp.MustCompile(`kb/skills/[^\s"']+\.md`)
  var gapKeywords = []string{"失败", "无法", "报错", "error", "failed", "重试", "不会", "找不到"}

  // SkillHit records a kb/skills/*.md path found in journal.
  type SkillHit struct {
      SkillPath string
      Date      time.Time
  }

  // GapSignal records a potential capability gap found in journal.
  type GapSignal struct {
      Date    time.Time
      Context string // surrounding line
      Keyword string
  }

  // ScanJournal scans journal files for skill path references since `since`.
  func ScanJournal(kbDir string, since time.Time) ([]SkillHit, error) {
      var hits []SkillHit
      err := filepath.WalkDir(filepath.Join(kbDir, "journal"), func(path string, d os.DirEntry, err error) error {
          if err != nil || d.IsDir() { return err }
          if !strings.HasSuffix(d.Name(), ".md") { return nil }
          if modTime, e := d.Info(); e == nil && modTime.ModTime().Before(since) { return nil }
          found, err := scanFileForSkills(path, since)
          hits = append(hits, found...)
          return err
      })
      return hits, err
  }

  // ScanJournalForGaps scans journal files for failure/gap keywords.
  func ScanJournalForGaps(kbDir string, since time.Time) ([]GapSignal, error) {
      var gaps []GapSignal
      err := filepath.WalkDir(filepath.Join(kbDir, "journal"), func(path string, d os.DirEntry, err error) error {
          if err != nil || d.IsDir() { return err }
          if !strings.HasSuffix(d.Name(), ".md") { return nil }
          found, err := scanFileForGaps(path)
          gaps = append(gaps, found...)
          return err
      })
      return gaps, err
  }

  func scanFileForSkills(path string, since time.Time) ([]SkillHit, error) {
      f, err := os.Open(path)
      if err != nil { return nil, err }
      defer f.Close()
      var hits []SkillHit
      sc := bufio.NewScanner(f)
      for sc.Scan() {
          matches := skillPathRe.FindAllString(sc.Text(), -1)
          for _, m := range matches {
              hits = append(hits, SkillHit{SkillPath: m, Date: since})
          }
      }
      return hits, sc.Err()
  }

  func scanFileForGaps(path string) ([]GapSignal, error) {
      f, err := os.Open(path)
      if err != nil { return nil, err }
      defer f.Close()
      var gaps []GapSignal
      sc := bufio.NewScanner(f)
      for sc.Scan() {
          line := sc.Text()
          lower := strings.ToLower(line)
          for _, kw := range gapKeywords {
              if strings.Contains(lower, strings.ToLower(kw)) {
                  gaps = append(gaps, GapSignal{Keyword: kw, Context: line})
                  break
              }
          }
      }
      return gaps, sc.Err()
  }

  // WeeklyEventFile returns the JSONL filename for a given time's ISO week.
  func WeeklyEventFile(varDir string, t time.Time) string {
      year, week := t.ISOWeek()
      return filepath.Join(varDir, fmt.Sprintf("events-%d-W%02d.jsonl", year, week))
  }
  ```

- [ ] **Step 3: 运行测试**

  ```bash
  cd src && go test ./internal/sevolver/... -run TestScanJournal -v
  ```
  预期: PASS.

- [ ] **Step 4: Commit**

  ```bash
  git add src/internal/sevolver/scanner.go src/internal/sevolver/scanner_test.go
  git commit -m "feat(sevolver): journal scanner for skill hits and gap signals"
  ```

### Task 2.3: skill_updater.go — frontmatter 生命周期管理

- [ ] **Step 1: 写失败测试**

  `src/internal/sevolver/skill_updater_test.go`:
  ```go
  package sevolver_test

  import (
      "os"
      "path/filepath"
      "testing"
      "time"
      "github.com/41490/ccclaw/internal/sevolver"
  )

  func TestUpdateSkillMeta_UpdatesLastUsed(t *testing.T) {
      dir := t.TempDir()
      skillFile := filepath.Join(dir, "my-skill.md")
      os.WriteFile(skillFile, []byte("---\nname: my-skill\nkeywords: [foo]\n---\nContent\n"), 0644)

      hit := sevolver.SkillHit{SkillPath: skillFile, Date: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)}
      if err := sevolver.UpdateSkillMeta(skillFile, hit); err != nil { t.Fatal(err) }

      b, _ := os.ReadFile(skillFile)
      if !strings.Contains(string(b), "last_used: 2026-03-12") { t.Error("last_used not updated") }
      if !strings.Contains(string(b), "use_count: 1") { t.Error("use_count not updated") }
      if !strings.Contains(string(b), "status: active") { t.Error("status not set") }
  }

  func TestMarkDeprecatedAndArchive(t *testing.T) {
      dir := t.TempDir()
      depDir := filepath.Join(dir, "deprecated")
      os.MkdirAll(depDir, 0755)
      skillFile := filepath.Join(dir, "old-skill.md")
      os.WriteFile(skillFile, []byte("---\nname: old\nstatus: dormant\n---\n"), 0644)

      if err := sevolver.ArchiveDeprecated(skillFile, depDir); err != nil { t.Fatal(err) }

      if _, err := os.Stat(skillFile); !os.IsNotExist(err) {
          t.Error("original file should be gone")
      }
      if _, err := os.Stat(filepath.Join(depDir, "old-skill.md")); err != nil {
          t.Error("file should exist in deprecated/")
      }
  }
  ```

- [ ] **Step 2: 实现 skill_updater.go**

  `src/internal/sevolver/skill_updater.go`:
  ```go
  package sevolver

  import (
      "fmt"
      "os"
      "regexp"
      "strconv"
      "strings"
      "time"
  )

  var fmRe = regexp.MustCompile(`(?s)^---\n(.+?)\n---\n`)

  // UpdateSkillMeta updates last_used, use_count, status in a skill's frontmatter.
  func UpdateSkillMeta(skillFile string, hit SkillHit) error {
      b, err := os.ReadFile(skillFile)
      if err != nil { return err }
      content := string(b)

      dateStr := hit.Date.Format("2006-01-02")
      content = setOrAddField(content, "last_used", dateStr)
      content = incrementField(content, "use_count")
      content = setOrAddField(content, "status", "active")

      return os.WriteFile(skillFile, []byte(content), 0644)
  }

  // MarkDormant sets status: dormant on a skill file.
  func MarkDormant(skillFile string) error {
      b, err := os.ReadFile(skillFile)
      if err != nil { return err }
      return os.WriteFile(skillFile, []byte(setOrAddField(string(b), "status", "dormant")), 0644)
  }

  // ArchiveDeprecated moves a skill file to deprecatedDir/.
  func ArchiveDeprecated(skillFile, deprecatedDir string) error {
      dst := deprecatedDir + "/" + filepath.Base(skillFile)
      b, err := os.ReadFile(skillFile)
      if err != nil { return err }
      if err := os.WriteFile(dst, b, 0644); err != nil { return err }
      return os.Remove(skillFile)
  }

  // setOrAddField sets (or adds) a YAML frontmatter field.
  func setOrAddField(content, key, value string) string {
      re := regexp.MustCompile(`(?m)^` + key + `:.*$`)
      line := key + ": " + value
      if re.MatchString(content) {
          return re.ReplaceAllString(content, line)
      }
      // inject before closing ---
      return strings.Replace(content, "\n---\n", "\n"+line+"\n---\n", 1)
  }

  // incrementField increments a numeric YAML field, defaulting to 0.
  func incrementField(content, key string) string {
      re := regexp.MustCompile(`(?m)^` + key + `:\s*(\d+)$`)
      m := re.FindStringSubmatch(content)
      cur := 0
      if len(m) > 1 {
          cur, _ = strconv.Atoi(m[1])
      }
      return setOrAddField(content, key, fmt.Sprintf("%d", cur+1))
  }
  ```

- [ ] **Step 3: 运行测试**

  ```bash
  cd src && go test ./internal/sevolver/... -run "TestUpdateSkillMeta|TestMarkDeprecated" -v
  ```
  预期: PASS.

- [ ] **Step 4: Commit**

  ```bash
  git add src/internal/sevolver/skill_updater.go src/internal/sevolver/skill_updater_test.go
  git commit -m "feat(sevolver): skill frontmatter lifecycle manager (dormant→deprecated→archive)"
  ```

### Task 2.4: gap_recorder.go + reporter.go + 主入口 sevolver.go

- [ ] **Step 1: 实现 gap_recorder.go**

  `src/internal/sevolver/gap_recorder.go`:
  ```go
  package sevolver

  import (
      "fmt"
      "os"
      "strings"
      "time"
  )

  const gapRetentionDays = 30

  // AppendGapSignals appends gap signals to kb/assay/gap-signals.md.
  func AppendGapSignals(kbDir string, gaps []GapSignal) error {
      if len(gaps) == 0 { return nil }
      path := kbDir + "/assay/gap-signals.md"
      var sb strings.Builder
      for _, g := range gaps {
          sb.WriteString(fmt.Sprintf("- %s | %s | %s\n",
              g.Date.Format("2006-01-02"), g.Keyword, g.Context))
      }
      f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
      if err != nil { return err }
      defer f.Close()
      _, err = f.WriteString(sb.String())
      return err
  }

  // PruneOldGapSignals removes entries older than 30 days from gap-signals.md.
  func PruneOldGapSignals(kbDir string) error {
      path := kbDir + "/assay/gap-signals.md"
      b, err := os.ReadFile(path)
      if os.IsNotExist(err) { return nil }
      if err != nil { return err }
      cutoff := time.Now().AddDate(0, 0, -gapRetentionDays)
      var kept []string
      for _, line := range strings.Split(string(b), "\n") {
          if line == "" { continue }
          // format: "- YYYY-MM-DD | ..."
          if len(line) > 12 {
              if d, err := time.Parse("- 2006-01-02", line[:12]); err == nil {
                  if d.Before(cutoff) { continue }
              }
          }
          kept = append(kept, line)
      }
      return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0644)
  }
  ```

- [ ] **Step 2: 实现 sevolver.go — 主入口编排**

  `src/internal/sevolver/sevolver.go`:
  ```go
  package sevolver

  import (
      "fmt"
      "io"
      "path/filepath"
      "time"
  )

  const (
      dormantDays    = 14
      deprecatedDays = 14 // after dormant
  )

  // Config holds sevolver runtime config.
  type Config struct {
      KBDir  string
      VarDir string
  }

  // Run executes the full sevolver daily pipeline.
  func Run(cfg Config, out io.Writer) error {
      now := time.Now()
      fmt.Fprintln(out, "sevolver: starting daily analysis", now.Format("2006-01-02"))

      // 1. Scan journal for skill hits and gap signals
      hits, err := ScanJournal(cfg.KBDir, now.AddDate(0, 0, -1))
      if err != nil { return fmt.Errorf("scan journal: %w", err) }
      gaps, err := ScanJournalForGaps(cfg.KBDir, now.AddDate(0, 0, -1))
      if err != nil { return fmt.Errorf("scan gaps: %w", err) }
      fmt.Fprintf(out, "  found %d skill hits, %d gap signals\n", len(hits), len(gaps))

      // 2. Update skill frontmatter for hits
      for _, h := range hits {
          fullPath := filepath.Join(cfg.KBDir, h.SkillPath)
          if err := UpdateSkillMeta(fullPath, h); err != nil {
              fmt.Fprintf(out, "  warn: update %s: %v\n", h.SkillPath, err)
          }
      }

      // 3. Check all skills for dormancy/deprecation
      depDir := filepath.Join(cfg.KBDir, "skills", "deprecated")
      if err := processSkillLifecycle(cfg.KBDir, depDir, now, out); err != nil {
          fmt.Fprintf(out, "  warn: lifecycle: %v\n", err)
      }

      // 4. Record gap signals
      if err := AppendGapSignals(cfg.KBDir, gaps); err != nil {
          return fmt.Errorf("record gaps: %w", err)
      }
      if err := PruneOldGapSignals(cfg.KBDir); err != nil {
          return fmt.Errorf("prune gaps: %w", err)
      }

      fmt.Fprintln(out, "sevolver: done")
      return nil
  }
  ```

  `processSkillLifecycle` 扫描所有 skills/*.md，检查 `last_used` 字段，超 14 天设为 dormant，再超 14 天（status=dormant 且 last_used > 28 天前）则 ArchiveDeprecated。

- [ ] **Step 3: 实现 ccclaw sevolver 子命令**

  `src/cmd/ccclaw/sevolver.go`:
  ```go
  package main
  // (同 archive.go 模式: cobra.Command + runE 调用 sevolver.Run)
  ```

- [ ] **Step 4: 新增 systemd 定时器**

  `src/dist/ops/systemd/ccclaw-sevolver.timer`:
  ```ini
  [Timer]
  OnCalendar=*-*-* 22:00:00 Asia/Shanghai
  Persistent=true
  ```

- [ ] **Step 5: 新增 kb 模板文件**

  - `src/dist/kb/assay/signal-rules.md` — managed section 含默认关键词规则
  - `src/dist/kb/assay/gap-signals.md` — 空文件 + 说明 header
  - `src/dist/kb/skills/deprecated/.gitkeep`

- [ ] **Step 6: 全量编译 + 测试**

  ```bash
  cd src && go build ./... && go test ./internal/sevolver/... -v
  ```

- [ ] **Step 7: Commit**

  ```bash
  git add src/internal/sevolver/ src/cmd/ccclaw/sevolver.go \
          src/dist/ops/systemd/ccclaw-sevolver.* \
          src/dist/kb/assay/ src/dist/kb/skills/deprecated/
  git commit -m "feat(sevolver): daily self-evolution service with skill lifecycle management"
  ```

---

## Chunk 4: Phase 3 — sevolver 深度分析触发器

**Scope:** 纯 Go 关键词提取积累到阈值（3 日连续同类缺口 or 累积 5 条未解决）后，触发一次 Claude Code 深度分析任务。

**Files:**
- Create: `src/internal/sevolver/deep_trigger.go` + `deep_trigger_test.go`
- Modify: `src/internal/sevolver/sevolver.go` — 在 Run() 末尾集成触发逻辑

### Task 3.1: deep_trigger.go — 阈值检测 + 任务创建

- [ ] **Step 1: 写失败测试**

  `src/internal/sevolver/deep_trigger_test.go`:
  ```go
  func TestShouldTriggerDeepAnalysis_ThresholdMet(t *testing.T) {
      gaps := []GapSignal{
          {Keyword: "失败", Date: time.Now().AddDate(0,0,-3)},
          {Keyword: "失败", Date: time.Now().AddDate(0,0,-2)},
          {Keyword: "失败", Date: time.Now().AddDate(0,0,-1)},
      }
      if !ShouldTriggerDeepAnalysis(gaps) { t.Error("should trigger: 3 days consecutive") }
  }

  func TestShouldTriggerDeepAnalysis_ThresholdNotMet(t *testing.T) {
      gaps := []GapSignal{{Keyword: "失败"}, {Keyword: "失败"}}
      if ShouldTriggerDeepAnalysis(gaps) { t.Error("should not trigger: only 2") }
  }
  ```

- [ ] **Step 2: 实现 deep_trigger.go**

  ```go
  package sevolver

  import (
      "time"
  )

  const (
      consecutiveDayThreshold = 3
      accumulatedGapThreshold = 5
  )

  // ShouldTriggerDeepAnalysis returns true if gap signals cross the threshold.
  func ShouldTriggerDeepAnalysis(gaps []GapSignal) bool {
      if len(gaps) >= accumulatedGapThreshold { return true }
      // check for 3 consecutive days with same keyword
      keyDays := map[string]map[string]bool{}
      for _, g := range gaps {
          day := g.Date.Format("2006-01-02")
          if keyDays[g.Keyword] == nil { keyDays[g.Keyword] = map[string]bool{} }
          keyDays[g.Keyword][day] = true
      }
      for _, days := range keyDays {
          if hasConsecutiveDays(days, consecutiveDayThreshold) { return true }
      }
      return false
  }

  func hasConsecutiveDays(days map[string]bool, n int) bool {
      // check if any n consecutive calendar days all appear in days
      for dayStr := range days {
          d, _ := time.Parse("2006-01-02", dayStr)
          streak := 0
          for i := 0; i < n; i++ {
              if days[d.AddDate(0, 0, i).Format("2006-01-02")] { streak++ }
          }
          if streak >= n { return true }
      }
      return false
  }
  ```

- [ ] **Step 3: 集成到 sevolver.Run() — 触发 GitHub Issue 深度分析任务**

  在 `sevolver.go` 的 `Run()` 末尾：
  ```go
  if ShouldTriggerDeepAnalysis(gaps) {
      fmt.Fprintln(out, "  threshold met: creating deep analysis issue")
      // 创建 GitHub Issue 作为 CCClaw 任务（由 ingest 拉起）
      // Issue title: "[sevolver] 能力缺口深度分析 YYYY-MM-DD"
      // Issue body: 累积的 gap signals + 请 Claude 分析并推荐 skill 改进
      // 通过 gh CLI 创建，避免直接依赖 GitHub SDK
      _ = createDeepAnalysisIssue(cfg, gaps, out)
  }
  ```

  `createDeepAnalysisIssue` 使用 `exec.Command("gh", "issue", "create", ...)` 创建 Issue，打上 `ccclaw` label，让 ingest 自动拉起处理。

- [ ] **Step 4: 运行全量测试**

  ```bash
  cd src && go test ./... 2>&1 | grep -E "FAIL|ok"
  ```
  预期: 全 ok.

- [ ] **Step 5: Commit**

  ```bash
  git add src/internal/sevolver/deep_trigger.go \
          src/internal/sevolver/deep_trigger_test.go \
          src/internal/sevolver/sevolver.go
  git commit -m "feat(sevolver): deep analysis threshold trigger via GitHub Issue creation"
  ```

---

## 执行顺序与里程碑

```
Phase 0 (升级保护)        → 1-2h  → Tag: v26.03.12-p0
Phase 1 (state_db 迁移)  → 2-3d  → Tag: v26.03.14-p1 (解决 #31)
Phase 2 (sevolver)       → 1-2d  → Tag: v26.03.16-p2
Phase 3 (deep trigger)   → 0.5d  → Tag: v26.03.17-p3
```

**关键约束:**
- `go build` 保持 `CGO_ENABLED=0` — DuckDB 仅通过 CLI 子进程调用，**无 CGO**
- 每个 Phase 完成后独立可部署，不依赖后续 Phase
- Phase 1 完成即解决 #31 锁冲突（JSONL 追加无锁）
