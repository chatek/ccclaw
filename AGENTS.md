# AGENTS.md — ccclaw 执行约束

本文件给所有执行体提供统一规则；Claude Code 专属约束继续写在 `CLAUDE.md`。

## 基本约束

- 所有注释、文档、报告使用中文
- 默认先查 Issue 决策，再动代码
- 未拍板的分歧点必须先回 Issue 讨论，禁止脑补式决定
- 变更优先解决根因，不做表面补丁
- 完成后必须补 `docs/reports/` 工程报告
- 除非绝对必要，否则优先直接在 `main` 上修订；仅在需要隔离风险、并行评审或分阶段交付时再新开书签/分支
- `ccclaw` 无参数与 `-h|--help` 默认输出帮助，`-V|--version` 输出版本
- 版本号统一使用 `yy.mm.dd.HHMM`

## 当前架构基线

- 当前阶段以 `phase0.5` 为准，默认执行模式是 `daemon`
- `tmux` 仅保留 debug attach、旧链路兼容与 patrol 补偿用途，不再作为默认执行内核
- 执行输出以 `stream-json` 为事实基线，运行态产物统一保留 `*.stream.jsonl`、`*.event.json` 与兼容 `*.json`
- `stats` 与 `status` 的聚合口径统一由 `src/internal/adapters/storage/` 提供，禁止在 CLI 或调用层重复拼装并行统计逻辑
- `sevolver` 负责 skill 生命周期维护、gap 聚合、deep-analysis Issue 升级与关闭收敛回写；相关字段变更必须同步 `kb/**/CLAUDE.md`
- 调度后端以 `systemd --user` 为优先，支持 `cron|none` 降级；安装、升级、排障文档必须同时覆盖三种状态

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

- `maintain` 及以上权限的 Issue 自动执行
- 其他 Issue 仅在受信任成员评论 `/ccclaw <批准词>` 后执行
- 批准词默认支持 `approve/go/confirm/批准/agree/同意/推进/通过/ok`
- 否决词默认支持 `reject/no/cancel/nil/null/拒绝/000`
- 成员身份必须通过 GitHub 权限动态判断

## 发布约束

- `src/Makefile` 是统一发布入口
- release 必须从 `src/dist/` 打包
- release 至少包含安装包与 `SHA256SUMS`
- 发布与升级禁止覆盖 `/opt/ccclaw` 本体仓库中的用户记忆
