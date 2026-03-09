# CLAUDE.md — ccclaw 工程规范

本文件为 Claude Code 在此仓库中的专属指导。

每次回复称我：`sysNOTA`
每次自称：`CCoder`

## 项目定位

`ccclaw` 是以 GitHub Issue 为异步入口、以单一 CLI 为执行面、以 systemd 为调度层、以 kb 为长期记忆的长期任务执行系统。

当前 `phase0` 的目标不是做“大而全平台”，而是先把以下能力做扎实：

- 可安装
- 可配置
- 可审计
- 可持续维护 `ccclaw` 自身仓库

## 当前目录约定

```text
ccclaw/
├── CLAUDE.md
├── AGENTS.md
├── LICENSE
├── README.md
├── docs/
│   ├── rfcs/
│   ├── plans/
│   └── reports/
└── src/
    ├── Makefile
    ├── go.mod
    ├── dist/
    │   ├── kb/
    │   ├── install.sh
    │   ├── upgrade.sh
    │   ├── .env.example
    │   ├── bin/
    │   └── ops/
    ├── cmd/
    │   └── ccclaw/
    ├── internal/
    └── ops/
```

## 技术约束

- 主语言：Golang
- CLI 框架：`cobra`
- 配置库：`viper`
- 状态后端：SQLite（`modernc.org/sqlite`）
- 脚本：Bash 优先
- 调度：systemd
- 配置格式：`.toml`
- 敏感配置：固定 `.env` 文件

## 工程底线

- 所有外部调用必须带超时
- 任务状态必须可审计
- 任何权限判断必须有显式来源
- 没有明确决策时，先去 Issue 讨论，不要脑补
- 文档、注释、提交说明统一中文

## phase0 关键决策

- 单一二进制：`ccclaw`
- 普通用户 Issue：只有管理员评论 `/ccclaw approve` 才能执行
- 管理员身份：运行时动态检查 GitHub 仓库权限
- 目录重构：源码全部收敛到 `src/`
- `src/dist/` 作为安装部署目录树与 release 打包源

## 交付要求

- 修改完成后，必须在 `docs/reports/` 记录工程报告
- 文件命名必须遵循：`yymmdd_[Issue No.]_[Case Summary].md`
- 在测试通过后，回 Issue 总结成果与后续优化建议
