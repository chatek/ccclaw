#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
INSTALL_SCRIPT="$ROOT_DIR/dist/install.sh"
BIN_PATH="$ROOT_DIR/dist/bin/ccclaw"
WORK_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/ccclaw-install-regression.XXXXXX")"

cleanup() {
  rm -rf "$WORK_ROOT"
}
trap cleanup EXIT

log() {
  printf '[install-regression] %s\n' "$*"
}

fail() {
  printf '[install-regression][FAIL] %s\n' "$*" >&2
  exit 1
}

assert_file_exists() {
  local path="$1"
  [[ -e "$path" ]] || fail "缺少文件: $path"
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  grep -Fq -- "$pattern" "$file" || fail "未匹配到内容: $pattern ($file)"
}

assert_matches() {
  local file="$1"
  local pattern="$2"
  grep -Eq -- "$pattern" "$file" || fail "未匹配到正则: $pattern ($file)"
}

assert_file_missing() {
  local path="$1"
  [[ ! -e "$path" ]] || fail "不应存在文件: $path"
}

assert_not_contains() {
  local file="$1"
  local pattern="$2"
  if grep -Fq -- "$pattern" "$file"; then
    fail "出现了不期望的内容: $pattern ($file)"
  fi
}

assert_eq() {
  local expected="$1"
  local actual="$2"
  local message="$3"
  [[ "$expected" == "$actual" ]] || fail "$message: expected=$expected actual=$actual"
}

assert_cmd_eq() {
  local expected="$1"
  local message="$2"
  shift 2
  local actual
  actual="$("$@")"
  [[ "$expected" == "$actual" ]] || fail "$message: expected=$expected actual=$actual"
}

run_case() {
  local logfile="$1"
  shift
  (
    set -euo pipefail
    "$@"
  ) >"$logfile" 2>&1
}

run_case_with_input() {
  local logfile="$1"
  local input="$2"
  shift 2
  (
    set -euo pipefail
    printf '%s\n' "$input" | "$@"
  ) >"$logfile" 2>&1
}

run_expect_fail() {
  local logfile="$1"
  shift
  if run_case "$logfile" "$@"; then
    fail "命令本应失败但成功: $*"
  fi
}

setup_sandbox() {
  local name="$1"
  local sandbox="$WORK_ROOT/$name"
  mkdir -p "$sandbox/home" "$sandbox/xdg" "$sandbox/bin"
  printf '%s\n' "$sandbox"
}

create_git_repo() {
  local path="$1"
  mkdir -p "$path"
  git -C "$path" init -q
  git -C "$path" config user.name "ccclaw-test"
  git -C "$path" config user.email "ccclaw-test@example.invalid"
  printf 'fixture\n' > "$path/README.md"
  git -C "$path" add README.md
  git -C "$path" commit -q -m "init fixture"
}

create_fake_claude() {
  local dir="$1"
  mkdir -p "$dir"
  cat > "$dir/claude" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
logfile="${CCCLAW_FAKE_CLAUDE_LOG:-}"
if [[ -n "$logfile" ]]; then
  printf '%s\n' "$*" >> "$logfile"
fi
case "${1:-}" in
  --version)
    printf '2.1.72 (Claude Code)\n'
    ;;
  auth)
    if [[ "${2:-}" == "status" && "${3:-}" == "--json" ]]; then
      printf '{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty"}\n'
      exit 0
    fi
    ;;
  plugin)
    if [[ "${2:-}" == "list" ]]; then
      printf 'superpowers@claude-plugins-official\n'
      exit 0
    fi
    if [[ "${2:-}" == "marketplace" && "${3:-}" == "list" ]]; then
      printf 'claude-plugins-official\n'
      exit 0
    fi
    ;;
esac
printf 'unexpected fake claude args: %s\n' "$*" >&2
exit 99
SCRIPT
  chmod 755 "$dir/claude"
}

test_version_script_timezone() {
  local expected
  expected="$(TZ=Asia/Shanghai date '+%y.%m.%d.%H%M')"
  assert_cmd_eq "$expected" "版本号脚本应固定使用北京时间" env TZ=UTC "$ROOT_DIR/ops/scripts/version.sh"
  assert_cmd_eq "$expected" "dist 同步后的版本号脚本应固定使用北京时间" env TZ=America/New_York "$ROOT_DIR/dist/ops/scripts/version.sh"
  log "已通过: 版本号脚本固定使用北京时间"
}

