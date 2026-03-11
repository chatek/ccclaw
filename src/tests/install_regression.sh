#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
INSTALL_SCRIPT="$ROOT_DIR/dist/install.sh"
BIN_PATH="$ROOT_DIR/dist/bin/ccclaw"
WORK_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/ccclaw-install-regression.XXXXXX")"
REAL_HOME="${HOME}"

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
  grep -Fq "$pattern" "$file" || fail "未匹配到内容: $pattern ($file)"
}

assert_matches() {
  local file="$1"
  local pattern="$2"
  grep -Eq "$pattern" "$file" || fail "未匹配到正则: $pattern ($file)"
}

assert_file_missing() {
  local path="$1"
  [[ ! -e "$path" ]] || fail "不应存在文件: $path"
}

assert_not_contains() {
  local file="$1"
  local pattern="$2"
  if grep -Fq "$pattern" "$file"; then
    fail "出现了不期望的内容: $pattern ($file)"
  fi
}

assert_eq() {
  local expected="$1"
  local actual="$2"
  local message="$3"
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
    *)
      fail "未知 fake systemctl 模式: $mode"
      ;;
  esac
  chmod 755 "$dir/systemctl"
}

test_first_install_and_idempotent_reinstall() {
  local sandbox app_dir home_repo task_repo xdg log1 log2 config_file env_file target_count gh_count
  sandbox="$(setup_sandbox first-install)"
  app_dir="$sandbox/app"
  home_repo="$sandbox/home-repo"
  task_repo="$sandbox/task-local"
  xdg="$sandbox/xdg"
  log1="$sandbox/first.log"
  log2="$sandbox/reinstall.log"
  config_file="$app_dir/ops/config/config.toml"
  env_file="$app_dir/.env"

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
  assert_file_missing "$xdg/systemd/user/ccclaw-ingest.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-ingest.timer"
  assert_file_missing "$xdg/systemd/user/ccclaw-run.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-run.timer"
  assert_file_missing "$xdg/systemd/user/ccclaw-patrol.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-patrol.timer"
  assert_file_missing "$xdg/systemd/user/ccclaw-journal.service"
  assert_file_missing "$xdg/systemd/user/ccclaw-journal.timer"
  assert_contains "$config_file" "repo = '41490/task-local'"
  assert_contains "$config_file" "local_path = '$task_repo'"
  assert_contains "$config_file" "default_target = '41490/task-local'"
  assert_contains "$env_file" 'GH_TOKEN='
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

test_systemd_degrade_preflight() {
  local sandbox readonly_xdg log
  sandbox="$(setup_sandbox systemd-degrade)"
  readonly_xdg="$sandbox/readonly-xdg"
  log="$sandbox/preflight.log"
  mkdir -p "$readonly_xdg"
  chmod 500 "$readonly_xdg"

  run_case "$log" \
    env \
      HOME="$REAL_HOME" \
      XDG_CONFIG_HOME="$readonly_xdg" \
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

test_local_repo_without_origin_requires_repo() {
  local sandbox task_repo log
  sandbox="$(setup_sandbox local-no-origin)"
  task_repo="$sandbox/task-local"
  log="$sandbox/fail.log"

  create_git_repo "$task_repo"

  run_expect_fail "$log" \
    env \
      HOME="$REAL_HOME" \
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
      HOME="$REAL_HOME" \
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

main() {
  prepare_dist
  log "开始执行 install.sh 回归测试"
  test_first_install_and_idempotent_reinstall
  log "已通过: 首装 + 幂等重装"
  test_systemd_degrade_preflight
  log "已通过: systemd 降级体检"
  test_systemd_preflight_accepts_busless_deploy
  log "已通过: systemd 无 user bus 仍允许部署"
  test_local_repo_without_origin_requires_repo
  log "已通过: local 无 origin 失败路径"
  test_remote_repo_path_must_stay_within_clone_root
  log "已通过: remote 路径越界失败路径"
  test_shell_integration_inject_and_remove
  log "已通过: shell 集成写入与回滚"
  log "全部 install.sh 回归测试通过"
}

main "$@"
