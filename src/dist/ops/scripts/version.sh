#!/usr/bin/env bash
set -euo pipefail

# 版本号统一按北京时间生成，避免发布机本地时区影响 tag 与产物命名。
TZ=Asia/Shanghai date '+%y.%m.%d.%H%M'