create_fake_gh() {
  local dir="$1"
  mkdir -p "$dir"
  cat > "$dir/gh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
log_file="${CCCLAW_FAKE_GH_LOG:-}"
repo_expected="${CCCLAW_FAKE_GH_REPO:-41490/ccclaw}"
release_dir="${CCCLAW_FAKE_GH_RELEASE_DIR:?}"
tag="${CCCLAW_FAKE_GH_TAG:?}"
if [[ -n "$log_file" ]]; then
  printf '%s\n' "$*" >> "$log_file"
fi
if [[ "${1:-}" == "release" && "${2:-}" == "view" ]]; then
  shift 2
  while (($#)); do
    case "$1" in
      --repo)
        shift
        [[ "${1:-}" == "$repo_expected" ]] || { printf 'unexpected repo: %s\n' "${1:-}" >&2; exit 1; }
        ;;
      --json)
        shift
        ;;
      --jq)
        shift
        ;;
      *)
        printf 'unsupported gh release view args: %s\n' "$*" >&2
        exit 1
        ;;
    esac
    shift || true
  done
  printf '%s\n' "$tag"
  exit 0
fi
if [[ "${1:-}" == "release" && "${2:-}" == "download" ]]; then
  shift 2
  release_tag="${1:-}"
  [[ -n "$release_tag" ]] || { printf 'missing release tag\n' >&2; exit 1; }
  [[ "$release_tag" == "$tag" ]] || { printf 'unexpected release tag: %s\n' "$release_tag" >&2; exit 1; }
  shift
  target_dir=""
  patterns=()
  while (($#)); do
    case "$1" in
      --repo)
        shift
        [[ "${1:-}" == "$repo_expected" ]] || { printf 'unexpected repo: %s\n' "${1:-}" >&2; exit 1; }
        ;;
      --dir)
        shift
        target_dir="${1:-}"
        ;;
      --pattern)
        shift
        patterns+=("${1:-}")
        ;;
      *)
        printf 'unsupported gh release download args: %s\n' "$*" >&2
        exit 1
        ;;
    esac
    shift || true
  done
  [[ -n "$target_dir" ]] || { printf 'missing target dir\n' >&2; exit 1; }
  mkdir -p "$target_dir"
  for pattern in "${patterns[@]}"; do
    cp "$release_dir/$pattern" "$target_dir/$pattern"
  done
  exit 0
fi
printf 'unsupported fake gh args: %s\n' "$*" >&2
exit 1
SCRIPT
  chmod 755 "$dir/gh"
}

create_release_fixture() {
  local dir="$1"
  local version="$2"
  local root="$dir/ccclaw_${version}_linux_amd64"
  mkdir -p "$root"
  cp -R "$ROOT_DIR/dist/." "$root/"
  tar -C "$dir" -czf "$dir/ccclaw_${version}_linux_amd64.tar.gz" "$(basename "$root")"
  (
    cd "$dir"
    sha256sum "ccclaw_${version}_linux_amd64.tar.gz" > SHA256SUMS
  )
}

prepare_dist() {
  [[ -x "$INSTALL_SCRIPT" ]] || fail "缺少安装脚本: $INSTALL_SCRIPT"
  [[ -x "$BIN_PATH" ]] || fail "缺少构建产物: $BIN_PATH"
}

create_fake_systemctl() {
  local dir="$1"
  local mode="${2:-showenv-fail}"
  mkdir -p "$dir"
  case "$mode" in
    showenv-fail)
      cat > "$dir/systemctl" <<'SCRIPT'
#!/usr/bin/env bash
if [[ "${1:-}" == "--user" && "${2:-}" == "show-environment" ]]; then
  printf 'Failed to connect to bus: No medium found\n' >&2
  exit 1
fi
exit 0
SCRIPT
      ;;
    ready)
      cat > "$dir/systemctl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
log_file="${CCCLAW_FAKE_SYSTEMCTL_LOG:-}"
if [[ -n "$log_file" ]]; then
  printf '%s\n' "$*" >> "$log_file"
fi
if [[ "${1:-}" == "--user" && "${2:-}" == "show-environment" ]]; then
  printf 'HOME=%s\n' "${HOME:-/tmp}"
  printf 'XDG_RUNTIME_DIR=%s\n' "${XDG_RUNTIME_DIR:-/run/user/1000}"
  exit 0
fi
exit 0
SCRIPT
      ;;
    *)
      fail "未知 fake systemctl 模式: $mode"
      ;;
  esac
  chmod 755 "$dir/systemctl"
}

create_fake_crontab() {
  local dir="$1"
  local mode="${2:-ready}"
  mkdir -p "$dir"
  case "$mode" in
    ready)
      cat > "$dir/crontab" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
store="${CCCLAW_FAKE_CRONTAB_FILE:?}"
case "${1:-}" in
  -l)
    if [[ -f "$store" && -s "$store" ]]; then
      cat "$store"
      exit 0
    fi
    printf 'no crontab for tester\n' >&2
    exit 1
    ;;
  -)
    cat > "$store"
    exit 0
    ;;
  *)
    printf 'unsupported crontab args: %s\n' "$*" >&2
    exit 1
    ;;
