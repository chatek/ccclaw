# 260310_NA_readme_repo_roles

## 背景

本轮目标是在根 `README.md` 与 `README_en.md` 中，补清楚 `ccclaw` 所涉及的三类仓库角色：

- 控制仓库
- 本体仓库
- 任务仓库

并额外说明：

- 本体仓库 `init|remote|local` 的含义与选择建议
- 任务仓库 `none|remote|local` 的含义与选择建议
- 多任务仓库的配置方式、默认路由和显式路由方法

## 依据

本次文档增补基于当前仓库已落地实现，而不是新的设计设想。

关键事实来源：

1. `src/internal/config/config.go`
   - 当前配置模型明确区分：
     - `github.control_repo`
     - `paths.home_repo`
     - `default_target`
     - `[[targets]]`
2. `src/internal/app/runtime.go`
   - 运行时已实现 `target_repo:` 显式路由
   - 未显式指定时，走 `default_target`
   - 若存在 enabled target 但未设置 `default_target`，任务进入阻塞，而不是猜测
3. `src/cmd/ccclaw/main.go`
   - 已提供：
     - `ccclaw target list`
     - `ccclaw target add`
     - `ccclaw target disable`
4. `docs/reports/260309_5_phase0_2_target_gate_install.md`
   - 已确认安装阶段允许 `targets = []`
   - 已确认安装后通过命令追加 target 绑定
5. `docs/reports/260310_7_phase0_4_first_release_delivery.md`
   - 已确认本体仓库支持 `init|remote|local`
   - 已确认任务仓库支持 `none|remote|local`

## 本次补充内容

### 1. 补“控制仓库”的角色说明

README 现在明确说明：

- 控制仓库是控制平面入口
- Issue、Comment、审批命令、审计记录都发生在这里
- 控制仓库不一定等于实际被修改的任务仓库

这样可以避免用户把“发任务的仓库”和“真正干活的仓库”混成同一个概念。

### 2. 补“本体仓库”的存在理由

README 现在明确说明：

- 本体仓库不是程序树
- 本体仓库也不是业务代码仓库
- 它存在是为了保存 `kb/` 和记忆类 `docs/`
- 程序树与记忆树分离，是为了保证升级不覆盖长期记忆

### 3. 补“模式选择建议”

README 现在把模式解释从“只列枚举值”提升为“带使用场景的选择指南”：

- 本体仓库：
  - `init`
  - `remote`
  - `local`
- 任务仓库：
  - `none`
  - `remote`
  - `local`

这样用户不需要先读安装脚本，也能理解为什么存在这些模式。

### 4. 补“多任务仓库配置”

README 现在新增：

- 多 `[[targets]]` 的角色说明
- `default_target` 的意义
- `target_repo:` 的显式路由写法
- 多 target 的命令行追加示例
- `config.toml` 的多 target 样例
- `disabled = true` 的当前语义

### 5. 中英文同步

新增内容已同时落入：

- `README.md`
- `README_en.md`

并保持同一技术口径，避免中英文文档对同一功能给出不同说法。

## 结果

经过本轮补充，README 对三类仓库的解释已经更接近真实运维模型：

- 控制仓库负责接任务与审计
- 本体仓库负责记忆与沉淀
- 任务仓库负责实际执行对象

同时，多仓库场景下的关键行为也已经在 README 中明确：

- 不写 `target_repo:` 时会依赖 `default_target`
- 多个启用 target 且不设默认值时，任务会阻塞
- 可通过 `target add/list/disable` 逐步维护目标仓库集合

## 后续建议

1. 若后续补上 `target remove`，README 中对应章节也应同步更新
2. 若未来引入“按 label/目录/规则自动路由”，应单独增加“高级路由”章节，而不要混入当前 phase0 的简单规则
