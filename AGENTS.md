# AGENTS.md — ccclaw 执行约束

本文件给所有执行体提供统一规则；Claude Code 专属约束继续写在 `CLAUDE.md`。

## 基本约束

- 所有注释、文档、报告使用中文
- 默认先查 Issue 决策，再动代码
- 未拍板的分歧点必须先回 Issue 讨论，禁止脑补式决定
- 变更优先解决根因，不做表面补丁
- 完成后必须补 `docs/reports/` 工程报告
- `ccclaw` 无参数与 `-h|--help` 默认输出帮助，`-V|--version` 输出版本
- 版本号统一使用 `yy.mm.dd.HHMM`

## 目录约束

- 根目录只保留项目说明、治理文件、开发文档
- 所有源码、构建、发布资产进入 `src/`
- `src/dist/` 是安装部署目录树与 release 打包源
- `docs/` 只放开发过程文档，不作为运行时配置目录
- 本机 release 归档目录固定为开发机外部目录 `/ops/logs/ccclaw/`，不得纳入版本控制

## 配置约束

- 敏感信息仅允许存入固定 `.env` 文件
- 一般配置统一使用 `.toml`
- 运行时不依赖调用者预先导出的环境变量

## 执行门禁

- 仅管理员 Issue 自动执行
- 非管理员 Issue 仅在管理员评论 `/ccclaw approve` 后执行
- 管理员身份必须通过 GitHub 权限动态判断

## 发布约束

- `src/Makefile` 是统一发布入口
- release 必须从 `src/dist/` 打包
- release 至少包含安装包、`SHA256SUMS`、`gpg` 签名与 `minisign` 签名
- 发布与升级禁止覆盖 `/opt/ccclaw` 本体仓库中的用户记忆
