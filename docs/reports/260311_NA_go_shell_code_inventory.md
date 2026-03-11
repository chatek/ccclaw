# 260311_NA_go_shell_code_inventory

## 背景

本轮对仓库当前可追踪的 Go 与 Shell 有效代码进行一次盘点，只统计代码行，不统计注释、空行与配置文件内容，并将结果固化为阶段报告。

## Issue 决策核查

2026-03-11 使用 `gh issue list --state all --limit 50` 核查现有议题，未发现与“仓库 Go/Shell 代码量盘点”直接对应的已拍板 Issue。

因此本次按 `NA` 现状盘点报告处理，不引入新的实现决策。

## 工具与口径

### 使用工具

- 工具：`scc v3.7.0`
- 安装位置：`~/.local/bin/scc`
- 安装命令：

```bash
mkdir -p "$HOME/.local/bin"
GOBIN="$HOME/.local/bin" go install github.com/boyter/scc/v3@latest
```

### 统计口径

- 仅统计 Git 已跟踪的 `*.go` 与 `*.sh`
- 排除 `src/.release/**` 中的历史 release 展开物与归档副本
- 排除 `src/dist/bin/ccclaw` 这类二进制产物
- 不统计配置文件，例如 `.toml`、`.env`、systemd 单元等
- 采用 `scc` 输出中的 `Code` 列，天然排除注释与空行
- 测试代码不剔除，但在结果中单独分组说明

### 复核命令

仓库维护口径：

```bash
"$HOME/.local/bin/scc" \
  --include-ext go,sh \
  --exclude-dir .git,src/.release,src/dist/bin \
  --no-cocomo --no-complexity --no-size .
```

逐文件明细口径：

```bash
files=$(git ls-files '*.go' '*.sh')
"$HOME/.local/bin/scc" \
  --by-file --format csv-stream \
  --no-cocomo --no-complexity --no-size \
  $files
```

去重有效口径：

```bash
"$HOME/.local/bin/scc" \
  --include-ext go,sh \
  --exclude-dir .git,src/.release,src/dist/bin \
  --no-cocomo --no-complexity --no-size \
  --no-duplicates .
```

## 统计结果

### 1. 仓库维护口径

这是“当前仓库实际维护了多少 Go/Shell 代码文件”的口径。

| 语言 | 文件数 | 有效代码行 |
| --- | ---: | ---: |
| Go | 24 | 8329 |
| Shell | 5 | 2278 |
| 合计 | 29 | 10607 |

### 2. 去重有效口径

这是“按内容去重后，真正独立的有效代码规模”的口径。

| 语言 | 文件数 | 有效代码行 |
| --- | ---: | ---: |
| Go | 24 | 8329 |
| Shell | 4 | 2275 |
| 合计 | 28 | 10604 |

唯一被去重的重复文件对为：

- `src/ops/scripts/healthcheck.sh`
- `src/dist/ops/scripts/healthcheck.sh`

两者内容一致，重复影响为 `3` 行 Shell 有效代码。

### 3. 测试与非测试拆分

| 分类 | 语言 | 文件数 | 有效代码行 |
| --- | --- | ---: | ---: |
| 非测试 | Go | 15 | 5895 |
| 非测试 | Shell | 4 | 1629 |
| 测试 | Go | 9 | 2434 |
| 测试 | Shell | 1 | 649 |

按仓库维护口径汇总：

- 非测试有效代码：`7524` 行
- 测试有效代码：`3083` 行

## 逐文件明细

以下明细按有效代码行从高到低排序。

| 语言 | 文件 | 分类 | 有效代码行 |
| --- | --- | --- | ---: |
| Go | `src/internal/app/runtime.go` | 非测试 | 1997 |
| Shell | `src/dist/install.sh` | 非测试 | 1600 |
| Go | `src/internal/app/runtime_test.go` | 测试 | 1035 |
| Go | `src/internal/adapters/storage/sqlite.go` | 非测试 | 984 |
| Shell | `src/tests/install_regression.sh` | 测试 | 649 |
| Go | `src/internal/config/config.go` | 非测试 | 496 |
| Go | `src/internal/executor/executor.go` | 非测试 | 484 |
| Go | `src/cmd/ccclaw/main_test.go` | 测试 | 467 |
| Go | `src/cmd/ccclaw/main.go` | 非测试 | 429 |
| Go | `src/internal/adapters/storage/sqlite_test.go` | 测试 | 300 |
| Go | `src/internal/scheduler/status.go` | 非测试 | 285 |
| Go | `src/internal/adapters/github/client.go` | 非测试 | 254 |
| Go | `src/internal/config/config_test.go` | 测试 | 246 |
| Go | `src/internal/memory/index.go` | 非测试 | 216 |
| Go | `src/internal/tmux/manager.go` | 非测试 | 199 |
| Go | `src/internal/scheduler/cron.go` | 非测试 | 189 |
| Go | `src/internal/scheduler/systemd.go` | 非测试 | 148 |
| Go | `src/internal/executor/executor_test.go` | 测试 | 130 |
| Go | `src/internal/core/task.go` | 非测试 | 124 |
| Go | `src/internal/adapters/github/client_test.go` | 测试 | 102 |
| Go | `src/internal/memory/index_test.go` | 测试 | 76 |
| Go | `src/internal/scheduler/cron_test.go` | 测试 | 61 |
| Go | `src/internal/scheduler/use.go` | 非测试 | 56 |
| Go | `src/internal/adapters/reporter/reporter.go` | 非测试 | 26 |
| Shell | `src/dist/upgrade.sh` | 非测试 | 23 |
| Go | `src/internal/core/task_test.go` | 测试 | 17 |
| Go | `src/internal/buildinfo/version.go` | 非测试 | 8 |
| Shell | `src/dist/ops/scripts/healthcheck.sh` | 非测试 | 3 |
| Shell | `src/ops/scripts/healthcheck.sh` | 非测试 | 3 |

## 结论

如果按“当前仓库维护规模”回答，本仓库现有 Go 与 Shell 有效代码总量为 `10607` 行。

如果按“去重后的独立有效代码”回答，本仓库现有 Go 与 Shell 有效代码总量为 `10604` 行。

建议后续将 `scc` 作为开发机常备工具保留；若需要常态化盘点，可再补一个 `src/Makefile` 统计目标，把本报告中的口径直接固化为命令入口。
