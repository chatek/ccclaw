# ccclaw release tree

本目录是 `ccclaw` 的 release 安装树源。

解压发布包后，默认入口是：

```bash
bash install.sh
```

本目录中：

- `install.sh` 负责交互式安装与首次配置采集
- `upgrade.sh` 负责程序发布树升级
- `bin/ccclaw` 是编译后的主二进制
- `ops/` 保存配置样板、systemd 单元与运维脚本
- `kb/` 提供本体仓库初始化目录树
- `kb/**/CLAUDE.md` 提供记忆记录与整理规约模板，升级时会无损合并
- `SHA256SUMS` 用于 release 资产校验

安装默认目标：

- 程序目录：`~/.ccclaw`
- 本体仓库：`/opt/ccclaw`
- 本体仓库模式：`init|remote|local`
- 任务仓库模式：`none|remote|local`

安装完成后，默认建议：

1. 运行 `~/.ccclaw/bin/ccclaw doctor`
2. 检查本体仓库与任务仓库绑定结果
3. 手工启用 `systemd --user` timer