esac
SCRIPT
      ;;
    missing)
      cat > "$dir/crontab" <<'SCRIPT'
#!/usr/bin/env bash
printf 'crontab: command not found\n' >&2
exit 127
SCRIPT
      ;;
    *)
      fail "未知 fake crontab 模式: $mode"
      ;;
  esac
  chmod 755 "$dir/crontab"
}

test_first_install_and_idempotent_reinstall() {
  local sandbox app_dir home_repo task_repo xdg log1 log2 config_file env_file croncfg_file target_count gh_count
  sandbox="$(setup_sandbox first-install)"
  app_dir="$sandbox/app"
  home_repo="$sandbox/home-repo"
  task_repo="$sandbox/task-local"
  xdg="$sandbox/xdg"
  log1="$sandbox/first.log"
  log2="$sandbox/reinstall.log"
  config_file="$app_dir/ops/config/config.toml"
  env_file="$app_dir/.env"
  croncfg_file="$app_dir/croncfg.md"

  create_git_repo "$task_repo"

  run_case "$log1" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --task-repo "41490/task-local" \
      --scheduler none

  assert_file_exists "$app_dir/bin/ccclaw"
  assert_file_exists "$config_file"
  assert_file_exists "$env_file"
  assert_file_exists "$croncfg_file"
  assert_file_missing "$xdg/systemd/user/ccclaw-ingest.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-ingest.timer"
  assert_file_missing "$xdg/systemd/user/ccclaw-run.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-run.timer"
  assert_file_missing "$xdg/systemd/user/ccclaw-patrol.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-patrol.timer"
  assert_file_missing "$xdg/systemd/user/ccclaw-journal.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-journal.timer"
  assert_file_missing "$xdg/systemd/user/ccclaw-archive.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-archive.timer"
  assert_file_missing "$xdg/systemd/user/ccclaw-sevolver.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-sevolver.timer"
  assert_contains "$config_file" 'repo = "41490/task-local"'
  assert_contains "$config_file" "local_path = \"$task_repo\""
  assert_contains "$config_file" 'default_target = "41490/task-local"'
  assert_contains "$env_file" 'GH_TOKEN='
  assert_contains "$croncfg_file" 'CCClaw 自动使用的调度主路径'
  assert_contains "$croncfg_file" '专家手工配置方式'
  assert_contains "$log1" '安装完成。'
  assert_contains "$log1" '请求=none, 生效=none'

  run_case "$log2" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --task-repo "41490/task-local" \
      --scheduler none

  target_count="$(grep -c '^\[\[targets\]\]' "$config_file")"
  gh_count="$(grep -c '^GH_TOKEN=' "$env_file")"
  assert_eq "1" "$target_count" "重复安装后 target 条目数量异常"
  assert_eq "1" "$gh_count" "重复安装后 GH_TOKEN 条目数量异常"
  assert_contains "$log2" "复用已有隐私配置: $env_file"
  assert_contains "$log2" "保留已有普通配置: $config_file"
}

test_home_repo_seed_commit_includes_version_and_ahead_hint() {
  local sandbox app_dir home_repo remote_repo log
  sandbox="$(setup_sandbox home-repo-seed-version)"
  app_dir="$sandbox/app"
  home_repo="$sandbox/home-repo"
  remote_repo="$sandbox/home-remote.git"
  log="$sandbox/install.log"

  create_git_repo "$home_repo"
  git init --bare -q "$remote_repo"
  git -C "$home_repo" branch -M main
  git -C "$home_repo" remote add origin "$remote_repo"
  git -C "$home_repo" push -q -u origin main

  run_case "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      CCCLAW_VERSION="26.03.15.0746" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode local \
      --task-repo-mode none \
      --scheduler none

  assert_eq "seed ccclaw home repo (v26.03.15.0746)" "$(git -C "$home_repo" log -1 --pretty=%s)" "seed commit 应携带版本号"
  assert_contains "$log" "[home_repo ahead 1，建议执行：git -C \"$home_repo\" push]"
}

test_systemd_degrade_preflight() {
  local sandbox readonly_xdg fakebin log
  sandbox="$(setup_sandbox systemd-degrade)"
  readonly_xdg="$sandbox/readonly-xdg"
  fakebin="$sandbox/fakebin"
  log="$sandbox/preflight.log"
  mkdir -p "$readonly_xdg"
  chmod 500 "$readonly_xdg"
  create_fake_crontab "$fakebin" missing

  run_case "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$readonly_xdg" \
      PATH="$fakebin:$PATH" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --preflight-only \
      --home-repo "$sandbox/home-repo" \
      --home-repo-mode init \
      --task-repo-mode none \
      --scheduler auto

  chmod 700 "$readonly_xdg"
  assert_contains "$log" '体检完成，未写入文件。'
  assert_contains "$log" '请求=auto, 生效=none'
  assert_matches "$log" 'dir_not_writable|user_bus_unavailable'
}

