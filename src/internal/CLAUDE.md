# CLAUDE.md — src/internal 作用域

本目录只放运行时内部实现。

## 约束

- 配置解析必须区分敏感项与普通项
- 所有外部命令调用必须带超时
- 权限判断必须可追溯
- doctor 检查应覆盖 Claude 能力依赖，不只检查程序本身
- 运行时只认固定 `.env` 文件，不依赖调用者预先导出的环境变量
- 版本号通过构建注入，运行时不得私自生成伪版本
- 默认执行模式固定为 `daemon`；`tmux` 相关分支只在显式配置或补偿/排障路径中生效
- 执行结果处理必须优先消费 `stream-json` 事实产物，并保证 `event/result/meta` 兼容链路可追溯
- `stats/status/journal/sevolver` 共享同一份存储与事件口径，新增聚合应优先下沉到存储层单点实现
- `sevolver` 相关实现必须保留 gap 升级、close_reason 回写、skill frontmatter 收敛等可审计字段
