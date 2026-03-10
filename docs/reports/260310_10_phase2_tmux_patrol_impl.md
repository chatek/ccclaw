# 260310_10_phase2_tmux_patrol_impl

## 背景

在 Phase 1 已完成 Claude JSON 结果解析、`token_usage` 落库和 `ccclaw stats` 基础统计后，Issue #10 的下一阶段是把执行模型从“同步阻塞等待 Claude 退出”升级为“tmux 异步 launch + patrol 巡查收尾”。

本轮目标限定在 Phase 2：

- 新增 tmux 会话管理层
- `run` 改为优先通过 tmux 启动任务
- 新增 `ccclaw patrol`
- 补齐 patrol 的 systemd timer/service
- 安装与升级链路同步纳入 tmux / patrol

本轮仍不进入 Phase 3 的 `--resume`、prompt 归档、journal、rtk 对比统计。

## 实现内容

### 1. 新增 tmux 管理层

新增 `src/internal/tmux/manager.go`：

- 封装 tmux `Launch / Status / List / CaptureOutput / Kill`
- 统一解析：
  - `session_name`
  - `session_created`
  - `pane_dead`
  - `pane_dead_status`
  - `pane_pid`
- 对 `no server running` / `can't find session` 做了显式错误收口，便于 runtime 区分“会话缺失”与“命令失败”

### 2. executor 切换为“tmux 优先，缺失时降级同步”

修改 `src/internal/executor/executor.go`：

- `Executor` 新增：
  - `resultDir`
  - `tmuxManager`
- 当本机存在 `tmux` 时：
  - `Run()` 不再同步等待 Claude 完成
  - 改为为 task 生成稳定的：
    - `SessionName`
    - `LogFile`
    - `ResultFile`
  - 通过 tmux 后台 launch：
    - `pipe-pane` 持续落日志
    - `bash -lc 'set -o pipefail; ... | tee result.json'`
  - 返回 `Pending=true`
- 当本机没有 `tmux` 时：
  - 自动降级为 Phase 1 的同步执行路径

这保证了：

- 新环境可获得 tmux 巡查能力
- 旧环境不会因为缺失 tmux 而彻底失去执行能力

### 3. `run` 改为“launch，不收尾”

修改 `src/internal/app/runtime.go`：

- `run` 对于 tmux 路径：
  - 把任务置为 `RUNNING`
  - 记录 `STARTED`
  - 记录“已挂入 tmux 会话”的事件
  - 不再立即写 DONE/FAILED
- 同步降级路径仍保留原先的即时收尾逻辑

因此现在的运行模型变为：

1. `ccclaw run` 发射任务
2. 任务在 tmux 会话中后台执行
3. `ccclaw patrol` 负责回收结果并更新最终状态

### 4. 新增 `ccclaw patrol`

修改 `src/cmd/ccclaw/main.go` 与 `src/internal/app/runtime.go`：

- 新增 `ccclaw patrol`
- 巡查逻辑按运行中任务逐个匹配对应 tmux session

已落地行为：

1. session 已结束
- 读取 result JSON
- 解析结果
- 写入 `token_usage`
- 更新 task 为 `DONE` 或 `FAILED`
- 清理 tmux session

2. session 运行中但超时
- 抓取最近 pane 输出
- 终止 tmux session
- 将任务记为 `DEAD`

3. session 丢失且没有结果文件
- 将任务记为 `DEAD`

4. session 接近超时
- 写入 `WARNING` 事件

5. 巡查孤儿 session
- 对没有对应运行中 task 的 `ccclaw-*` session 直接清理

### 5. doctor / systemd / install 同步收口

本轮把外围链路一并补齐，避免“代码依赖 tmux，安装却不知道”：

- `doctor` 新增 `tmux CLI` 检查
- user systemd 单元集扩充为：
  - `ccclaw-ingest.*`
  - `ccclaw-run.*`
  - `ccclaw-patrol.*`
- `src/dist/install.sh`
  - preflight 与依赖安装把 `tmux` 纳入必装工具
  - 安装摘要与后续启用提示改为包含 `ccclaw-patrol.timer`
- `src/dist/upgrade.sh`
  - 重启提示改为同时包含 `ccclaw-patrol.timer`
- `src/ops/systemd/` 与 `src/dist/ops/systemd/`
  - 新增 `ccclaw-patrol.service`
  - 新增 `ccclaw-patrol.timer`

## 测试与验证

新增/扩展测试：

- `src/internal/executor/executor_test.go`
  - 保留同步 JSON 解析测试
  - 新增 tmux launch 返回 `Pending` 测试
- `src/internal/app/runtime_test.go`
  - 新增 dead tmux session 经 `patrol` 收尾为 `DONE` 的测试
- 原有 SQLite / stats 测试继续通过

执行验证：

```bash
/usr/local/go/bin/go test ./...
make -C src dist-sync GO=/usr/local/go/bin/go GOFMT=/usr/local/go/bin/gofmt
bash src/tests/install_regression.sh
```

结果：

- Go 单测全部通过
- `dist/ops` 已同步新增 patrol unit
- `install.sh` 回归测试全部通过

## 本轮未做

仍留在 Phase 3 之后：

- `--resume`
- 基于 `last_session_id` 的恢复重试
- prompt 归档
- `ccclaw journal`
- rtk 前后对比统计

## 结论

本轮已把 #10 的 Phase 2 核心增强落地：

- `ccclaw run` 已具备 tmux 异步执行能力
- `ccclaw patrol` 已能完成任务收尾与超时巡查
- 安装、升级、systemd、doctor 已和 tmux/patrol 依赖保持一致

后续进入 Phase 3 时，可以直接围绕现有 `last_session_id` 和 patrol 数据面追加恢复与 journal，不需要再回头重构执行模型。