test_systemd_preflight_accepts_busless_deploy() {
  local sandbox fakebin log
  sandbox="$(setup_sandbox systemd-busless-deploy)"
  fakebin="$sandbox/fakebin"
  log="$sandbox/preflight.log"
  create_fake_systemctl "$fakebin"

  run_case "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --preflight-only \
      --home-repo "$sandbox/home-repo" \
      --home-repo-mode init \
      --task-repo-mode none \
      --scheduler auto

  assert_contains "$log" '请求=auto, 生效=systemd'
  assert_contains "$log" '未直连 user bus'
  assert_contains "$log" '手工启用 timer'
}

test_systemd_preflight_auto_guides_manual_cron() {
  local sandbox readonly_xdg fakebin crontab_file log app_dir
  sandbox="$(setup_sandbox systemd-auto-manual-cron)"
  readonly_xdg="$sandbox/readonly-xdg"
  fakebin="$sandbox/fakebin"
  crontab_file="$sandbox/crontab.txt"
  log="$sandbox/preflight.log"
  app_dir="$sandbox/home/.ccclaw"
  mkdir -p "$readonly_xdg"
  chmod 500 "$readonly_xdg"
  create_fake_crontab "$fakebin" ready

  run_case "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$readonly_xdg" \
      PATH="$fakebin:$PATH" \
      CCCLAW_FAKE_CRONTAB_FILE="$crontab_file" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --preflight-only \
      --home-repo "$sandbox/home-repo" \
      --home-repo-mode init \
      --task-repo-mode none \
      --scheduler auto

  chmod 700 "$readonly_xdg"
  assert_contains "$log" '请求=auto, 生效=none'
  assert_contains "$log" '安装器不再自动托管 cron'
  assert_contains "$log" "$app_dir/croncfg.md"
  assert_contains "$log" "$app_dir/bin/ccclaw scheduler status --config $app_dir/ops/config/config.toml --env-file $app_dir/.env"
  assert_contains "$log" "*/4 * * * * $app_dir/bin/ccclaw ingest --config $app_dir/ops/config/config.toml --env-file $app_dir/.env"
  assert_contains "$log" "*/2 * * * * $app_dir/bin/ccclaw patrol --config $app_dir/ops/config/config.toml --env-file $app_dir/.env"
  assert_contains "$log" "50 23 * * * $app_dir/bin/ccclaw journal --config $app_dir/ops/config/config.toml --env-file $app_dir/.env"
}

test_explicit_cron_mode_fails_fast() {
  local sandbox fakebin crontab_file log app_dir
  sandbox="$(setup_sandbox explicit-cron-fail)"
  fakebin="$sandbox/fakebin"
  crontab_file="$sandbox/crontab.txt"
  log="$sandbox/preflight.log"
  app_dir="$sandbox/app"
  create_fake_crontab "$fakebin" ready

  run_expect_fail "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      CCCLAW_FAKE_CRONTAB_FILE="$crontab_file" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --preflight-only \
      --app-dir "$app_dir" \
      --home-repo "$sandbox/home-repo" \
      --home-repo-mode init \
      --task-repo-mode none \
      --scheduler cron

  assert_contains "$log" '安装器不再自动托管 cron'
  assert_contains "$log" "$app_dir/croncfg.md"
  assert_not_contains "$log" '写入或更新当前用户受控 crontab 规则'
}

test_legacy_cron_config_fails_fast_on_yes_mode() {
  local sandbox fakebin crontab_file log app_dir config_dir
  sandbox="$(setup_sandbox legacy-cron-config)"
  fakebin="$sandbox/fakebin"
  crontab_file="$sandbox/crontab.txt"
  log="$sandbox/preflight.log"
  app_dir="$sandbox/app"
  config_dir="$app_dir/ops/config"
  create_fake_crontab "$fakebin" ready
  mkdir -p "$config_dir"
  cat > "$config_dir/config.toml" <<'EOF'
[scheduler]
mode = "cron"
EOF

  run_expect_fail "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      CCCLAW_FAKE_CRONTAB_FILE="$crontab_file" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --preflight-only \
      --app-dir "$app_dir" \
      --home-repo "$sandbox/home-repo" \
      --home-repo-mode init \
      --task-repo-mode none

  assert_contains "$log" '安装器不再自动托管 cron'
  assert_contains "$log" "$app_dir/croncfg.md"
}

