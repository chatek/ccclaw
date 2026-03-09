# 260308_3_phase0_bootstrap

## 背景

Issue #3 已拍板 `phase0` 的核心决策：

- 单一 `ccclaw` CLI
- 源码全部收敛到 `src/`
- 配置统一使用 `.toml`
- 敏感配置固定落在 `.env`
- 状态后端切换为 SQLite
- 普通用户 Issue 只有管理员评论 `/ccclaw approve` 后才可执行
- 管理员身份通过 GitHub 动态权限判断
- 默认部署方式保持 `systemd`

## 本次完成

### 目录重构

- 根目录收敛为说明文档与开发文档
- 所有源码迁入 `src/`
- 新增 `src/dist/` 作为安装与发布目录树
- 新增 `src/ops/` 保存部署资产源文件

### CLI 与配置

- 新增单一二进制入口：`src/cmd/ccclaw/main.go`
- 提供 `ingest / run / status / doctor / config` 子命令
- 配置改为 `TOML + 固定 .env`
- `.env` 增加严格权限校验：必须 `0600` 或更严格，且禁止符号链接

### 状态与审计

- 新增 SQLite 存储：`src/internal/adapters/storage/sqlite.go`
- 建立 `tasks` 与 `task_events` 两张表
- 记录入队、阻塞、启动、失败、完成、死信等事件

### GitHub 门禁

- 新增 `gh api` 适配层：`src/internal/adapters/github/client.go`
- 管理员身份通过仓库协作权限动态判断
- 非管理员 Issue 需要管理员评论 `/ccclaw approve`
- 支持在 ingest/run 时重新同步审批状态

### 运行与部署

- 新增 Claude 执行器封装
- 新增 `doctor` 检查：配置、`.env`、gh CLI、GitHub 网络、SQLite、systemd timer、磁盘空间
- 新增 `src/dist/install.sh` 与 `src/dist/upgrade.sh`
- 新增 systemd service/timer 样板

## 测试

在 `src/` 下执行：

```bash
go mod tidy
go test ./...
make build
```

结果：通过。

## 已知限制

- `doctor` 当前要求目标机器存在并启用了 `ccclaw-ingest.timer` 与 `ccclaw-run.timer`
- `run` 仍假定执行器为 `claude` 二进制，phase0 未扩展多模型 provider
- 当前 target repo 以 `config.toml` 中显式配置为准，尚未引入多仓库自动发现机制

## 后续建议

- 增加去重回帖机制，避免 BLOCKED 状态重复评论
- 为审批命令增加 `/ccclaw reject` 与 `/ccclaw rerun`
- 为 `status` 增加事件回放与单任务详情查询
- 为 `install.sh` 增加 systemd 自动安装与启停选项
- 为多目标仓库补充仓库级权限策略与路由规则
