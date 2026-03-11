# Issue #14 渐进式 kb 加载最小实现

## 背景

- 依据 Issue #14 在 2026-03-11 的拍板评论，仅推进 `4.1 渐进式上下文加载`
- 元数据载体采用 `CLAUDE.md` YAML frontmatter
- 本轮要求“最小化进步”，不连带推进 `4.2` 全量目录迁移，也不进入 `4.3` 强制验证门控

## 本次实现

### 1. kb 索引改为元数据/摘要导向

- `src/internal/memory/index.go` 不再把 Markdown 正文作为匹配与 prompt 注入的默认载荷
- 新增对 `CLAUDE.md` YAML frontmatter 的解析，当前识别：
  - `name`
  - `description`
  - `trigger`
  - `keywords`
- 无 frontmatter 时，回退到 Markdown 标题与首条有效正文作为摘要

### 2. prompt 中的 kb 注入从全文改为摘要

- `src/internal/app/runtime.go` 的 `buildPrompt()` 改为只注入：
  - 路径
  - 标题
  - 摘要
  - 关键词
- 同时明确提示执行器：仅在确有必要时再按路径打开原文细节

这一步已经形成第一层“元数据先行”的最小闭环，直接降低默认 prompt 体积。

### 3. `target.kb_path` 真正参与索引

- 运行时新增按 target 维度选择 kb 索引的逻辑
- 若 target 指定 `kb_path`，则优先使用该路径构建/缓存索引
- 解决了此前配置存在但运行时仍固定使用全局 `kb_dir` 的偏差

### 4. Skill 规范文档补充 frontmatter 约束

- 更新 `src/dist/kb/skills/CLAUDE.md`
- 更新 `src/dist/kb/skills/L1/CLAUDE.md`
- 更新 `src/dist/kb/skills/L2/CLAUDE.md`

本轮只补规范说明，不迁移现有知识资产。

## 验证

执行：

```bash
cd /opt/src/ccclaw/src
make fmt
make test
```

结果：

- `make fmt` 通过
- `make test` 通过

新增覆盖：

- `src/internal/memory/index_test.go`
  - 验证 frontmatter 元数据匹配
  - 验证无 frontmatter 时的摘要回退
- `src/internal/app/runtime_test.go`
  - 验证 `target.kb_path` 生效
  - 验证 prompt 不再注入 kb 正文全文

## 收益

- 立即减少默认 prompt 中的 kb 载荷
- 为后续目录制 Skill 留出 frontmatter 兼容入口
- 修正 target 级 kb 路由未生效的问题

## 未做事项

- 未实现“匹配后再二次按需加载 references 正文”的第三层加载
- 未对现有 kb 文档做批量目录化迁移
- 未接入任务完成态的强制验证门控

## 后续建议

- 下一步可在 `kb/skills/` 下先选 1 到 2 个高频 Skill，按目录制 + frontmatter 试点
- 待真实 Skill 数量增长后，再补更细的 summary 索引与 references 按需加载
- `4.3` 建议独立成一轮，以免把 prompt 优化和状态机改造混在一起
