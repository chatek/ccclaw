#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${APP_DIR:-$HOME/.ccclaw}"
exec "$APP_DIR/bin/ccclaw" doctor --config "$APP_DIR/ops/config/config.toml" --env-file "$APP_DIR/.env"
