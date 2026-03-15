# 260315_73_release_artifact_single_flow_fix

## 背景

- 对应 Issue：[#73](https://github.com/41490/ccclaw/issues/73)
- 问题：`make release` 在一次发布过程中会重复触发 `build/package/checksum`，导致：
  - `/ops/logs/ccclaw/<version>/` 本地缓存包
  - GitHub release 上传资产
  不是同一份 artifact

用户已在 Issue #73 明确拍板：

1. `/ops/logs/ccclaw/<version>/` 必须与 GitHub release 资产严格一致
2. 本 Issue 先只做 Phase 1，收敛为单次 release 只产一份 artifact
3. 本地缓存继续采用“本地生成后即复制”，但上传与缓存必须复用同一份文件

## 变更方案

修改文件：

- `src/Makefile`

### 1. 新增 `release-assets`

新增聚合目标：

```make
release-assets: checksum release-notes
```

作用：

- 在同一 make 进程内先生成发布资产与 `SHA256SUMS`
- 再生成 release notes
- 避免 `release` 通过递归 `$(MAKE)` 重新触发第二次构建

### 2. 改写 `release` 依赖图

原实现：

- `release` 先递归 `make archive`
- 再递归 `make release-notes`

新实现：

```make
release: release-preflight release-assets archive
```

这样：

- `archive` 与 `gh release create` 都复用同一个 `$(PACKAGE_FILE)` 与 `$(CHECKSUM_FILE)`
- 在一次 `make release` 中，`build/package/checksum` 只会跑一次

### 3. 保持缓存落盘时机不变

本轮没有改动缓存落盘时机，仍是：

1. 本地生成 artifact
2. 复制到 `/ops/logs/ccclaw/<version>/`
3. 再用同一份 `.release` 产物上传 GitHub release

这符合本轮拍板的 A3。

## 验证

### 1. 依赖图静态检查

执行：

```bash
cd /opt/src/ccclaw/src
make -n release VERSION=26.03.15.1735
```

结果：

- 只出现一次 `go build`
- 只出现一次 `tar -C .release -czf ...`
- `archive` 与 `gh release create` 都引用同一个 `.release/ccclaw_<version>_linux_amd64.tar.gz`

### 2. 单进程本地回归

执行：

```bash
cd /opt/src/ccclaw/src
make clean
make release-assets archive VERSION=26.03.15.1735 ARCHIVE_ROOT=/tmp/ccclaw-release-archive.2X37P6
```

观察到：

- 实际输出里只出现一次 `go build`

### 3. 资产一致性校验

执行：

```bash
sha256sum \
  /opt/src/ccclaw/src/.release/ccclaw_26.03.15.1735_linux_amd64.tar.gz \
  /tmp/ccclaw-release-archive.2X37P6/26.03.15.1735/ccclaw_26.03.15.1735_linux_amd64.tar.gz \
  /opt/src/ccclaw/src/.release/SHA256SUMS \
  /tmp/ccclaw-release-archive.2X37P6/26.03.15.1735/SHA256SUMS
```

结果：

- `PACKAGE_FILE` 两处 SHA256 完全一致
- `SHA256SUMS` 两处 SHA256 完全一致

额外执行：

```bash
cmp -s ...package... ...package...
cmp -s ...SHA256SUMS... ...SHA256SUMS...
```

结果：

- `package_equal=yes`
- `checksum_equal=yes`

### 4. 测试

执行：

```bash
cd /opt/src/ccclaw/src
make test
```

结果：

- `go test ./...` 全部通过

## 结论

Issue #73 的 Phase 1 已落地：

1. `release` 不再通过递归 `make` 触发重复构建
2. 单次发布流程中只生成一份 artifact
3. 本地缓存与 `.release` 产物已能做到严格一致

本轮尚未做：

- 跨机器/跨时间的 reproducible packaging
- 固定 tar/gzip 元数据规范化

这些属于后续可选的 Phase 2，不在本轮修复范围内。