test_systemd_install_auto_enable_and_restart() {
  local sandbox fakebin app_dir home_repo task_repo xdg log1 log2 config_file readme_file systemctl_log
  sandbox="$(setup_sandbox systemd-auto-enable)"
  fakebin="$sandbox/fakebin"
  app_dir="$sandbox/app"
  home_repo="$sandbox/home-repo"
  task_repo="$sandbox/task-local"
  xdg="$sandbox/xdg"
  log1="$sandbox/install.log"
  log2="$sandbox/reinstall.log"
  config_file="$app_dir/ops/config/config.toml"
  readme_file="$app_dir/README.md"
  systemctl_log="$sandbox/systemctl.log"

  create_git_repo "$task_repo"
  create_fake_systemctl "$fakebin" ready

  run_case "$log1" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      CCCLAW_FAKE_SYSTEMCTL_LOG="$systemctl_log" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --task-repo "41490/task-local" \
      --scheduler systemd

  assert_file_exists "$readme_file"
  assert_contains "$readme_file" 'scheduler logs -f'
  assert_contains "$readme_file" 'scheduler doctor'
  assert_contains "$config_file" 'calendar_timezone = "Asia/Shanghai"'
  assert_contains "$config_file" '[scheduler.timers]'
  assert_contains "$config_file" '[scheduler.logs]'
  assert_contains "$systemctl_log" '--user daemon-reload'
  assert_contains "$systemctl_log" '--user enable --now ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer'

  mkdir -p "$xdg/systemd/user"
  printf '[Unit]\nDescription=legacy\n' > "$xdg/systemd/user/ccclaw-run.service"
  printf '[Unit]\nDescription=legacy\n' > "$xdg/systemd/user/ccclaw-run.timer"

  run_case "$log2" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      CCCLAW_FAKE_SYSTEMCTL_LOG="$systemctl_log" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --task-repo "41490/task-local" \
      --scheduler systemd

  assert_file_missing "$xdg/systemd/user/ccclaw-run.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-run.timer"
  assert_contains "$systemctl_log" '--user disable --now ccclaw-run.timer'
  assert_contains "$systemctl_log" '--user restart ccclaw-ingest.timer ccclaw-patrol.timer ccclaw-journal.timer ccclaw-archive.timer ccclaw-sevolver.timer'
}

test_merge_managed_markdown_preserves_skill_meta_fields() {
  local sandbox template target
  sandbox="$(setup_sandbox skill-meta-preserve)"
  template="$sandbox/dist/kb/skills/L1/demo/CLAUDE.md"
  target="$sandbox/home-repo/kb/skills/L1/demo/CLAUDE.md"

  mkdir -p "$(dirname "$template")" "$(dirname "$target")"
  cat > "$template" <<'EOF'
---
name: demo-skill
description: 新版说明
keywords:
  - demo
status: active
gap_signals: []
gap_escalations: []
---

<!-- ccclaw:managed:start -->
新版受管区
<!-- ccclaw:managed:end -->

<!-- ccclaw:user:start -->
<!-- 默认用户区 -->
<!-- ccclaw:user:end -->
EOF

  cat > "$target" <<'EOF'
---
name: demo-skill
description: 旧版说明
keywords:
  - demo
last_used: 2026-03-10
use_count: 7
status: dormant
gap_signals:
  - gap-2026-03-10
gap_escalations:
  - fingerprint: sg-demo
    status: escalated
    issue_number: 88
    issue_url: https://github.com/41490/ccclaw/issues/88
    gap_ids:
      - gap-2026-03-10
---

<!-- ccclaw:managed:start -->
旧版受管区
<!-- ccclaw:managed:end -->

<!-- ccclaw:user:start -->
本机补充
<!-- ccclaw:user:end -->
EOF

  env \
    CCCLAW_INSTALL_LIB_ONLY=1 \
    bash -c 'set -euo pipefail; install_script="$1"; template_file="$2"; target_file="$3"; set --; source "$install_script"; merge_managed_markdown "$template_file" "$target_file"' \
      _ \
      "$INSTALL_SCRIPT" \
      "$template" \
      "$target"

  assert_contains "$target" 'description: 新版说明'
  assert_contains "$target" '新版受管区'
  assert_contains "$target" '本机补充'
  assert_contains "$target" 'last_used: 2026-03-10'
  assert_contains "$target" 'use_count: 7'
  assert_contains "$target" 'status: dormant'
  assert_contains "$target" 'gap_signals:'
  assert_contains "$target" '  - gap-2026-03-10'
  assert_contains "$target" 'gap_escalations:'
  assert_contains "$target" 'fingerprint: sg-demo'
  assert_contains "$target" 'issue_number: 88'
  assert_not_contains "$target" 'status: active'
  assert_not_contains "$target" 'gap_signals: []'
  assert_not_contains "$target" 'gap_escalations: []'
  assert_eq "1" "$(grep -c '^last_used:' "$target")" "last_used 不应重复"
  assert_eq "1" "$(grep -c '^use_count:' "$target")" "use_count 不应重复"
  assert_eq "1" "$(grep -c '^status:' "$target")" "status 不应重复"
  assert_eq "1" "$(grep -c '^gap_signals:' "$target")" "gap_signals 不应重复"
  assert_eq "1" "$(grep -c '^gap_escalations:' "$target")" "gap_escalations 不应重复"
}

