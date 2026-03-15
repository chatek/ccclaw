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
- release 校验统一使用 `SHA256SUMS`
- 安装时本体仓库支持 `init|remote|local`
- 安装时任务仓库绑定支持 `remote|local`
- 升级仅允许无损刷新关键 `kb/**/CLAUDE.md`，不得覆盖用户记忆

## 当前架构基线

- 执行默认走 `daemon`，`tmux` 只作为 debug attach、旧链路兼容与 patrol 补偿手段
- Claude 输出协议默认走 `stream-json`，运行产物固定包含 `*.stream.jsonl`、`*.event.json` 与兼容 `*.json`
- 运行态事实统一沉淀到 `state.json + JSONL/hashchain`，`stats/status/journal/sevolver` 共享同一份存储口径
- `sevolver` 已负责 skill 生命周期维护、gap 聚合、deep-analysis Issue 升级、关闭原因回填与收敛回写
- `status` 与 `stats` 需要同时保留人类可读输出和 JSON 输出，并显式展示 `sevolver` / `sevolver_deep_analysis` 聚合
- 调度后端以 `systemd --user` 优先，可降级到受控 `cron` 或 `none`

## 全局底线

- 未拍板问题先回 Issue 讨论，禁止脑补
- 注释、文档、报告统一中文
- 工程报告必须写入 `docs/reports/`
- 文件命名遵循 `yymmdd_[Issue No.]_[Case Summary].md`
- 升级程序时禁止覆盖用户本体仓库记忆内容
- `ccclaw` 无参数与 `-h|--help` 显示帮助，`-V|--version` 显示版本
- 非 `maintain` 成员 Issue 仅在受信任成员评论 `/ccclaw <批准词>` 后放行，且最新评论可显式否决
