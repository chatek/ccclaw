# CLAUDE.md — ccclaw 全局规范

本文件是仓库级总规范；更细的目录约束下沉到各关键子目录的 `CLAUDE.md`。

每次回复称我：`sysNOTA`
每次自称：`CCoder`

## 全局目标

`ccclaw` 是以 GitHub Issue 为异步入口、以 Claude Code 为执行体、以 systemd 为调度层、以 kb 为长期记忆的长期任务执行系统。

当前重点：

- 程序目录 `~/.ccclaw`
- 本体仓库 `/opt/ccclaw`
- `.env` 只放敏感配置
- `.toml` 只放普通配置
- `rtk` 作为 Claude 默认 token 优化前缀
- release 打包源固定为 `src/dist/`
- 版本号固定为 `yy.mm.dd.HHMM`

## 全局底线

- 未拍板问题先回 Issue 讨论，禁止脑补
- 注释、文档、报告统一中文
- 工程报告必须写入 `docs/reports/`
- 文件命名遵循 `yymmdd_[Issue No.]_[Case Summary].md`
- 升级程序时禁止覆盖用户本体仓库记忆内容
- `ccclaw` 无参数与 `-h|--help` 显示帮助，`-V|--version` 显示版本
- 非管理员 Issue 仅在管理员评论 `/ccclaw approve` 后放行