test_interactive_mode_accepts_short_words() {
  local sandbox task_repo log input
  sandbox="$(setup_sandbox interactive-short)"
  task_repo="$sandbox/task-local"
  log="$sandbox/interactive.log"

  create_git_repo "$task_repo"
  git -C "$task_repo" remote add origin https://github.com/41490/task-local.git
  input="$(cat <<EOF

i
$sandbox/home-repo
l
$task_repo

n
EOF
)"

  run_case_with_input "$log" "$input" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --simulate \
      --skip-deps

  assert_contains "$log" '阶段 1/4 安装拓扑'
  assert_contains "$log" '本体仓库来源: init -> '
  assert_contains "$log" '任务仓库绑定: 41490/task-local @ '
  assert_contains "$log" '调度器: 请求=none, 生效=none'
}

test_interactive_mode_accepts_full_words() {
  local sandbox log input
  sandbox="$(setup_sandbox interactive-full)"
  log="$sandbox/interactive.log"
  input="$(cat <<EOF

init
$sandbox/home-repo
none
none
EOF
)"

  run_case_with_input "$log" "$input" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --simulate \
      --skip-deps

  assert_contains "$log" '本体仓库来源: init -> '
  assert_contains "$log" '调度器: 请求=none, 生效=none'
  assert_contains "$log" '计划调度模式: 请求=none, 生效=none'
}

test_local_repo_without_origin_requires_repo() {
  local sandbox task_repo log
  sandbox="$(setup_sandbox local-no-origin)"
  task_repo="$sandbox/task-local"
  log="$sandbox/fail.log"

  create_git_repo "$task_repo"

  run_expect_fail "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --simulate \
      --home-repo "$sandbox/home-repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --scheduler none

  assert_contains "$log" 'local 模式下无法从仓库自动推断 owner/repo，且未提供有效 --task-repo'
}

test_remote_repo_path_must_stay_within_clone_root() {
  local sandbox log clone_root outside_path
  sandbox="$(setup_sandbox remote-outside-root)"
  log="$sandbox/fail.log"
  clone_root="$sandbox/clone-root"
  outside_path="$sandbox/outside/task"

  run_expect_fail "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --simulate \
      --home-repo "$sandbox/home-repo" \
      --home-repo-mode init \
      --task-repo-mode remote \
      --task-repo-remote 41490/ccclaw \
      --task-repo 41490/ccclaw \
      --task-clone-root "$clone_root" \
      --task-repo-path "$outside_path" \
      --scheduler none

  assert_contains "$log" "remote 模式下任务仓库本地路径必须位于固定 clone 入口 $clone_root"
}

test_shell_integration_inject_and_remove() {
  local sandbox app_dir home_repo task_repo bashrc log1 log2 log3 block_count
  sandbox="$(setup_sandbox shell-integration)"
  app_dir="$sandbox/app"
  home_repo="$sandbox/home-repo"
  task_repo="$sandbox/task-local"
  bashrc="$sandbox/home/.bashrc"
  log1="$sandbox/install.log"
  log2="$sandbox/reinstall.log"
  log3="$sandbox/remove.log"

  create_git_repo "$task_repo"
  cat > "$bashrc" <<'RC'
# user custom line
export FOO=bar
RC

  run_case "$log1" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      BASHRC_FILE="$bashrc" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --inject-shell bashrc \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --task-repo "41490/task-local" \
      --scheduler none

  assert_contains "$bashrc" '# user custom line'
  assert_contains "$bashrc" '# >>> ccclaw managed block >>>'
  assert_contains "$bashrc" 'export PATH="'"$sandbox"'/bin:$PATH"'

  run_case "$log2" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      BASHRC_FILE="$bashrc" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --inject-shell bashrc \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --task-repo "41490/task-local" \
      --scheduler none

  block_count="$(grep -c '^# >>> ccclaw managed block >>>$' "$bashrc")"
  assert_eq "1" "$block_count" "重复安装后 shell 受控块数量异常"

  run_case "$log3" \
    env \
      HOME="$sandbox/home" \
      BASHRC_FILE="$bashrc" \
      "$INSTALL_SCRIPT" \
      --remove-shell bashrc

  assert_contains "$bashrc" '# user custom line'
  assert_not_contains "$bashrc" '# >>> ccclaw managed block >>>'
  assert_not_contains "$bashrc" 'export PATH="'"$sandbox"'/bin:$PATH"'
}

