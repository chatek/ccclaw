# 260312_NA_version_timezone_generation_fix

## 背景

当前 release `26.03.12.0830` 暴露出版本号生成语义不一致问题：

- 仓库约束要求版本号格式为 `yy.mm.dd.HHMM`
- 项目其它时间语义已默认对齐 `Asia/Shanghai`
- 但 `src/Makefile` 直接使用发布机本地 `date`
- 当发布机位于 `US/Eastern` 时，会生成 `08:30`
- 按项目既有时间基准，这一时刻应表达为 `20:30`

问题根因不是“12 小时/24 小时制格式错误”，而是“版本号生成时区错误”。

## 本轮修复

### 1. Makefile 改为显式脚本生成版本号

将：

```makefile
VERSION ?= $(shell date '+%y.%m.%d.%H%M')
```

改为：

```makefile
VERSION ?= $(shell $(CURDIR)/ops/scripts/version.sh)
```

### 2. 新增统一版本号脚本

新增 `src/ops/scripts/version.sh`：

- 固定 `TZ=Asia/Shanghai`
- 输出格式仍为 `yy.mm.dd.HHMM`

这样无论发布机位于哪个时区，默认版本号都按北京时间生成。

### 3. 增补回归校验

在 `src/tests/install_regression.sh` 增加校验：

- 当调用环境 `TZ=UTC` 时，脚本输出仍应等于 `Asia/Shanghai`
- 当调用环境 `TZ=America/New_York` 时，`dist/ops/scripts/version.sh` 输出仍应等于 `Asia/Shanghai`

同时补充 README 说明：

- 版本时间基准固定为 `Asia/Shanghai`
- 不跟随发布机本地时区

## 验证

已执行：

```bash
cd src
go test ./...
make test-install
```

重点确认：

- 版本脚本在非北京时间环境下仍输出北京时间
- `dist-sync` 后发布树中的脚本也保持同样行为
- 既有安装回归不受影响

## 结论

后续若不手工覆盖 `VERSION=...`，默认生成版本号将统一按北京时间输出。

因此下一次自动生成版本号时：

- 同一真实时刻若北京时间为 `2026-03-12 20:30`
- 产物前缀应为 `ccclaw_26.03.12.2030_...`

而不是此前错误地跟随发布机本地时区生成 `26.03.12.0830`。
