# ccclaw {{VERSION}} 发布说明

## 资产

- 安装包：`{{PACKAGE_FILE}}`
- 校验文件：`{{CHECKSUM_FILE}}`
- 安装入口：`bash install.sh`
- 升级入口：`~/.ccclaw/upgrade.sh`

## 术语对齐

- 控制仓库：负责接收 Issue、评论审批与执行门禁，不等于实际干活仓库。
- 知识仓库：负责保存长期记忆、设计、报告与 `kb/**`，默认路径 `/opt/ccclaw`。
- 任务仓库：通过 `[[targets]]` 绑定的实际工作仓库，可有多个。

## 调度说明

- 默认调度模式仍为 `auto`，优先 `systemd --user`。
- 当 `systemd --user` 不可部署时，安装器会在 `crontab` 可用的前提下自动降级到受控 `cron`。
- 受控 `cron` 仅管理带 `ccclaw` 标记的块，更新与清理不会覆盖用户其它规则。

## 升级影响

- 默认情况下不涉及破坏性迁移。
- 若本版调整了调度行为，请优先执行 `ccclaw scheduler status` 复核当前后端状态。

## 迁移动作

1. 检查 `~/.ccclaw/ops/config/config.toml` 中的 `[scheduler]` 配置是否符合预期。
2. 如需切换后端，优先执行 `ccclaw scheduler use systemd|cron|none`，不要手工混写两套调度。
3. 升级后执行一次 `ccclaw doctor`，确认没有双重调度或失效规则。

## 回滚命令

```bash
~/.ccclaw/bin/ccclaw scheduler status
~/.ccclaw/bin/ccclaw scheduler use none
# 或按需切回
~/.ccclaw/bin/ccclaw scheduler use cron
~/.ccclaw/bin/ccclaw scheduler use systemd
```

## 发布检查

- release 必须从 `src/dist/` 打包。
- release 资产至少包含安装包与 `SHA256SUMS`。
- 升级程序不得覆盖 `/opt/ccclaw` 知识仓库中的用户记忆。
