# CLAUDE.md — src/internal/adapters 作用域

本目录只放外部系统适配器。

## 约束

- 所有 GitHub、SQLite、reporter 交互都要保留可审计错误信息
- 外部命令、网络请求、数据库操作都必须可超时、可失败回传
- 不在适配器层隐式吞掉权限、签名、发布相关错误
