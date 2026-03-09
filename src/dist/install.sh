#!/usr/bin/env bash
set -euo pipefail

PREFIX="${PREFIX:-/opt/ccclaw}"
BIN_TARGET="${BIN_TARGET:-/usr/local/bin/ccclaw}"
DIST_DIR="$(cd "$(dirname "$0")" && pwd)"

mkdir -p "$PREFIX/bin" "$PREFIX/ops/config" "$PREFIX/ops/systemd" "$PREFIX/ops/scripts" "$PREFIX/kb" /var/lib/ccclaw /var/log/ccclaw

if [[ ! -x "$DIST_DIR/bin/ccclaw" ]]; then
  echo "未找到 $DIST_DIR/bin/ccclaw，请先在 src/ 下执行 make build" >&2
  exit 1
fi

install -m 755 "$DIST_DIR/bin/ccclaw" "$PREFIX/bin/ccclaw"
cp -R "$DIST_DIR/kb/." "$PREFIX/kb/"
if [[ ! -f "$PREFIX/ops/config/config.toml" ]]; then
  install -m 644 "$DIST_DIR/ops/config/config.example.toml" "$PREFIX/ops/config/config.toml"
fi
install -m 644 "$DIST_DIR/ops/systemd/ccclaw-ingest.service" "$PREFIX/ops/systemd/ccclaw-ingest.service"
install -m 644 "$DIST_DIR/ops/systemd/ccclaw-ingest.timer" "$PREFIX/ops/systemd/ccclaw-ingest.timer"
install -m 644 "$DIST_DIR/ops/systemd/ccclaw-run.service" "$PREFIX/ops/systemd/ccclaw-run.service"
install -m 644 "$DIST_DIR/ops/systemd/ccclaw-run.timer" "$PREFIX/ops/systemd/ccclaw-run.timer"
install -m 755 "$DIST_DIR/ops/scripts/healthcheck.sh" "$PREFIX/ops/scripts/healthcheck.sh"

if [[ ! -f "$PREFIX/.env" ]]; then
  install -m 600 "$DIST_DIR/.env.example" "$PREFIX/.env"
fi

ln -sf "$PREFIX/bin/ccclaw" "$BIN_TARGET"

cat <<MSG
安装完成。

- 程序目录: $PREFIX
- 可执行文件: $BIN_TARGET
- 隐私配置: $PREFIX/.env
- 普通配置: $PREFIX/ops/config/config.toml

下一步：
1. 编辑 $PREFIX/.env
2. 编辑 $PREFIX/ops/config/config.toml
3. 将 systemd 单元复制到 /etc/systemd/system/
4. systemctl daemon-reload && systemctl enable --now ccclaw-ingest.timer ccclaw-run.timer
MSG
