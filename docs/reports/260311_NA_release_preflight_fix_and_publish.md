# 260311_NA_release_preflight_fix_and_publish

## 背景

2026-03-11，用户要求“立即将当前版本发布为 release，以便进行 upgrade 检验”。

按仓库约束，先核查了当前公开议题与发布入口：

- `gh issue list --state open --limit 30` 显示当前 open issue 为 `#27/#26/#25/#24/#22`，没有单独的“立即发布当前版本”议题。
- `src/Makefile` 明确是统一发布入口，release 必须从 `src/dist/` 打包，并同时产出安装包与 `SHA256SUMS`。
- 当前工作树在执行前为干净状态，`gh auth status` 已登录可用。

## 发现的问题

首次直接执行 `cd src && make release` 失败，暴露出发布链路自锁：

1. `release` 目标先执行 `archive` 和 `release-notes`
2. 这两个目标内部会触发 `dist-sync`
3. `dist-sync` 会把 `src/ops/` 同步到受跟踪的 `src/dist/ops/`
4. 最后才执行 `git diff --quiet`

结果是：即使初始工作树干净，只要 `src/ops/` 与 `src/dist/ops/` 有任何已提交但未同步的差异，`make release` 也会把自己阻塞掉。

本次实际暴露出的不同步文件有：

- `src/dist/ops/config/config.example.toml`
- `src/dist/ops/examples/install-flow.md`
- `src/dist/ops/examples/release-notes-template.md`

## 修复动作

### 1. 修正 release 前置门禁顺序

在 `src/Makefile` 中新增 `release-preflight`：

- 先检查工作树是否干净
- 再检查 `gh` 是否已登录
- 再检查当前 `HEAD` 是否已经推送到上游分支
- 仅在前置检查通过后，才调用 `archive` 与 `release-notes`

同时 `gh release create` 现在显式带 `--target <full_sha>`，避免 tag 模糊落到远端旧提交。

这样 `release` 的语义回到“用干净且已推送的源码创建发布”，而不是“先改工作树，再检查自己是否改过工作树”。

### 2. 同步已跟踪的 dist 文档

将以下文件同步到与 `src/ops/` 一致：

- `src/dist/ops/config/config.example.toml`
- `src/dist/ops/examples/install-flow.md`
- `src/dist/ops/examples/release-notes-template.md`

同步内容主要是“本体仓库”术语统一为“知识仓库”，以及补齐 ingest / approval 的说明注释。

### 3. 固化修复提交

为保证 release tag 对应真实源码状态，先提交最小修复：

- 提交：`df4a706`
- 标题：`fix: 修正 release 预检顺序并同步 dist 文档`

在发现“本地 ahead 但远端 tag 仍指向旧提交”之后，又继续补了一轮发布门禁修复，避免后续再次踩坑。

## 发布结果

修复提交后第一次重新执行 `cd src && make release` 时，虽然资产上传成功，但随后核实发现：

- 本地源码已前进到 `df4a706`
- 远端 `main` 仍停留在 `af2cfb0`
- `gh release create` 在未显式指定 tag 提交的情况下，实际把 `26.03.11.1127` 建在了远端旧提交 `af2cfb0` 上

这会导致“release 源码快照”和“本地产出的安装包资产”不一致，因此立即执行了纠正动作：

1. `git push origin main`，把本地修复与报告提交推到远端
2. `gh release delete 26.03.11.1127 --cleanup-tag --yes`，删除错位 release 与 tag
3. 再次执行 `cd src && make release`

最终有效发布结果为：

- release tag：`26.03.11.1129`
- 发布时间：`2026-03-11T15:30:01Z`
- 目标分支：`main`
- tag 实际指向提交：`6304696`
- release 页面：<https://github.com/41490/ccclaw/releases/tag/26.03.11.1129>

发布资产：

- `ccclaw_26.03.11.1127_linux_amd64.tar.gz`
- `SHA256SUMS`

本机归档目录：

- `/ops/logs/ccclaw/26.03.11.1127/`

## 校验信息

安装包 `sha256`：

```text
4b518be754f5d635a24e8d940cf49649545760bf41fef81818d8ce065a0fd2f8
```

`src/.release/SHA256SUMS` 内容与本地重新计算结果一致。

GitHub release 资产元数据也显示安装包 digest 为：

```text
sha256:4b518be754f5d635a24e8d940cf49649545760bf41fef81818d8ce065a0fd2f8
```

## 验证过程

本轮实际执行并确认：

```bash
gh issue list --state open --limit 30
gh auth status
cd src && make test
cd src && make release
git push origin main
gh release delete 26.03.11.1127 --cleanup-tag --yes
cd src && make release
gh release view 26.03.11.1129 --json url,tagName,name,assets,publishedAt,targetCommitish
gh api repos/41490/ccclaw/git/ref/tags/26.03.11.1129
ls -lah /ops/logs/ccclaw/26.03.11.1129
sha256sum src/.release/ccclaw_26.03.11.1129_linux_amd64.tar.gz
cat src/.release/SHA256SUMS
```

验证结果：

- `make test` 通过
- 首次 `make release` 成功复现了自锁问题
- 第二次 `make release` 暴露出“本地 ahead 但 release tag 仍落到远端旧提交”的错位问题
- 推送提交并清理错误 tag 后，最终 `make release` 成功创建正确 GitHub release
- 本机归档目录存在安装包与 `SHA256SUMS`
- 校验值在本地文件与 GitHub 资产元数据之间一致
- 最终 release tag `26.03.11.1129` 已确认指向提交 `6304696`

## 结论

本轮已完成“当前版本立即发布为 release”目标，同时修复并识别了两个发布风险：

- `src/Makefile` 的 preflight 顺序错误会导致 release 自锁
- 本地提交未先 push 时，`gh release create` 会让 tag 落在远端旧提交上；现已通过 upstream 同步校验与 `--target <full_sha>` 双重约束收口

当前有效 release 为 `26.03.11.1129`，且其 tag、源码、资产与本机归档已经对齐。