test_remove_cron_cleans_managed_block_only() {
  local sandbox fakebin crontab_file app_dir home_repo task_repo log1 log2
  sandbox="$(setup_sandbox cron-cleanup-only)"
  fakebin="$sandbox/fakebin"
  crontab_file="$sandbox/crontab.txt"
  app_dir="$sandbox/app"
  home_repo="$sandbox/home-repo"
  task_repo="$sandbox/task-local"
  log1="$sandbox/install-none.log"
  log2="$sandbox/remove-cron.log"

  create_git_repo "$task_repo"
  create_fake_crontab "$fakebin" ready
  printf '15 4 * * * echo keep-me\n' > "$crontab_file"

  run_case "$log1" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      CCCLAW_FAKE_CRONTAB_FILE="$crontab_file" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --task-repo "41490/task-local" \
      --scheduler none

  cat > "$crontab_file" <<EOF
15 4 * * * echo keep-me
# >>> ccclaw managed cron >>>
*/5 * * * * $app_dir/bin/ccclaw ingest --config $app_dir/ops/config/config.toml --env-file $app_dir/.env
17 * * * * $app_dir/bin/ccclaw patrol --config $app_dir/ops/config/config.toml --env-file $app_dir/.env
23 2 * * * $app_dir/bin/ccclaw journal --config $app_dir/ops/config/config.toml --env-file $app_dir/.env
# <<< ccclaw managed cron <<<
EOF

  run_case "$log2" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      CCCLAW_FAKE_CRONTAB_FILE="$crontab_file" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --app-dir "$app_dir" \
      --remove-cron

  assert_contains "$crontab_file" '15 4 * * * echo keep-me'
  assert_not_contains "$crontab_file" '# >>> ccclaw managed cron >>>'
  assert_not_contains "$crontab_file" "$app_dir/bin/ccclaw ingest --config $app_dir/ops/config/config.toml --env-file $app_dir/.env"
  assert_contains "$log2" '已清理 ccclaw 受控 crontab 规则，并保留其他用户规则'
}

test_install_keeps_claude_read_only() {
  local sandbox fakebin claude_log app_dir home_repo task_repo log
  sandbox="$(setup_sandbox claude-readonly)"
  fakebin="$sandbox/fakebin"
  claude_log="$sandbox/claude.log"
  app_dir="$sandbox/app"
  home_repo="$sandbox/home-repo"
  task_repo="$sandbox/task-local"
  log="$sandbox/install.log"

  create_git_repo "$task_repo"
  create_fake_claude "$fakebin"

  run_case "$log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      CCCLAW_FAKE_CLAUDE_LOG="$claude_log" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      "$INSTALL_SCRIPT" \
      --yes \
      --skip-deps \
      --app-dir "$app_dir" \
      --home-repo "$home_repo" \
      --home-repo-mode init \
      --task-repo-mode local \
      --task-repo-local "$task_repo" \
      --task-repo "41490/task-local" \
      --scheduler none

  assert_file_exists "$claude_log"
  assert_contains "$claude_log" '--version'
  assert_contains "$claude_log" 'auth status --json'
  assert_contains "$claude_log" 'plugin list'
  assert_contains "$claude_log" 'plugin marketplace list'
  assert_not_contains "$claude_log" 'plugin marketplace add'
  assert_not_contains "$claude_log" 'plugin install'
  assert_not_contains "$claude_log" 'auth login'
  assert_contains "$log" 'Claude 生态处理: 默认只读探查，未自动修改 marketplace/plugins/rtk 全局配置'
}

