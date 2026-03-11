# 260311_NA_release_publish_announcement

## 背景

在本地完成 `26.03.11.0440` 版本打包并经过人工安装验证成功后，需要将该版本正式发布到 GitHub Releases，并在仓库 Discussions 的 `Announcements` 分类下补一则对应公告。

## 执行

本轮先核查了以下前置条件：

- `gh auth status` 已登录，且 token 具备 `repo` 与 `write:discussion` 权限
- GitHub 上不存在同名 release `26.03.11.0440`
- 仓库 `41490/ccclaw` 的 Discussions 已启用 `Announcements` 分类

随后执行正式发布：

```bash
gh release create 26.03.11.0440 \
  src/.release/ccclaw_26.03.11.0440_linux_amd64.tar.gz \
  src/.release/SHA256SUMS \
  --repo 41490/ccclaw \
  --target 3c3637ba3c9f29e1ba2ea4df350aa5cefd3f83ee \
  --title "26.03.11.0440" \
  --notes-file src/.release/RELEASE_NOTES.md
```

之后通过 GitHub GraphQL API 在 `Announcements` 分类创建讨论公告，标题为：

```text
ccclaw 26.03.11.0440 已发布
```

## 结果

GitHub Release 已创建成功：

- release URL：<https://github.com/41490/ccclaw/releases/tag/26.03.11.0440>
- tag：`26.03.11.0440`
- 发布时间：`2026-03-11T08:50:32Z`

已上传资产：

- `ccclaw_26.03.11.0440_linux_amd64.tar.gz`
- `SHA256SUMS`

对应公告已创建成功：

- discussion URL：<https://github.com/41490/ccclaw/discussions/21>
- 分类：`Announcements`

## 验证

已核查：

- `gh release view 26.03.11.0440 --repo 41490/ccclaw --json url,name,tagName,publishedAt,isDraft,isPrerelease,assets` 返回成功
- release 页面已显示 2 个资产，安装包 digest 为 `sha256:ae46c80e6faa939239d42a52e9b2f09fb383334870e6bb374cb4c8506a699ae6`
- `Announcements` 分类下此前无同版本公告，本轮新建讨论成功返回 URL

## 结论

`26.03.11.0440` 已从“仅本地待测版本”切换为正式 GitHub 发布版本，并补齐了对外公告入口，后续可直接基于 release 页面继续分发与收集安装反馈。
