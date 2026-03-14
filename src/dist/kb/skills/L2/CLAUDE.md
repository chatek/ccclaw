# CLAUDE.md

<!-- ccclaw:managed:start -->
本目录记录组合级技巧与稳定工作流。

## 适用内容

- 跨工具、跨阶段的完整流程
- 面向交付结果的操作剧本、巡检套路、升级套路
- 需要前置判断、分支决策与验收标准的复合技能

## 组织方式

- 每份文档都应链接回所依赖的 L1 技巧
- 单个工作流建议使用目录封装，并在 `CLAUDE.md` 头部写 YAML frontmatter
- 目录 `summary.md` 汇总高价值工作流、适用边界与替代路径
- 若流程已绑定特定系统或仓库，应显式标注作用域

## 维护原则

- 当底层 L1 更新时，检查对应 L2 是否要同步修订
- 长期稳定的 L2 可以再沉淀到 `designs/` 或根 `summary.md` 作为标准作业路径
- 过时流程保留最小历史记录，并标注失效时间与替代方案

## sevolver 自动维护字段

目录内工作流 Skill 若声明 YAML frontmatter，应允许 `ccclaw-sevolver` 自动维护以下字段并在升级时保留：

```yaml
last_used: YYYY-MM-DD
use_count: 0
status: active
gap_signals: []
gap_escalations: []
```

- `status: dormant` 表示超过 14 天未命中
- `status: deprecated` 表示已移入 `kb/skills/deprecated/`，不再参与默认加载
- `gap_escalations` 记录 gap 被升级为 deep-analysis issue 后的处理状态
- `gap_escalations[].status: escalated` 表示该缺口已升级处理中
- `gap_escalations[].status: converged` 表示关联 deep-analysis issue 已关闭，skill 侧已完成收敛回写
- `gap_escalations[].close_reason` 反映 deep-analysis issue 的关闭结论，决定该工作流应继续收敛、改写还是标记废弃
<!-- ccclaw:managed:end -->

<!-- ccclaw:user:start -->
<!-- 本区块留给本机人工补充；升级会保留这里的内容。 -->
<!-- ccclaw:user:end -->
