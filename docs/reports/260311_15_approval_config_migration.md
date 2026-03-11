# Issue #15 审批配置迁移收口

## 背景

上一轮已完成审批语义升级：

- `approval.command` 废弃
- 新配置改为 `approval.words` / `approval.reject_words`
- 默认门槛调整为 `maintain`

但直接升级后，旧安装中的 `config.toml` 仍可能保留：

```toml
[approval]
command = "/ccclaw approve"
minimum_permission = "admin"
```

这类配置会在启动期直接失败。如果只返回通用 TOML 解析错误，迁移成本仍然偏高。

## 本轮改动

### 1. 为旧配置增加定向报错

在 `src/internal/config/config.go` 中新增旧字段探测：

- 加载前先检查 `[approval]` 段是否仍含 `command`
- 若命中，则直接返回带迁移指引的错误
- 错误文案明确提示执行 `ccclaw config migrate-approval`

这样用户不会再只看到抽象的 “字段不存在 / 解析失败”。

### 2. 新增迁移命令

在 `src/cmd/ccclaw/main.go` 中新增：

```bash
ccclaw config migrate-approval
```

行为：

- 扫描 `config.toml` 的 `[approval]` 配置段
- 若仍使用旧 `command` 字段，则改写为：
  - `minimum_permission`
  - `words`
  - `reject_words`
- 已有 `minimum_permission` 原值保留，不在迁移时擅自降权
- 已经迁移过的配置不会重复改写

### 3. README 补充迁移说明

同步更新：

- `README.md`
- `README_en.md`

新增“旧审批配置迁移”小节，给出旧配置示例和一条可直接执行的迁移命令。

## 验证

执行：

```bash
cd /opt/src/ccclaw/src
go test ./...
```

结果：通过。

新增覆盖：

- `Load()` 命中旧字段时返回迁移提示
- `MigrateLegacyApproval()` 正确改写审批段
- 已迁移配置重复执行时 no-op
- CLI `config migrate-approval` 可直接改写测试配置文件

## 取舍

- 迁移命令当前采用“按段改写”而不是完整 TOML AST 保真编辑，因此 `[approval]` 段内原有注释不会保留
- 该取舍的目的是先提供稳定、低依赖、可立即执行的迁移入口
- 历史 `docs/reports` / `docs/rfcs` 中保留旧门禁描述，视为历史记录，不在本轮篡改
