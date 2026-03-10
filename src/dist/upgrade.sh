#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="${APP_DIR:-$HOME/.ccclaw}"
HOME_REPO="${HOME_REPO:-/opt/ccclaw}"
UPGRADE_PROGRAM="${UPGRADE_PROGRAM:-1}"
UPGRADE_CLAUDE="${UPGRADE_CLAUDE:-0}"
REFRESH_CLAUDE_ASSETS="${REFRESH_CLAUDE_ASSETS:-0}"

echo "升级分轨:"
echo "- 程序发布树: $([[ "$UPGRADE_PROGRAM" == "1" ]] && echo enabled || echo skipped)"
echo "- Claude 安装: $([[ "$UPGRADE_CLAUDE" == "1" ]] && echo enabled || echo skipped)"
echo "- plugins/skills 刷新: $([[ "$REFRESH_CLAUDE_ASSETS" == "1" ]] && echo enabled || echo skipped)"
echo "- 本体仓库保护: 不覆盖用户记忆；仅无损刷新关键 kb/CLAUDE.md 受管区块"

if [[ "$UPGRADE_PROGRAM" == "1" ]]; then
  args=(--yes --app-dir "$APP_DIR" --home-repo "$HOME_REPO")
  if [[ "$UPGRADE_CLAUDE" == "1" ]]; then
    args+=(--install-claude)
  fi
  "$SCRIPT_DIR/install.sh" "${args[@]}"
fi

if [[ "$REFRESH_CLAUDE_ASSETS" == "1" ]]; then
  echo "plugins/skills 刷新依赖 install.sh 中的 Claude 资产配置逻辑；若 Claude 当前不可用，此步会被跳过。"
fi

echo "升级完成。若使用 user systemd，请执行: systemctl --user daemon-reload && systemctl --user restart ccclaw-ingest.timer ccclaw-run.timer"
