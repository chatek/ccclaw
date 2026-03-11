# Issue #27 深化 Issue 驱动规约实现报告

## 决策来源

- Issue：<https://github.com/41490/ccclaw/issues/27>
- 拍板评论：<https://github.com/41490/ccclaw/issues/27#issuecomment-4039760618>

本轮按拍板执行以下范围：

1. 术语最小变更：
   - 保留“控制仓库”
   - 原“本体仓库”统一升级为“知识仓库”
   - “任务仓库”维持现名
2. Issue 入口范围：
   - 采用模式 B
   - 控制仓库与所有已绑定任务仓库都可承载带 `ccclaw` 标签的 Issue
   - 允许控制仓库与某个任务仓库重叠
3. 否决策略：
   - 只转 `BLOCKED`
   - 不自动关闭 Issue
4. 标签移除策略：
   - 移除 `ccclaw` 标签后转 `BLOCKED`
   - 重新加标签后恢复判定

## 实现内容

### 1. 运行时升级为多仓库 ingress

修改文件：

- `src/internal/app/runtime.go`
- `src/internal/adapters/github/client.go`
- `src/internal/adapters/reporter/reporter.go`
- `src/internal/core/task.go`
- `src/internal/adapters/storage/sqlite.go`

关键变化：

1. `ingest` 不再只轮询 `github.control_repo`
   - 改为轮询：
     - `github.control_repo`
     - 所有启用的 `[[targets]].repo`
   - 按 repo 去重
2. `run`、审批判定、评论回写都改为基于 Issue 实际所在仓库执行
3. 任务唯一键从“仅 issue number”升级为：
   - `issue_repo#issue_number#body`
4. `tasks` 表新增 `issue_repo`
   - 启动时自动补列
   - 老数据自动回填为 `control_repo`

### 2. 标签门禁升级为全生命周期规则

修改文件：

- `src/internal/app/runtime.go`

关键变化：

1. `syncIssue()` 新增标签阻塞条件
   - 缺少 `github.issue_label` 时直接 `BLOCKED`
2. `ingest` 新增“已存在任务回查”
   - 若某任务所属 repo 本轮未出现在带标签列表中
   - 则再次直接读取该 Issue
   - 从而识别：
     - 标签被移除
     - Issue 被关闭
3. `BLOCKED` 任务在标签恢复后可重新进入判定

### 3. 路由规则调整

修改文件：

- `src/internal/app/runtime.go`
- `src/internal/app/runtime_test.go`

运行时路由顺序改为：

1. Issue 正文中显式 `target_repo:` 优先
2. 否则，若 Issue 所在仓库本身就是启用 target，则直接路由到该仓库
3. 否则使用 `default_target`
4. 再不满足则阻塞

这样可以兼容：

- 控制仓库承载任务入口
- 任务仓库直接承载任务入口
- 控制仓库与任务仓库重叠
- 跨仓库显式路由

### 4. 文档与配置注释对齐

修改文件：

- `README.md`
- `README_en.md`
- `src/ops/config/config.example.toml`
- `src/ops/examples/install-flow.md`
- `src/ops/examples/release-notes-template.md`
- `src/internal/config/config.go`

对齐内容：

1. 原“本体仓库”统一升级为“知识仓库”
2. README 中补充：
   - 控制仓库不是唯一入口
   - 控制仓库与已绑定任务仓库都可贴 `ccclaw` 标签触发
   - 标签移除后进入 `BLOCKED`
3. 配置样例与生成注释同步说明：
   - `ingest` 会按 repo 逐个抓取
   - `issue_label` 被移除后任务会阻塞

## 测试与验证

执行：

```bash
cd /opt/src/ccclaw/src
go test ./...
```

结果：

- 全部通过

本轮补充的关键测试点：

1. `Issue 所在仓库就是 target` 时可直接路由
2. 缺少触发标签时任务进入 `BLOCKED`
3. 可信作者 + 可信否决评论时，否决命令优先于作者权限

## 风险与后续

### 已解决风险

- 不同仓库相同 issue number 的任务主键碰撞
- 评论回写到错误仓库
- 标签仅在 `ingest` 生效、移除后仍继续执行的语义缺口

### 尚未进入本轮

1. 自动关闭 / 自动 reopen Issue
   - 按拍板，继续人工处理
2. 更细粒度的入口仓库白名单配置
   - 当前规则即“控制仓库 + 所有启用 target repo”
3. README 历史章节与历史报告的全仓术语回刷
   - 本轮只修运行文档与配置样例，不改历史记录语义

## 结论

Issue #27 本轮已完成两项核心落地：

1. 将 `ccclaw` 标签从“列表筛选条件”升级为“运行时全生命周期门禁”
2. 将任务入口从“仅控制仓库”扩展为“控制仓库 + 所有已绑定任务仓库”

同时按拍板完成了“本体仓库 -> 知识仓库”的主要文档对齐。
