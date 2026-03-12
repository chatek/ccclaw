# 260312_NA_issue22_tmux_claude_path_forensics

## 背景

2026-03-12，Issue [#22](https://github.com/41490/ccclaw/issues/22) 已在评论 <https://github.com/41490/ccclaw/issues/22#issuecomment-4044618658> 收到 `/ccclaw go` 批准，但随后在评论 <https://github.com/41490/ccclaw/issues/22#issuecomment-4044665414> 被回帖为执行失败：

- 状态：`DEAD`
- 错误：`tmux 会话 ccclaw-41490_ccclaw_22_body-4430 已丢失，且未找到结果文件`

用户要求追查：

1. 批准是否真的生效
2. tmux 启动 Claude 的链路究竟卡在哪里
3. 应该如何修复

本报告只做现场取证、根因定位与处置建议，不在本轮直接改代码。

## 结论摘要

### 1. `issuecomment-4044618658` 的批准已经生效

通过 `gh api repos/41490/ccclaw/issues/22/comments --paginate` 核查，#22 关键时间线如下：

- `2026-03-12T07:32:18Z`
  - 评论 ID: `4044618658`
  - 评论内容：`/ccclaw go`
- `2026-03-12T07:35:17Z`
  - `state.db/task_events`
  - 事件：`状态变更: BLOCKED -> NEW`
- `2026-03-12T07:40:17Z`
  - `state.db/task_events`
  - 事件：`开始执行任务`
- `2026-03-12T07:40:17Z`
  - `state.db/task_events`
  - 事件：`任务已挂入 tmux 会话 ccclaw-41490_ccclaw_22_body-4430`
- `2026-03-12T07:42:15Z`
  - `state.db/task_events`
  - 事件：`tmux 会话 ccclaw-41490_ccclaw_22_body-4430 已丢失，且未找到结果文件`

因此，失败原因不是“admin 批准未被识别”，而是“批准后进入执行，tmux/Claude 启动链路失败”。

### 2. 真正卡点在 tmux pane 内的 `PATH` 收窄，导致 `ccclaude` 找不到 `claude`

当前执行链路为：

```text
run timer
  -> ccclaw run
     -> executor.launchTMux()
        -> tmux new-session
        -> tmux send-keys "bash -lc 'env ... /home/zoomq/.ccclaw/bin/ccclaude ...'"
           -> /home/zoomq/.ccclaw/bin/ccclaude
              -> exec rtk proxy claude "$@"  或  exec claude "$@"
```

现场复现表明：

- 交互 shell 中：
  - `claude` 位于 `/home/zoomq/.local/bin/claude`
  - 当前 `PATH` 包含 `~/.local/bin`
- tmux 新 pane 中：
  - `PATH=/usr/local/bin:/usr/bin:/bin:/usr/local/games:/usr/games:/usr/local/go/bin`
  - 不包含 `~/.local/bin`
  - `command -v claude` 无结果
  - `command -v rtk` 无结果

基于同一份 prompt 做最小复现，tmux pane 直接报出：

```text
/home/zoomq/.ccclaw/bin/ccclaude: line 12: exec: claude: not found
```

这说明 tmux 会话已经启动，但在 `ccclaude -> exec claude` 这一跳被拦住，Claude 进程根本没有真正起来。

### 3. 失败回帖之所以表现为“tmux 会话丢失”，是因为当前收口逻辑没有把这类 stderr-only 失败建模清楚

复现场景下：

- `result.json` 会被 `tee` 创建
- 但大小为 `0`
- 失败信息只出现在 tmux pane/stderr 中
- 当前 `LoadResult()` 只读 `result.json`
- `patrol` 在拿不到有效 JSON 结果时，会退化为“会话丢失/无结果文件”这一类通用死亡原因

因此，Issue #22 上的失败回帖描述的是“收口阶段看到的表象”，不是最前面的真正根因。

## 证据

### 1. GitHub 评论时间线

`gh api repos/41490/ccclaw/issues/22/comments --paginate` 返回：

- `4044618658`
  - `2026-03-12T07:32:18Z`
  - `ZoomQuiet`
  - `/ccclaw go`
- `4044665414`
  - `2026-03-12T07:42:16Z`
  - `ZoomQuiet`
  - 失败回帖：`tmux 会话 ... 已丢失，且未找到结果文件`

### 2. 本地状态库

`sqlite3 ~/.ccclaw/var/state.db` 中：

- `tasks`
  - `41490/ccclaw#22#body`
  - `state = DEAD`
  - `approved = 1`
  - `approval_actor = ZoomQuiet`
  - `approval_comment_id = 4044618658`
- `task_events`
  - `BLOCKED -> NEW`
  - `STARTED`
  - `任务已挂入 tmux 会话`
  - `DEAD`

### 3. tmux 启动日志

`~/.ccclaw/log/41490_ccclaw_22_body.log` 只记录到 tmux 输入的命令，没有任何 Claude JSON 结果。

同时，按同一路径复现时可在 pane 中稳定得到：

```text
/home/zoomq/.ccclaw/bin/ccclaude: line 12: exec: claude: not found
```

### 4. PATH 差异

交互 shell：

- `PATH` 含 `/home/zoomq/.local/bin`
- `command -v claude` 返回 `/home/zoomq/.local/bin/claude`

tmux pane：

- `PATH` 不含 `/home/zoomq/.local/bin`
- `command -v claude` 无结果
- `command -v rtk` 无结果

### 5. 相关实现位置

- `src/internal/executor/executor.go`
  - `buildTMuxCommand()` 把执行器命令包装成 `bash -lc 'env ... /home/zoomq/.ccclaw/bin/ccclaude ... | tee result.json'`
- `src/internal/tmux/manager.go`
  - `Launch()` 采用 `tmux new-session` 后再 `send-keys` 将命令键入 shell
- `/home/zoomq/.ccclaw/bin/ccclaude`
  - 内部再 `exec rtk ...` 或 `exec claude ...`

问题不是 `ccclaw` 找不到 `ccclaude`，而是 `ccclaude` 所依赖的 `claude`/`rtk` 依赖 PATH 查找，而 tmux pane 的 PATH 不完整。

## 根因分析

根因分两层：

### 根因 A：tmux pane 启动出来的 shell PATH 与交互 shell 不一致

当前机器上 `claude` 与 `rtk` 安装在：

- `/home/zoomq/.local/bin/claude`
- `/home/zoomq/.local/bin/rtk`

但 tmux pane 内 PATH 不含该目录，导致：

- `ccclaude` 能启动
- `ccclaude` 内部 `exec claude` 失败
- Claude 主进程根本没起来

### 根因 B：失败收口只依赖 stdout 结果文件，stderr 现场没有进入任务错误模型

当前 tmux 命令只做：

```text
... | tee result.json
```

这意味着：

- 只有 stdout 被写入 `result.json`
- `claude: not found` 这类 stderr-only 错误不会进入结果文件
- `patrol` 最终只能看到“空结果/缺结果”
- Issue 回帖变成“tmux 会话丢失，且未找到结果文件”

所以现在既有“Claude 根本没起来”的真实问题，也有“错误呈现被掩盖”的可观测性问题。

## 与已有问题的关系

本问题与 #31 相关，但不等价。

- #31 已确认 `run/patrol` 定时重叠会造成 `SQLITE_BUSY`
- 本报告进一步坐实：即便不讨论 SQLite 锁冲突，tmux pane 自身也可能因为 PATH 不完整而根本启动不了 Claude

换句话说：

- `SQLITE_BUSY` 会放大执行链路不稳定
- `PATH -> claude not found` 则是 Claude 启动失败的更前置根因

这两个问题应该拆开治理，不能只修一个。

## 建议方案

### 方案 1：执行器初始化时把 `claude` / `rtk` 解析成绝对路径

在 `executor.New()` 或配置装载阶段：

- 对 `ccclaude` 包装器内依赖的 `claude` / `rtk` 做一次 `exec.LookPath`
- 解析成功后：
  - 直接把绝对路径写入环境变量供 `ccclaude` 使用
  - 或在生成 tmux 命令时显式注入完整 PATH

优点：

- 根因修复最直接
- 不依赖用户 shell 启动脚本
- systemd / tmux / cron 三种场景都更稳定

建议优先级：最高。

### 方案 2：改写 `ccclaude`，优先读取显式可配置的 Claude/rtk 二进制路径

例如让 `ccclaude` 支持：

- `CCCLAW_CLAUDE_BIN`
- `CCCLAW_RTK_BIN`

然后：

- 若存在显式路径，则直接 `exec "$CCCLAW_CLAUDE_BIN"` 或 `exec "$CCCLAW_RTK_BIN"`
- 否则才回退到 PATH 查找

优点：

- wrapper 更稳
- 部署时诊断更清晰

建议与方案 1 组合落地。

### 方案 3：tmux 路径改为“直接给 new-session 一个命令”，不要依赖 `send-keys` 注入交互 shell

当前实现是：

```text
tmux new-session -d
tmux send-keys <大段命令>
```

更稳妥的方式是：

```text
tmux new-session -d -s <name> -c <dir> "bash -lc '...'"
```

这样可以：

- 避免交互 shell/prompt 干扰
- 让 pane 生命周期与实际命令生命周期绑定
- `patrol` 可直接用 pane dead / exit code 判断完成

这属于结构性修复，建议单独做。

### 方案 4：tmux 结果采集同时覆盖 stderr

至少应做到以下二选一：

1. `stdout+stderr` 一起落到结构化结果外的诊断文件
2. 当 `result.json` 为空时，把 pane 最后若干行输出拼进失败原因

否则以后仍会重复出现：

- 用户看到的是“会话丢失”
- 真错误却藏在 tmux pane 里

## 建议执行顺序

1. 先修执行器 PATH / 绝对路径问题
   - 确保 tmux 内真正能拉起 `claude`/`rtk`
2. 再修 tmux 启动模型
   - 从 `send-keys` 迁移到 `new-session 直接带命令`
3. 最后补失败收口与回归测试
   - 覆盖 `claude not found`
   - 覆盖 `rtk not found`
   - 覆盖 `result.json` 空文件
   - 覆盖 stderr-only 失败提示

## 验收标准

1. 在 user systemd timer 下，Issue #22 这类低风险任务能够真正启动 Claude，而不是停在 wrapper 层
2. tmux pane 内 `command -v claude` / `command -v rtk` 不再依赖用户手工登录态 PATH
3. 当 `claude` 或 `rtk` 缺失时，Issue 回帖应明确报告：
   - 缺哪个命令
   - 发生在 tmux/执行器启动阶段
4. 不再出现“真实错误是 `claude: not found`，对外却只显示 `tmux 会话丢失`”这种误导性回帖

## 本轮产出

- 已确认 #22 的 admin 批准评论确实生效
- 已确认失败点发生在 `tmux pane -> ccclaude -> exec claude` 这一跳
- 已确认直接阻塞原因为 tmux pane PATH 缺失 `~/.local/bin`
- 已给出不改代码前提下的修复方向与验收标准
