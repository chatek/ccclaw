# 260311_NA_local_release_package_hold

## 背景

用户要求先编译并打包最新 release 版本，但本轮只允许在本地 `src/.release/` 目录生成发布资产，暂不发布到 GitHub，等待人工测试通过后再正式发布。

结合仓库现有发布约束，确认应走 `src/Makefile` 的非发布链路：

- release 打包源固定为 `src/dist/`
- release 资产至少包含安装包与 `SHA256SUMS`
- `make release` 会调用 `gh release create`，不符合“先只本地打包”的要求
- 因此本轮只执行 `make release-notes`，由其串联 `build/package/checksum`，但不进入 GitHub 发布步骤

## 执行

本轮固定版本号为 `26.03.11.0440`，执行：

```bash
cd src
go test ./...
make release-notes VERSION=26.03.11.0440
```

其中 `make release-notes` 实际完成了以下链路：

- `build`
- `dist-sync`
- `package`
- `checksum`
- `release-notes`

未执行：

- `make archive`
- `make release`

因此没有向 `/ops/logs/ccclaw/` 归档，也没有创建 GitHub release。

## 结果

已在 `src/.release/` 生成本地发布资产：

- `ccclaw_26.03.11.0440_linux_amd64.tar.gz`
- `SHA256SUMS`
- `RELEASE_NOTES.md`
- 解包目录 `ccclaw_26.03.11.0440_linux_amd64/`

安装包 SHA256 为：

```text
ae46c80e6faa939239d42a52e9b2f09fb383334870e6bb374cb4c8506a699ae6
```

## 验证

已完成以下验证：

- `go test ./...` 通过
- `./.release/ccclaw_26.03.11.0440_linux_amd64/bin/ccclaw -V` 输出 `26.03.11.0440`
- `.release/RELEASE_NOTES.md` 已按模板渲染成功

## 结论

本轮已完成“先本地生成最新 release 包、暂不发布”的要求。后续待人工测试通过后，可基于同一版本资产决定是否执行正式归档与 GitHub release。
