# CLAUDE.md

<!-- ccclaw:managed:start -->
本目录记录原子级技巧。

## 适用内容

- 单条命令、短脚本、最小排障步骤
- 清晰输入输出、前置条件、失败信号与回滚动作
- 能在几分钟内独立复用的操作卡片

## 组织方式

- 文件名优先体现动作与对象
- 单个技巧建议使用目录封装，并在 `CLAUDE.md` 头部写 YAML frontmatter
- 目录 `summary.md` 汇总高频技巧、危险操作与常见组合入口
- 当同类技巧超过数个时，按主题拆子目录，并补各自 `summary.md`

## 升层条件

- 同一类 L1 技巧稳定串联后，抽象到 `../L2/`
- 升层后在原文档或 `summary.md` 中补回链，保证检索路径清晰

## sevolver 自动维护字段

目录内 Skill 若带 YAML frontmatter，应允许 `ccclaw-sevolver` 自动维护以下字段并在升级时保留：

```yaml
last_used: YYYY-MM-DD
use_count: 0
status: active
gap_signals: []
```

- `status: dormant` 表示超过 14 天未命中
- `status: deprecated` 表示已移入 `kb/skills/deprecated/`，不再作为活动 Skill 加载
<!-- ccclaw:managed:end -->

<!-- ccclaw:user:start -->
<!-- 本区块留给本机人工补充；升级会保留这里的内容。 -->
<!-- ccclaw:user:end -->
