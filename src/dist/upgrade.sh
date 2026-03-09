#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
"$SCRIPT_DIR/install.sh"

echo "升级完成；如已安装 systemd 单元，请执行 systemctl daemon-reload 并重启对应 timer。"