test_upgrade_downloads_release_and_migrates_config() {
  local sandbox fakebin fixture_dir app_dir home_repo config_file env_file gh_log version old_bin croncfg_file
  sandbox="$(setup_sandbox upgrade-release)"
  fakebin="$sandbox/fakebin"
  fixture_dir="$sandbox/release-fixture"
  app_dir="$sandbox/app"
  home_repo="$sandbox/home-repo-custom"
  config_file="$app_dir/ops/config/config.toml"
  env_file="$app_dir/.env"
  croncfg_file="$app_dir/croncfg.md"
  gh_log="$sandbox/gh.log"
  version="$("$BIN_PATH" -V)"
  old_bin="$app_dir/bin/ccclaw"

  mkdir -p "$app_dir/bin" "$app_dir/ops/config" "$fixture_dir"
  create_fake_gh "$fakebin"
  create_release_fixture "$fixture_dir" "$version"

  cat > "$old_bin" <<'SCRIPT'
#!/usr/bin/env bash
if [[ "${1:-}" == "-V" || "${1:-}" == "--version" ]]; then
  printf 'old-version\n'
  exit 0
fi
printf 'old binary should not be used for command: %s\n' "$*" >&2
exit 90
SCRIPT
  chmod 755 "$old_bin"

  cat > "$app_dir/install.sh" <<'SCRIPT'
#!/usr/bin/env bash
printf 'unexpected local install invocation\n' >&2
exit 99
SCRIPT
  chmod 755 "$app_dir/install.sh"

  install -m 755 "$ROOT_DIR/dist/upgrade.sh" "$app_dir/upgrade.sh"
  cat > "$config_file" <<EOF
default_target = "41490/task-local"

[github]
control_repo = "someone/else"
issue_label = "ccclaw"
limit = 20

[paths]
app_dir = "$app_dir"
home_repo = "$home_repo"
var_dir = "$app_dir/var"
log_dir = "$app_dir/log"
kb_dir = "$home_repo/kb"
env_file = "$env_file"

[executor]
provider = "claude-code"
binary = ""
command = ["$app_dir/bin/ccclaude"]
timeout = "30m"

[scheduler]
mode = "none"
systemd_user_dir = "$sandbox/xdg/systemd/user"

[approval]
command = "/ccclaw approve"
minimum_permission = "admin"

[[targets]]
repo = "41490/task-local"
local_path = "/opt/src/task-local"
kb_path = "$home_repo/kb"
EOF
  printf 'GH_TOKEN=\n' > "$env_file"
  chmod 600 "$env_file"

  run_case "$sandbox/upgrade.log" \
    env \
      HOME="$sandbox/home" \
      XDG_CONFIG_HOME="$sandbox/xdg" \
      PATH="$fakebin:$PATH" \
      BIN_LINK="$sandbox/bin/ccclaw" \
      CCCLAW_FAKE_GH_LOG="$gh_log" \
      CCCLAW_FAKE_GH_RELEASE_DIR="$fixture_dir" \
      CCCLAW_FAKE_GH_TAG="$version" \
      "$app_dir/upgrade.sh"

  assert_contains "$gh_log" 'release view --repo 41490/ccclaw --json tagName --jq .tagName'
  assert_contains "$gh_log" "release download $version --repo 41490/ccclaw"
  assert_eq "$version" "$("$app_dir/bin/ccclaw" -V)" "升级后二进制版本异常"
  assert_contains "$config_file" 'control_repo = "41490/ccclaw"'
  assert_contains "$config_file" "home_repo = \"$home_repo\""
  assert_contains "$config_file" '[scheduler.timers]'
  assert_contains "$config_file" '[scheduler.logs]'
  assert_contains "$config_file" 'minimum_permission = "admin"'
  assert_not_contains "$config_file" 'command = "/ccclaw approve"'
  assert_contains "$app_dir/install.sh" 'CONTROL_REPO_DEFAULT="41490/ccclaw"'
  assert_file_exists "$croncfg_file"
  assert_contains "$croncfg_file" 'ccclaw cron 专家手工配置说明'
  assert_eq "seed ccclaw home repo (v$version)" "$(git -C "$home_repo" log -1 --pretty=%s)" "upgrade 应把 release tag 传给 seed commit"
}

main() {
  prepare_dist
  test_version_script_timezone
  log "开始执行 install.sh 回归测试"
  test_first_install_and_idempotent_reinstall
  log "已通过: 首装 + 幂等重装"
  test_home_repo_seed_commit_includes_version_and_ahead_hint
  log "已通过: home_repo seed 版本号与 ahead 提示"
  test_systemd_degrade_preflight
  log "已通过: systemd 降级体检"
  test_systemd_preflight_accepts_busless_deploy
  log "已通过: systemd 无 user bus 仍允许部署"
  test_systemd_preflight_auto_guides_manual_cron
  log "已通过: auto 在无 systemd 时仅输出手工 cron 指引"
  test_explicit_cron_mode_fails_fast
  log "已通过: 显式 cron 安装路径直接失败"
  test_legacy_cron_config_fails_fast_on_yes_mode
  log "已通过: 继承旧 cron 配置时非交互安装直接失败"
  test_systemd_install_auto_enable_and_restart
  log "已通过: systemd 安装自动启用与重装重启"
  test_merge_managed_markdown_preserves_skill_meta_fields
  log "已通过: Skill frontmatter sevolver 字段升级保留"
  test_interactive_mode_accepts_short_words
  log "已通过: 交互模式接受单字母输入"
  test_interactive_mode_accepts_full_words
  log "已通过: 交互模式接受完整单词输入"
  test_local_repo_without_origin_requires_repo
  log "已通过: local 无 origin 失败路径"
  test_remote_repo_path_must_stay_within_clone_root
  log "已通过: remote 路径越界失败路径"
  test_shell_integration_inject_and_remove
  log "已通过: shell 集成写入与回滚"
  test_remove_cron_cleans_managed_block_only
  log "已通过: 受控 cron 清理工具仅移除托管块"
  test_install_keeps_claude_read_only
  log "已通过: Claude 默认只读探查"
  test_upgrade_downloads_release_and_migrates_config
  log "已通过: upgrade.sh 官方 release 升级与配置迁移"
  log "全部 install.sh 回归测试通过"
}

main "$@"
