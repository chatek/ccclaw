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
echo "- Claude 资产自动刷新: disabled(默认只读策略，已停用)"
echo "- 本体仓库保护: 不覆盖用户记忆；仅无损刷新关键 kb/CLAUDE.md 受管区块"

if [[ "$UPGRADE_PROGRAM" == "1" ]]; then
  args=(--yes --app-dir "$APP_DIR" --home-repo "$HOME_REPO")
  if [[ "$UPGRADE_CLAUDE" == "1" ]]; then
    args+=(--install-claude)
  fi
  "$SCRIPT_DIR/install.sh" "${args[@]}"
fi

if [[ "$REFRESH_CLAUDE_ASSETS" == "1" ]]; then
  echo "REFRESH_CLAUDE_ASSETS=1 已不再触发任何自动 marketplace/plugin/rtk 改动；如需修改 Claude 资产，请手工执行。"
fi

echo "升级完成。若当前会话未能自动直连 user bus，请在登录会话中执行: systemctl --user daemon-reload && systemctl --user restart ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer"
