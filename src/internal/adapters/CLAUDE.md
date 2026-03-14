# CLAUDE.md — src/internal/adapters 作用域

本目录只放外部系统适配器。

## 约束

- 所有 GitHub、SQLite、reporter 交互都要保留可审计错误信息
- 外部命令、网络请求、数据库操作都必须可超时、可失败回传
- 不在适配器层隐式吞掉权限、签名、发布相关错误
- `storage` 适配层是运行态聚合单一事实源；`task_class`、token、event、slot 等统计必须在这里统一口径
- JSONL/hashchain、state store、repo slot store 之间的关系必须保持可回放、可核对、可增量扩展
- GitHub 适配必须保留 deep-analysis Issue 升级、关闭原因读取与回写链路的原始错误上下文
