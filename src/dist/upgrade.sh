#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="${APP_DIR:-$HOME/.ccclaw}"
HOME_REPO="${HOME_REPO:-/opt/ccclaw}"

echo "本次升级仅覆盖程序发布树与可重建配置，不自动修改本体仓库: $HOME_REPO"
"$SCRIPT_DIR/install.sh" --yes --app-dir "$APP_DIR" --home-repo "$HOME_REPO"

echo "升级完成。若使用 user systemd，请执行: systemctl --user daemon-reload && systemctl --user restart ccclaw-ingest.timer ccclaw-run.timer"
