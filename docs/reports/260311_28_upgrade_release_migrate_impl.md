# 260311_28_upgrade_release_migrate_impl

## 背景

承接 Issue #28 的定案：

1. `upgrade.sh` 默认升级源固定为 `41490/ccclaw` 最新官方 release
2. 升级必须强制校验 `SHA256SUMS`
3. `config migrate` 第一版允许按段重写并丢失局部注释
4. 保留 `upgrade.sh` 入口名，只修正其行为

同时，安装链路不再询问控制仓库，统一固定为官方控制面 `41490/ccclaw`。

## 本轮改动

### 1. `upgrade.sh` 改成真正的官方 release 升级器

文件：

- `src/dist/upgrade.sh`

调整后行为：

- 固定通过 `gh release view/download` 拉取 `41490/ccclaw` 最新 release
- 按当前主机平台拼接安装包名
  - 目前支持 `linux/amd64`
  - 同时补了 `linux/arm64` 路径探测
- 强制下载：
  - `ccclaw_<tag>_<goos>_<goarch>.tar.gz`
  - `SHA256SUMS`
- 使用 `sha256sum -c` 校验安装包
- 解压到临时目录后，执行临时 release tree 中的 `install.sh`
- 升级前从现有 `config.toml` 回填：
  - `paths.app_dir`
  - `paths.home_repo`

这样升级入口不再重跑本地旧安装器，也不会把 `home_repo` 错误回退到 `/opt/ccclaw`。

### 2. `install.sh` 固定官方控制仓库，不再交互询问

文件：

- `src/dist/install.sh`

调整：

- `CONTROL_REPO` 固定为 `41490/ccclaw`
- 去掉 `--control-repo` 解析与交互提示
- 安装流程说明改成“普通配置中固定写入官方控制仓库”

这样控制面来源不再漂移，也和 Issue 决策保持一致。

### 3. 旧配置升级改为总迁移 `config migrate`

文件：

- `src/internal/config/config.go`
- `src/cmd/ccclaw/main.go`
- `src/dist/install.sh`

新增：

```bash
ccclaw config migrate
```

本轮总迁移策略：

- 若存在旧 `approval.command`
  - 先迁移为 `words/reject_words`
- 再按当前结构重写完整 `config.toml`
  - 补齐 `scheduler.calendar_timezone`
  - 补齐 `[scheduler.timers]`
  - 补齐 `[scheduler.logs]`
  - 固定 `github.control_repo = "41490/ccclaw"`
- 重复执行幂等

安装器在检测到已有 `config.toml` 时，已从原先的：

```bash
ccclaw config migrate-approval
```

切换为：

```bash
ccclaw config migrate
```

因此旧安装在升级后会自动补齐缺失配置段。

### 4. 为 shell 脚本补齐两处稳定性修复

#### 4.1 修复 `tar | head` 在 `pipefail` 下触发 141

原升级脚本用：

```bash
tar -tzf ... | head -n 1 | cut -d/ -f1
```

在 `set -o pipefail` 下会因为 `tar` 收到 `SIGPIPE` 返回 `141`，导致升级假失败。

现改为：

- 先把 `tar -tzf` 输出落到临时文件
- 再读取首行

#### 4.2 修复 `[[ -n "$value" ]] && ...` 在 `set -e` 下打断旧配置升级

`install.sh` 和 `upgrade.sh` 中原有多处：

```bash
[[ -n "$value" ]] && VAR="$value"
```

在旧配置缺字段时会直接中断脚本。

现已统一改为显式：

```bash
if [[ -n "$value" ]]; then
  VAR="$value"
fi
```

## 文档同步

同步更新：

- `src/dist/README.md`
- `src/ops/examples/app-readme.md`
- `src/ops/config/config.example.toml`
- `src/ops/examples/install-flow.md`

文档现在明确说明：

- `upgrade.sh` 固定从官方 latest release 升级
- `github.control_repo` 固定为 `41490/ccclaw`

## 回归验证

执行：

```bash
cd /opt/src/ccclaw/src
go test ./...
make dist-sync
bash tests/install_regression.sh
```

结果通过。

新增覆盖重点：

- `config migrate` CLI 与内部迁移逻辑
- 旧配置迁移后补齐 scheduler 新段
- `control_repo` 统一收口到官方值
- `upgrade.sh` 通过 fake `gh` 下载官方 release 资产
- 校验 `SHA256SUMS`
- 使用临时 release tree 覆盖安装目录
- 升级后版本变化与配置迁移同时成立

## 结论

Issue #28 报告的两个直接症状已经同时收口：

1. `bash ~/.ccclaw/upgrade.sh` 现在会真正下载并安装最新官方 release
2. 存量 `config.toml` 在升级时会自动补入新配置段，而不是继续停留在旧结构

当前仍保留兼容命令 `config migrate-approval`，但安装/升级默认已统一切到 `config migrate`。
