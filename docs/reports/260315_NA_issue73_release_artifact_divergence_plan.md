# 260315_NA_issue73_release_artifact_divergence_plan

## 背景

用户要求针对本轮新发现的关键 CD 问题：

- 本地 release 缓存包与 GitHub release 资产不一致

新建一个专门 Issue，深入探查根因，给出详细方案，并在 Issue 中明确提醒需要拍板的决策点。

## 前置核查

本轮先按仓库约束检查是否已有同类议题，再下钻发布链路：

1. `gh issue list --state all --limit 100` 检索 `release/缓存/SHA256/archive`
2. 复核 `src/Makefile`
3. 对 `26.03.15.1721` 的以下三份资产做差异核验：
   - `/ops/logs/ccclaw/26.03.15.1721/ccclaw_26.03.15.1721_linux_amd64.tar.gz`
   - `src/.release/ccclaw_26.03.15.1721_linux_amd64.tar.gz`
   - GitHub release `26.03.15.1721` 对应下载资产

核查结果：

- 截至本轮没有现成的专门 Issue 直接跟踪“本地缓存包与 GitHub 发布资产不一致”
- `#72` 只是本次发布升级汇报入口，不适合作为修复 Issue

## 根因探查结论

### 1. 触发点在 `src/Makefile`

关键链路：

- `package`：`dist-sync -> build -> tar -czf`
- `checksum`：依赖 `package`
- `archive`：依赖 `checksum`
- `release-notes`：依赖 `checksum`
- `release`：
  - 先递归执行 `make archive`
  - 再递归执行 `make release-notes`
  - 最后执行 `gh release create`

也就是说，一次 `make release` 实际会跑两次完整的：

- `build`
- `package`
- `checksum`

### 2. 差异不是内容漂移，而是 artifact 生成时机漂移

对 `26.03.15.1721` 的实测结果：

- 展开后的文件内容一致
- `bin/ccclaw` 二进制内容 hash 一致
- 但两个 tar 包中的 `bin/ccclaw` mtime 不一致：
  - 缓存包：`2026-03-15 05:21:41`
  - `.release` / GitHub 资产：`2026-03-15 05:21:42`

这说明：

1. 第二次 `go build` 重新写了 `dist/bin/ccclaw`
2. `tar` 记录了新的 mtime
3. 于是 tar 流发生变化
4. 最终导致：
   - `/ops/logs/ccclaw/<version>/` 缓存的是第一次构建后的 artifact
   - GitHub release 上传的是第二次构建后的 artifact

### 3. 证据链

- 两份 `.tar.gz` SHA256 不一致
- 两份解压后的 `bin/ccclaw` SHA256 一致
- 两份解压树内容 hash 一致
- `tar --full-time -tvzf` 显示 `bin/ccclaw` mtime 相差 1 秒

因此根因可以明确写成：

- 当前 release 编排不是“单次构建、单份 artifact、多处复用”
- 而是“同一版本在一次 release 流程中被重复构建并重复打包”

## 新建 Issue

已创建：

- Issue：[#73](https://github.com/41490/ccclaw/issues/73)
- 标题：`bug: release 归档缓存与 GitHub 发布资产不一致`

Issue 主体说明了：

1. 发现事实
2. 已确认根因边界
3. 风险
4. 本 Issue 目标

## Issue 回帖方案

已在 Issue #73 发布详细评论：

- 评论链接：<https://github.com/41490/ccclaw/issues/73#issuecomment-4062655051>

回帖内容包括：

### 1. 根因定位

- 明确指出 `release` 中两次递归 `$(MAKE)` 是直接触发点
- 明确指出第二次 `build/package` 刷新了 `bin/ccclaw` 的 mtime
- 明确指出问题不是内容不一致，而是 artifact 身份不一致

### 2. 推荐修复方案

分两层：

#### Phase 1：先收敛为单 artifact 流

- 一次 `make release` 只允许生成一份 `PACKAGE_FILE`
- 本地缓存、`.release/`、GitHub release 全部复用同一份文件

#### Phase 2：按需补可复现归档强化

- 固定 tar 顺序、owner/group、mtime、gzip header 等
- 目标是未来做到跨机器/跨时间重复构建仍得到相同字节流

### 3. 最小验收标准

要求修复后至少满足：

1. 单次 `make release` 只发生一次 `go build`
2. `/ops/logs/ccclaw/<version>/` 与 `src/.release/` SHA256 一致
3. GitHub 再下载回来的 release 资产 SHA256 与本地缓存一致
4. `SHA256SUMS` 在三处语义统一
5. 当前发布、升级能力不退化

### 4. 待拍板决策点

我在评论里明确提醒了 3 个不宜擅自拍板的问题：

1. 本地缓存是否必须成为 GitHub release 资产的严格镜像
2. 本 Issue 是否只做 Phase 1，还是连 Phase 2 一起做
3. 本地缓存的落盘时机应放在上传前复制，还是上传成功后再回填

## 我的建议

当前最稳妥的拍板应为：

1. `/ops/logs/ccclaw/<version>/` 必须与 GitHub release 资产严格一致
2. 本 Issue 先只修 Phase 1，不扩到完整 reproducible build
3. 本地缓存继续走“本地生成后即复制”，但前提是上传与缓存严格复用同一份 artifact

理由：

- 这三点可以先把当前明确的 CD 缺陷真正收口
- 同时不把问题过度扩展成更重的供应链工程
- Phase 2 可在后续单独开 RFC/Issue 深入

## 结论

本轮已完成：

1. 深入探查 `Makefile` release 双重构建根因
2. 创建专门 Issue #73
3. 在 Issue 中回复详细方案
4. 明确提醒需要用户拍板的具体决策项

本轮未修改运行代码与发布脚本。
