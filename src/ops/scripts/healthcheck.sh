#!/usr/bin/env bash
set -euo pipefail

ccclaw doctor --config /opt/ccclaw/ops/config/config.toml --env-file /opt/ccclaw/.env
