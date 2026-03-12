# CLAUDE.md

<!-- ccclaw:managed:start -->
本目录记录可复用技巧与操作套路。

## 分层原则

- `L1/` 存放原子技巧、短流程、命令组合、最小排障步骤
- `L2/` 存放跨主题组合技、稳定工作流、面向结果的完整套路
- 新技巧先落到最小可复用层级，再逐步抽象提升

## 汇总方式

- 各层目录维护自己的 `summary.md`
- 根 `summary.md` 汇总高频技巧、升级提醒、常用入口与跨层链接
- 同一技巧若被多处引用，应在上层 `summary.md` 建唯一索引，避免重复维护

## Skill 载体

- 单个 Skill 统一使用目录承载，入口文件命名为 `CLAUDE.md`
- `CLAUDE.md` 头部使用 YAML frontmatter 记录 `name`、`description`、`keywords`
- `summary.md` 只保留目录级人工汇总，不再承担单个 Skill 的结构化元数据职责

## 定期整理

- 每次复用成功后，补充适用条件、前置依赖、失败信号与退出条件
- 当多个 L1 技巧稳定组合后，抽升到 L2，并在 L1 中回链
- 废弃技巧要显式标注失效原因与替代方案

## sevolver 自动维护字段

以下字段由 `ccclaw-sevolver` 自动维护，升级时需要保留原值，不得手工回填默认值：

```yaml
last_used: YYYY-MM-DD
use_count: 0
status: active
gap_signals: []
```

- `status: dormant` 表示超过 14 天未命中
- `status: deprecated` 表示已移入 `kb/skills/deprecated/`，默认不再加载
<!-- ccclaw:managed:end -->

<!-- ccclaw:user:start -->
<!-- 本区块留给本机人工补充；升级会保留这里的内容。 -->
<!-- ccclaw:user:end -->
