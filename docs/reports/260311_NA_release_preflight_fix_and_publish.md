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
- 仅在前置检查通过后，才调用 `archive` 与 `release-notes`

这样 `release` 的语义回到“用干净源码创建发布”，而不是“先改工作树，再检查自己是否改过工作树”。

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

## 发布结果

修复提交后重新执行 `cd src && make release`，发布成功：

- release tag：`26.03.11.1127`
- 发布时间：`2026-03-11T15:27:31Z`
- 目标分支：`main`
- release 页面：<https://github.com/41490/ccclaw/releases/tag/26.03.11.1127>

发布资产：

- `ccclaw_26.03.11.1127_linux_amd64.tar.gz`
- `SHA256SUMS`

本机归档目录：

- `/ops/logs/ccclaw/26.03.11.1127/`

## 校验信息

安装包 `sha256`：

```text
3ff84327ffc4116ee9eea4c99f69ab792938441f6f48d4e56f1793866e17897f
```

`src/.release/SHA256SUMS` 内容与本地重新计算结果一致。

GitHub release 资产元数据也显示安装包 digest 为：

```text
sha256:3ff84327ffc4116ee9eea4c99f69ab792938441f6f48d4e56f1793866e17897f
```

## 验证过程

本轮实际执行并确认：

```bash
gh issue list --state open --limit 30
gh auth status
cd src && make test
cd src && make release
gh release view 26.03.11.1127 --json url,tagName,name,assets,publishedAt,targetCommitish
ls -lah /ops/logs/ccclaw/26.03.11.1127
sha256sum src/.release/ccclaw_26.03.11.1127_linux_amd64.tar.gz
cat src/.release/SHA256SUMS
```

验证结果：

- `make test` 通过
- 首次 `make release` 成功复现了自锁问题
- 修复后 `make release` 成功创建 GitHub release
- 本机归档目录存在安装包与 `SHA256SUMS`
- 校验值在本地文件与 GitHub 资产元数据之间一致

## 结论

本轮已完成“当前版本立即发布为 release”目标，同时顺手修复了发布入口的真实根因。后续再做 release 时，`src/Makefile` 已不会因为 `dist-sync` 修改受跟踪文件而自锁。
