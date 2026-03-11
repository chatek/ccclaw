# Issue #15 审批同义词与否决指令改造

## 背景

Issue #15 最终决策明确要求将审批机制从单一的 `/ccclaw approve` 升级为：

- 前缀固定为 `/ccclaw`
- 支持批准同义词列表
- 支持否决词列表
- 匹配忽略大小写
- 默认权限门槛从 `admin` 下调到 `maintain`
- 最新可信评论可覆盖更早的批准结果

现状实现仍停留在单字符串精确匹配，配置、运行时阻塞提示、发布样例和文档均与最新决策不一致。

## 本次改动

### 1. 配置层升级

将 `approval.command` 直接升级为：

```toml
[approval]
minimum_permission = "maintain"
words = ["approve", "go", "confirm", "批准", "agree", "同意", "推进", "通过", "ok"]
reject_words = ["reject", "no", "cancel", "nil", "null", "拒绝", "000"]
```

同时补充：

- 默认值注入
- `approval.words` 非空校验
- `approval.minimum_permission` 合法值校验
- 通过 `UnmarshalExact` 让旧字段 `approval.command` 显式报错，避免静默兼容造成误判

### 2. GitHub 审批匹配逻辑重写

在 `src/internal/adapters/github/client.go` 中：

- 新增固定前缀常量 `ApprovalPrefix = "/ccclaw"`
- 用 `MatchApprovalCommand` 替代旧的精确字符串判断
- 逐行扫描评论正文
- 仅取 `/ccclaw` 后第一个词作为动作词
- 动作词统一转小写后再匹配批准词/否决词集合
- `FindApproval` 改为返回最后一条可信审批指令的完整信息：
  - 是否批准
  - 是否否决
  - 命令文本
  - 评论人
  - 评论 ID

这样最新评论中的可信否决词可以覆盖更早的批准词。

### 3. 运行时审批语义调整

在 `src/internal/app/runtime.go` 中：

- 发起者本身满足 `maintain` 门槛时，仍默认放行
- 但如果最新评论中出现可信否决词，则会显式转回阻塞
- 如果最新评论中出现可信批准词，则显式通过
- 阻塞原因文案改为新规则说明，不再引用废弃字段 `approval.command`
- 任务元数据中的 `ApprovalCommand` 改为记录实际命中的命令，而不是配置模板值

这保证了“可信作者默认放行”和“管理员可显式撤回批准”两种语义同时成立。

### 4. 发布源与文档同步

同步更新了以下内容，避免运行时行为与用户看到的说明不一致：

- `src/ops/config/config.example.toml`
- `src/dist/ops/config/config.example.toml`
- `src/dist/install.sh`
- `README.md`
- `README_en.md`
- `AGENTS.md`
- `CLAUDE.md`

## 测试

执行：

```bash
cd /opt/src/ccclaw/src
go test ./...
```

结果：全部通过。

另外新增了针对性测试，覆盖：

- 同义词与大小写无关匹配
- 否决词匹配
- `FindApproval` 选择最新评论中的可信命令
- 可信作者被最新评论否决后重新阻塞
- 旧配置字段 `approval.command` 被拒绝加载

## 风险与后续

- 本次按 Issue 决策执行“直接升级”，旧配置文件若仍保留 `approval.command` 将在启动时失败，需要同步迁移
- 否决语义目前只支持“最新评论优先”的单次结论，不保留更细粒度审批历史解释；如后续需要 UI 或审计增强，可再补状态说明
