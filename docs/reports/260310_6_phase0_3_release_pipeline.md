# 260310_6_phase0_3_release_pipeline

## 背景

对应 Issue：#6 `phase0.3: 项目状态清查、治理文件同步与统一 release 发布流`

本轮目标：

1. 将 `src/Makefile` 从基础开发命令升级为统一发布入口
2. 补齐 `ccclaw` 的版本输出与帮助行为
3. 将 release / 安装 / gate / 配置约束同步到根 `AGENTS.md` 与多级 `CLAUDE.md`
4. 让 `src/dist/` 更接近真正可发包安装的 release 树

## 本轮变更

### 1. CLI 版本能力

- 新增 `internal/buildinfo`
- `ccclaw -V|--version` 可输出构建注入版本
- 新增 `ccclaw version`
- `ccclaw` 无参数默认显示帮助

版本格式由 `src/Makefile` 注入，统一为：

```text
yy.mm.dd.HHMM
```

### 2. Makefile 发布流

`src/Makefile` 新增并统一：

- `fmt`
- `build`
- `dist-sync`
- `package`
- `checksum`
- `sign`
- `archive`
- `release`
- `clean`

当前发布约束：

- 默认平台：`linux/amd64`
- release tag：直接使用 `yy.mm.dd.HHMM`
- 产物格式：`tar.gz`
- 校验：`SHA256SUMS`
- 签名：同时要求 `gpg` 与 `minisign`
- 本机归档目录：`/ops/logs/ccclaw/<version>/`

### 3. release 树补强

- 新增 `src/dist/README.md`
- 扩充 `src/dist/.env.example`
  - `ANTHROPIC_BASE_URL`
  - `ANTHROPIC_AUTH_TOKEN`
- `install.sh` 安装完成后新增输出：
  - 当前主机部署成果总表
  - 默认触发方式
  - `systemd --user` 与 `crontab` 样板提示
  - 日常使用流程
  - 开源协作流程

### 4. 治理文件同步

已更新：

- 根 `AGENTS.md`
- 根 `CLAUDE.md`
- `docs/CLAUDE.md`
- `src/CLAUDE.md`
- `src/dist/CLAUDE.md`
- `src/internal/CLAUDE.md`

已新增：

- `src/cmd/CLAUDE.md`
- `src/ops/CLAUDE.md`
- `src/internal/adapters/CLAUDE.md`
- `src/dist/ops/CLAUDE.md`

本轮同步后的重点约束：

- 根目录只保留治理与开发文档
- `src/dist/` 是 release 打包源
- 运行时只认固定 `.env`
- 普通配置只认 `.toml`
- 非管理员 Issue 必须管理员评论 `/ccclaw approve`
- release 需要 `SHA256SUMS + gpg + minisign`

### 5. 版本控制忽略规则

已补 `.gitignore`：

- `src/.release/`
- `src/.build/`
- `*.tar.gz`
- `*.asc`
- `*.minisig`
- `SHA256SUMS`

目标是避免任何二进制与 release 产物进入仓库版本控制。

## 验证

已执行：

```bash
cd src && go test ./...
cd src && make build
cd src && ./dist/bin/ccclaw -V
cd src && ./dist/bin/ccclaw version
cd src && ./dist/bin/ccclaw
cd src && make checksum
bash -n src/dist/install.sh src/dist/upgrade.sh
```

验证结果：

- `go test ./...` 通过
- `make build` 通过
- `ccclaw -V` 输出符合 `yy.mm.dd.HHMM`
- 无参数默认输出帮助
- `make checksum` 可成功生成 `tar.gz` 与 `SHA256SUMS`
- `install.sh` / `upgrade.sh` 语法检查通过

## 当前未完成与环境限制

### 1. `minisign` 未安装

当前开发环境中：

```text
minisign: command not found
```

因此本轮只完成了 `sign` / `release` 目标实现，尚未在本机完成真实签名验证。

### 2. `/ops/logs/ccclaw/` 已补齐

后续已通过 `sudo` 创建并调整属主为 `zoomq:zoomq`：

- `/ops/logs`
- `/ops/logs/ccclaw`

因此归档目录约束已可落地。

### 3. 真正 `gh release create` 尚未执行

原因：

- 本机尚未满足 `minisign` 签名条件

## 后续建议

1. 在发布机安装 `minisign` 并准备密钥文件
2. 为 `/ops/logs/ccclaw/` 配置可写权限
3. 在发布机执行一次完整：
   - `make sign`
   - `make archive`
   - `make release`
4. 追加 release 后回写 Issue #6，记录首个正式 release tag 与资产清单
