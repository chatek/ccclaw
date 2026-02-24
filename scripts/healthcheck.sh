#!/usr/bin/env bash
# scripts/healthcheck.sh — ccclaw 健康检查
set -euo pipefail

STATE_DIR="${CCCLAW_STATE_DIR:-/var/lib/ccclaw}"
LOG_DIR="${CCCLAW_LOG_DIR:-/var/log/ccclaw}"
DEAD_THRESHOLD=3

echo "=== ccclaw 健康检查 $(date -u +%Y-%m-%dT%H:%M:%SZ) ==="

# 检查目录
for dir in "$STATE_DIR" "$LOG_DIR"; do
    if [[ ! -d "$dir" ]]; then
        echo "ERROR: 目录不存在: $dir"
        exit 1
    fi
done

# 统计任务状态
total=$(find "$STATE_DIR" -name "*.json" | wc -l)
dead=$(grep -rl '"state":"DEAD"' "$STATE_DIR" 2>/dev/null | wc -l)
running=$(grep -rl '"state":"RUNNING"' "$STATE_DIR" 2>/dev/null | wc -l)

echo "任务总数: $total"
echo "死信任务: $dead"
echo "运行中:   $running"

# 死信告警
if [[ "$dead" -ge "$DEAD_THRESHOLD" ]]; then
    echo "WARN: 死信任务数 ($dead) 达到阈值 ($DEAD_THRESHOLD)，请人工介入"
    exit 2
fi

# 检查 systemd timer 状态
for timer in ccclaw-ingest.timer ccclaw-run.timer; do
    if systemctl is-active --quiet "$timer" 2>/dev/null; then
        echo "OK: $timer 运行中"
    else
        echo "WARN: $timer 未运行"
    fi
done

echo "=== 健康检查通过 ==="
