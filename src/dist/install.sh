#!/usr/bin/env bash
set -euo pipefail

YES=0
SIMULATE=0
SKIP_DEPS=0
INSTALL_CLAUDE=0
PREFLIGHT_ONLY=0
REUSE_GH_AUTH=1
REUSE_CLAUDE_AUTH=1

APP_DIR_DEFAULT="$HOME/.ccclaw"
HOME_REPO_DEFAULT="/opt/ccclaw"
CONTROL_REPO_DEFAULT="41490/ccclaw"
BIN_LINK_DEFAULT="$HOME/.local/bin/ccclaw"
SYSTEMD_USER_DIR_DEFAULT="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
TASK_CLONE_ROOT_DEFAULT="/opt/src/3claw"
SCHEDULER_DEFAULT="auto"
DIST_DIR="$(cd "$(dirname "$0")" && pwd)"

APP_DIR="${APP_DIR:-$APP_DIR_DEFAULT}"
HOME_REPO="${HOME_REPO:-$HOME_REPO_DEFAULT}"
HOME_REPO_MODE="${HOME_REPO_MODE:-init}"
HOME_REPO_REMOTE="${HOME_REPO_REMOTE:-}"
CONTROL_REPO="${CONTROL_REPO:-$CONTROL_REPO_DEFAULT}"
BIN_LINK="${BIN_LINK:-$BIN_LINK_DEFAULT}"
SYSTEMD_USER_DIR="$SYSTEMD_USER_DIR_DEFAULT"
TASK_CLONE_ROOT="${TASK_CLONE_ROOT:-$TASK_CLONE_ROOT_DEFAULT}"
SCHEDULER="${SCHEDULER:-$SCHEDULER_DEFAULT}"

TASK_REPO_MODE="${TASK_REPO_MODE:-none}"
TASK_REPO_REMOTE="${TASK_REPO_REMOTE:-}"
TASK_REPO_LOCAL="${TASK_REPO_LOCAL:-}"
TASK_REPO="${TASK_REPO:-}"
TASK_REPO_PATH="${TASK_REPO_PATH:-}"
TASK_KB_PATH="${TASK_KB_PATH:-}"

ENV_FILE=""
CONFIG_FILE=""
STATE_DB=""
LOG_DIR=""
KB_DIR=""
CLAUDE_WRAPPER=""
SCHEDULER_EFFECTIVE=""
SCHEDULER_REASON="未判定"
SYSTEMD_READY=0
SYSTEMD_CONTROL_READY=0
SYSTEMD_CHECK_STATUS="未探查"
SYSTEMD_CONTROL_STATUS="未探查"
GH_AUTH_STATUS="未探查"
GH_TOKEN_DETECTED=""
CLAUDE_AUTH_READY=0
CLAUDE_AUTH_METHOD="未探查"
GH_TOKEN_SOURCE="未写入"
ANTHROPIC_API_KEY_SOURCE="未写入"
ANTHROPIC_BASE_URL_SOURCE="未写入"
ANTHROPIC_AUTH_TOKEN_SOURCE="未写入"
GREPTILE_API_KEY_SOURCE="未写入"

expand_path() {
  local path="$1"
  if [[ -z "$path" ]]; then
    printf '\n'
    return 0
  fi
  if [[ "$path" == ~* ]]; then
    printf '%s\n' "${path/#\~/$HOME}"
  else
    printf '%s\n' "$path"
  fi
}

refresh_paths() {
  APP_DIR="$(expand_path "$APP_DIR")"
  HOME_REPO="$(expand_path "$HOME_REPO")"
  BIN_LINK="$(expand_path "$BIN_LINK")"
  TASK_CLONE_ROOT="$(expand_path "$TASK_CLONE_ROOT")"
  TASK_REPO_LOCAL="$(expand_path "$TASK_REPO_LOCAL")"
  TASK_REPO_PATH="$(expand_path "$TASK_REPO_PATH")"
  TASK_KB_PATH="$(expand_path "$TASK_KB_PATH")"
  ENV_FILE="$APP_DIR/.env"
  CONFIG_FILE="$APP_DIR/ops/config/config.toml"
  STATE_DB="$APP_DIR/var/state.db"
  LOG_DIR="$APP_DIR/log"
  KB_DIR="$HOME_REPO/kb"
  CLAUDE_WRAPPER="$APP_DIR/bin/ccclaude"
}

log() { printf '[ccclaw-install] %s\n' "$*"; }
warn() { printf '[ccclaw-install][WARN] %s\n' "$*" >&2; }
fail() { printf '[ccclaw-install][FAIL] %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<USAGE
用法: $(basename "$0") [选项]

选项:
  --yes                     非交互模式，尽量使用默认值
  --simulate                只打印安装流程与当前探查结果，不写入文件
  --preflight-only          只执行安装前体检，不落盘
  --skip-deps               跳过系统依赖安装
  --install-claude          非交互模式下自动执行 Claude 官方安装
  --scheduler MODE          调度器模式: auto|systemd|cron|none
  --task-clone-root PATH    remote 任务仓库本地 clone 根目录，默认 /opt/src/3claw
  --app-dir PATH            程序目录，默认 ~/.ccclaw
  --home-repo PATH          本体仓库目录，默认 /opt/ccclaw
  --home-repo-mode MODE     本体仓库模式: init|remote|local
  --home-repo-remote URL    本体远程仓库 URL 或 owner/repo；传入后自动切换 remote 模式
  --control-repo REPO       控制仓库，默认 41490/ccclaw
  --task-repo-mode MODE     任务仓库模式: none|remote|local
  --task-repo-remote URL    任务远程仓库 URL 或 owner/repo；传入后自动切换 remote 模式
  --task-repo-local PATH    本地已有任务仓库路径；传入后自动切换 local 模式
  --task-repo REPO          任务仓库 owner/repo；未传时尽量自动推断
  --task-repo-path PATH     任务仓库本地路径；remote 模式默认 /opt/src/3claw/owner/repo
  --task-repo-kb-path PATH  任务仓库对应 kb 路径，默认继承全局 kb_dir
  --reuse-gh-auth           优先复用 gh auth token 写入 GH_TOKEN
  --reuse-claude-auth       优先复用本机 Claude 登录态，允许 API Key 留空
  -h, --help                显示帮助
USAGE
}

while (($#)); do
  case "$1" in
    --yes) YES=1 ;;
    --simulate) SIMULATE=1 ;;
    --preflight-only) PREFLIGHT_ONLY=1 ;;
    --skip-deps) SKIP_DEPS=1 ;;
    --install-claude) INSTALL_CLAUDE=1 ;;
    --scheduler) shift; SCHEDULER="$1" ;;
    --task-clone-root) shift; TASK_CLONE_ROOT="$1" ;;
    --app-dir) shift; APP_DIR="$1" ;;
    --home-repo) shift; HOME_REPO="$1" ;;
    --home-repo-mode) shift; HOME_REPO_MODE="$1" ;;
    --home-repo-remote) shift; HOME_REPO_REMOTE="$1"; HOME_REPO_MODE="remote" ;;
    --control-repo) shift; CONTROL_REPO="$1" ;;
    --task-repo-mode) shift; TASK_REPO_MODE="$1" ;;
    --task-repo-remote) shift; TASK_REPO_REMOTE="$1"; TASK_REPO_MODE="remote" ;;
    --task-repo-local) shift; TASK_REPO_LOCAL="$1"; TASK_REPO_MODE="local" ;;
    --task-repo) shift; TASK_REPO="$1" ;;
    --task-repo-path) shift; TASK_REPO_PATH="$1" ;;
    --task-repo-kb-path) shift; TASK_KB_PATH="$1" ;;
    --reuse-gh-auth) REUSE_GH_AUTH=1 ;;
    --reuse-claude-auth) REUSE_CLAUDE_AUTH=1 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "未知参数: $1" ;;
  esac
  shift
done

refresh_paths

have() { command -v "$1" >/dev/null 2>&1; }

print_stage() {
  printf '\n== %s ==\n' "$1"
}

prompt_default() {
  local var_name="$1" label="$2" default_value="$3" secret="${4:-0}" input
  if [[ "$YES" -eq 1 ]]; then
    printf -v "$var_name" '%s' "$default_value"
    return 0
  fi
  if [[ "$secret" -eq 1 ]]; then
    read -r -s -p "$label [$default_value]: " input
    printf '\n' >&2
  else
    read -r -p "$label [$default_value]: " input
  fi
  if [[ -z "$input" ]]; then
    input="$default_value"
  fi
  printf -v "$var_name" '%s' "$input"
}

prompt_mode() {
  local var_name="$1" label="$2" default_value="$3" allowed="$4"
  local input="$default_value"
  while true; do
    prompt_default input "$label" "$input"
    case " $allowed " in
      *" $input "*) printf -v "$var_name" '%s' "$input"; return 0 ;;
      *) warn "无效取值: $input；允许值: $allowed" ;;
    esac
  done
}

path_needs_sudo() {
  local path="$1"
  [[ "$path" == /opt/* || "$path" == /srv/* || "$path" == /var/* ]]
}

run_maybe_sudo_for_path() {
  local path="$1"
  shift
  if path_needs_sudo "$path" && [[ ! -w "$(dirname "$path")" && ! -w "$path" ]]; then
    sudo "$@"
  else
    "$@"
  fi
}

ensure_path_owner() {
  local path="$1"
  if [[ ! -e "$path" ]]; then
    return 0
  fi
  if path_needs_sudo "$path" && [[ ! -w "$path" ]]; then
    sudo chown -R "$(id -un)":"$(id -gn)" "$path"
  fi
}

ensure_dir() {
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] mkdir -p $*"
    return 0
  fi
  mkdir -p "$@"
}

dir_has_entries() {
  local path="$1"
  [[ -d "$path" ]] || return 1
  find "$path" -mindepth 1 -maxdepth 1 | read -r _
}

first_existing_parent() {
  local path="$1"
  while [[ ! -e "$path" && "$path" != "/" ]]; do
    path="$(dirname "$path")"
  done
  printf '%s\n' "$path"
}

path_writable_or_creatable() {
  local path="$1" existing
  if [[ -e "$path" ]]; then
    [[ -w "$path" ]]
    return $?
  fi
  existing="$(first_existing_parent "$path")"
  [[ -w "$existing" ]]
}

path_is_within() {
  local child="$1" parent="$2"
  child="${child%/}"
  parent="${parent%/}"
  case "$child/" in
    "$parent/"*) return 0 ;;
  esac
  return 1
}

append_line_if_missing() {
  local file="$1" line="$2"
  if grep -Fqx "$line" "$file" 2>/dev/null; then
    return 0
  fi
  printf '%s\n' "$line" >> "$file"
}

managed_file_has_markers() {
  local file="$1"
  grep -Fq '<!-- ccclaw:managed:start -->' "$file" 2>/dev/null \
    && grep -Fq '<!-- ccclaw:managed:end -->' "$file" 2>/dev/null \
    && grep -Fq '<!-- ccclaw:user:start -->' "$file" 2>/dev/null \
    && grep -Fq '<!-- ccclaw:user:end -->' "$file" 2>/dev/null
}

extract_marked_block() {
  local file="$1" start_marker="$2" end_marker="$3"
  awk -v start_marker="$start_marker" -v end_marker="$end_marker" '
    $0 == start_marker { in_block=1; next }
    $0 == end_marker { in_block=0; exit }
    in_block { print }
  ' "$file"
}

render_template_with_user_block() {
  local template="$1" user_block_file="$2" output="$3"
  awk -v user_start='<!-- ccclaw:user:start -->' \
      -v user_end='<!-- ccclaw:user:end -->' \
      -v user_block_file="$user_block_file" '
    $0 == user_start {
      print
      while ((getline line < user_block_file) > 0) {
        print line
      }
      close(user_block_file)
      in_user=1
      next
    }
    $0 == user_end {
      in_user=0
      print
      next
    }
    in_user { next }
    { print }
  ' "$template" > "$output"
}

merge_managed_markdown() {
  local template="$1" target="$2" tmp_file user_block_file
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] merge managed markdown $template -> $target"
    return 0
  fi
  mkdir -p "$(dirname "$target")"
  if [[ ! -f "$target" ]]; then
    install -m 644 "$template" "$target"
    return 0
  fi

  tmp_file="$(mktemp)"
  user_block_file="$(mktemp)"
  if managed_file_has_markers "$target"; then
    extract_marked_block "$target" '<!-- ccclaw:user:start -->' '<!-- ccclaw:user:end -->' > "$user_block_file"
  else
    {
      printf '<!-- 从升级前本地版本迁入；以下内容视为用户自定义补充，后续升级保留。 -->\n'
      cat "$target"
    } > "$user_block_file"
  fi

  render_template_with_user_block "$template" "$user_block_file" "$tmp_file"
  if ! cmp -s "$tmp_file" "$target"; then
    cat "$tmp_file" > "$target"
  fi
  rm -f "$tmp_file" "$user_block_file"
}

merge_kb_contracts() {
  local template relative target
  while IFS= read -r template; do
    relative="${template#$DIST_DIR/kb/}"
    target="$HOME_REPO/kb/$relative"
    merge_managed_markdown "$template" "$target"
  done < <(find "$DIST_DIR/kb" -type f -name 'CLAUDE.md' | sort)
}

normalize_repo_slug() {
  local repo="$1"
  repo="${repo%.git}"
  repo="${repo%/}"
  case "$repo" in
    git@github.com:*) repo="${repo#git@github.com:}" ;;
    ssh://git@github.com/*) repo="${repo#ssh://git@github.com/}" ;;
    https://github.com/*) repo="${repo#https://github.com/}" ;;
    http://github.com/*) repo="${repo#http://github.com/}" ;;
  esac
  if [[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
    printf '%s\n' "$repo"
    return 0
  fi
  return 1
}

clone_url_from_repo_input() {
  local input="$1"
  if slug="$(normalize_repo_slug "$input" 2>/dev/null)"; then
    printf 'https://github.com/%s.git\n' "$slug"
    return 0
  fi
  printf '%s\n' "$input"
}

repo_name_from_slug() {
  local slug="$1"
  printf '%s\n' "${slug##*/}"
}

repo_owner_from_slug() {
  local slug="$1"
  printf '%s\n' "${slug%%/*}"
}

task_clone_path_from_slug() {
  local slug="$1"
  printf '%s/%s/%s\n' "$TASK_CLONE_ROOT" "$(repo_owner_from_slug "$slug")" "$(repo_name_from_slug "$slug")"
}

repo_slug_from_local_repo() {
  local repo_path="$1" remote_url
  remote_url="$(git -C "$repo_path" remote get-url origin 2>/dev/null || true)"
  if [[ -z "$remote_url" ]]; then
    return 1
  fi
  normalize_repo_slug "$remote_url"
}

validate_mode() {
  local label="$1" value="$2" allowed="$3"
  case " $allowed " in
    *" $value "*) return 0 ;;
    *) fail "$label 取值无效: $value；允许值: $allowed" ;;
  esac
}

config_has_targets() {
  grep -q '^\[\[targets\]\]' "$CONFIG_FILE" 2>/dev/null
}

read_env_value() {
  local file="$1" key="$2"
  [[ -f "$file" ]] || return 1
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$file"
}

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[\\/&]/\\&/g'
}

upsert_env_key() {
  local file="$1" key="$2" value="$3" escaped
  escaped="$(escape_sed_replacement "$value")"
  if grep -q "^${key}=" "$file" 2>/dev/null; then
    sed -i "s/^${key}=.*/${key}=${escaped}/" "$file"
  else
    printf '%s=%s\n' "$key" "$value" >> "$file"
  fi
}

probe_claude() {
  local claude_bin claude_ver plugins marketplaces skills_market auth_state install_channel
  claude_bin="$(command -v claude 2>/dev/null || true)"
  claude_ver="$(claude --version 2>/dev/null || true)"
  plugins="$(claude plugin list 2>/dev/null || true)"
  marketplaces="$(claude plugin marketplace list 2>/dev/null || true)"
  skills_market="$(printf '%s' "$marketplaces" | grep -F 'anthropic-agent-skills' || true)"
  auth_state="$(claude auth status --json 2>/dev/null || true)"
  if [[ -n "$auth_state" || -f "$HOME/.claude/.credentials.json" ]]; then
    CLAUDE_AUTH_READY=1
    CLAUDE_AUTH_METHOD="cli-session"
  elif [[ -n "${ANTHROPIC_BASE_URL:-}" && -n "${ANTHROPIC_AUTH_TOKEN:-}" ]]; then
    CLAUDE_AUTH_READY=1
    CLAUDE_AUTH_METHOD="proxy-env"
  else
    CLAUDE_AUTH_READY=0
    CLAUDE_AUTH_METHOD="missing"
  fi
  if claude_install_channel_reachable; then
    install_channel="reachable"
  else
    install_channel="blocked"
  fi
  cat <<INFO
== Claude 探查 ==
- claude_bin: ${claude_bin:-<missing>}
- claude_version: ${claude_ver:-<missing>}
- install_channel: ${install_channel}
- credentials_file: $( [[ -f "$HOME/.claude/.credentials.json" ]] && echo present || echo missing )
- settings_json: $( [[ -f "$HOME/.claude/settings.json" ]] && echo present || echo missing )
- auth_status_json: $( [[ -n "$auth_state" ]] && echo present || echo missing )
- auth_method: ${CLAUDE_AUTH_METHOD}
- installed_plugins: $(printf '%s' "$plugins" | grep -c '@' || true)
- official_marketplace: $(printf '%s' "$marketplaces" | grep -F 'claude-plugins-official' >/dev/null && echo present || echo missing)
- skills_marketplace: $( [[ -n "$skills_market" ]] && echo present || echo missing )
- proxy_env: $( [[ -n "${ANTHROPIC_BASE_URL:-}" && -n "${ANTHROPIC_AUTH_TOKEN:-}" ]] && echo present || echo missing )
- go_version: $(go version 2>/dev/null || echo missing)
- sudo_nopasswd: $(sudo -n true >/dev/null 2>&1 && echo enabled || echo missing)
INFO
}

probe_gh_auth() {
  GH_TOKEN_DETECTED=""
  if ! have gh; then
    GH_AUTH_STATUS="missing_cli"
    return 0
  fi
  if gh auth status >/dev/null 2>&1; then
    GH_AUTH_STATUS="logged_in"
    if [[ "$REUSE_GH_AUTH" -eq 1 ]]; then
      GH_TOKEN_DETECTED="$(gh auth token 2>/dev/null || true)"
    fi
    if [[ -z "$GH_TOKEN_DETECTED" ]]; then
      GH_AUTH_STATUS="logged_in_without_token"
    fi
    return 0
  fi
  GH_AUTH_STATUS="not_logged_in"
}

probe_systemd_user() {
  local dir_owner=""
  SYSTEMD_READY=0
  SYSTEMD_CONTROL_READY=0
  if [[ -d "$SYSTEMD_USER_DIR" ]]; then
    dir_owner="$(stat -c '%U' "$SYSTEMD_USER_DIR" 2>/dev/null || true)"
  fi
  if [[ "$SCHEDULER" == "cron" || "$SCHEDULER" == "none" ]]; then
    SYSTEMD_CHECK_STATUS="skip(${SCHEDULER})"
    SYSTEMD_CONTROL_STATUS="skip(${SCHEDULER})"
    return 0
  fi
  if ! have systemctl; then
    SYSTEMD_CHECK_STATUS="missing_systemctl"
    SYSTEMD_CONTROL_STATUS="missing_systemctl"
    return 0
  fi
  if [[ -d "$SYSTEMD_USER_DIR" && -n "$dir_owner" && "$dir_owner" != "$(id -un)" ]]; then
    SYSTEMD_CHECK_STATUS="dir_owner=${dir_owner}"
    SYSTEMD_CONTROL_STATUS="skip(dir_owner)"
    return 0
  fi
  if ! path_writable_or_creatable "$SYSTEMD_USER_DIR"; then
    SYSTEMD_CHECK_STATUS="dir_not_writable"
    SYSTEMD_CONTROL_STATUS="skip(dir_not_writable)"
    return 0
  fi
  SYSTEMD_READY=1
  SYSTEMD_CHECK_STATUS="ready"
  if systemctl --user show-environment >/dev/null 2>&1; then
    SYSTEMD_CONTROL_READY=1
    SYSTEMD_CONTROL_STATUS="ready"
  else
    SYSTEMD_CONTROL_STATUS="user_bus_unavailable"
  fi
}

decide_scheduler() {
  case "$SCHEDULER" in
    systemd)
      if [[ "$SYSTEMD_READY" -eq 1 ]]; then
        SCHEDULER_EFFECTIVE="systemd"
        if [[ "$SYSTEMD_CONTROL_READY" -eq 1 ]]; then
          SCHEDULER_REASON="user systemd 可部署，且当前会话可直接控制"
        else
          SCHEDULER_REASON="user systemd 单元可部署，但当前会话未直连 user bus，安装后需手工启用 timer"
        fi
      else
        fail "已显式指定 --scheduler systemd，但当前环境不可用: $SYSTEMD_CHECK_STATUS"
      fi
      ;;
    auto)
      if [[ "$SYSTEMD_READY" -eq 1 ]]; then
        SCHEDULER_EFFECTIVE="systemd"
        if [[ "$SYSTEMD_CONTROL_READY" -eq 1 ]]; then
          SCHEDULER_REASON="自动选择 user systemd"
        else
          SCHEDULER_REASON="自动选择 user systemd；当前会话未直连 user bus，安装后需手工启用 timer"
        fi
      else
        SCHEDULER_EFFECTIVE="none"
        SCHEDULER_REASON="user systemd 不可用，自动降级为 none ($SYSTEMD_CHECK_STATUS)"
      fi
      ;;
    cron|none)
      SCHEDULER_EFFECTIVE="$SCHEDULER"
      SCHEDULER_REASON="按显式参数执行"
      ;;
  esac
}

print_preflight() {
  local gh_line systemd_line clone_root_line
  probe_gh_auth
  probe_systemd_user
  decide_scheduler
  case "$GH_AUTH_STATUS" in
    logged_in) gh_line="OK: 已登录 gh，支持复用 GH_TOKEN" ;;
    logged_in_without_token) gh_line="WARN: gh 已登录，但未能直接提取 token；后续可能仍需手工补录 GH_TOKEN" ;;
    not_logged_in) gh_line="WARN: 未检测到 gh 登录态；后续需要手工填写 GH_TOKEN" ;;
    missing_cli) gh_line="WARN: 未找到 gh；后续需要手工填写 GH_TOKEN" ;;
    *) gh_line="WARN: gh 状态未知" ;;
  esac
  case "$SYSTEMD_CHECK_STATUS" in
    ready)
      if [[ "$SYSTEMD_CONTROL_READY" -eq 1 ]]; then
        systemd_line="OK: user systemd 可部署，当前会话可直接控制"
      else
        systemd_line="WARN: user systemd 单元可部署，但当前会话未直连 user bus；安装后需在登录会话中手工启用 timer"
      fi
      ;;
    skip*) systemd_line="OK: 已跳过 user systemd 探查，当前调度模式 $SCHEDULER" ;;
    *) systemd_line="WARN: user systemd 不可直接使用($SYSTEMD_CHECK_STATUS)，将按 $SCHEDULER_EFFECTIVE 继续" ;;
  esac
  if [[ "$TASK_REPO_MODE" == "remote" ]]; then
    clone_root_line="OK: remote 任务仓库 clone 根目录固定为 $TASK_CLONE_ROOT"
  else
    clone_root_line="OK: 当前未启用 remote 任务仓库绑定"
  fi
  print_stage "安装前体检"
  cat <<INFO
- Claude: $( [[ "$CLAUDE_AUTH_READY" -eq 1 ]] && printf 'OK: 已检测到 %s' "$CLAUDE_AUTH_METHOD" || printf 'WARN: 未检测到可复用登录态，后续可能需要手工补录 API/代理配置' )
- GitHub: $gh_line
- Scheduler: $systemd_line
- Remote Clone Root: $clone_root_line
- 计划调度模式: 请求=$SCHEDULER, 生效=$SCHEDULER_EFFECTIVE
INFO
}

print_flow() {
  cat <<FLOW
== 计划执行流程 ==
1. 探查现有环境：claude / gh / rg / sqlite3 / tmux / rtk / git / node / npm / uv / systemd --user
2. 决定程序目录与本体仓库目录：
   - 程序目录: $APP_DIR
   - 本体仓库: $HOME_REPO
   - 本体仓库模式: $HOME_REPO_MODE
   - 调度模式: $SCHEDULER
3. 生成固定隐私配置：
   - .env: $ENV_FILE
   - 优先复用已有 .env 与 gh auth token
4. 生成普通配置：
   - config.toml: $CONFIG_FILE
   - 记录 repo/path/执行器/调度等非敏感配置
5. 初始化或接管本体仓库：
   - init: 在目标目录 git init，并写入 dist/kb 初始记忆树
   - remote: clone 指定仓库到目标目录，再补齐 kb 初始树
   - local: 接管本地已有 git 仓库，再补齐 kb 初始树
6. Claude 适配：
   - 先探查官方安装通道可达性
   - 交互模式可确认后执行官方安装脚本
   - 非交互模式仅在 --install-claude 时自动安装
   - 授权态优先复用 CLI session，其次代理环境变量或手工 API Key
7. 安装程序文件：
   - $APP_DIR/bin/ccclaw
   - $APP_DIR/bin/ccclaude
   - $APP_DIR/install.sh
   - $APP_DIR/upgrade.sh
   - $APP_DIR/ops/*
8. 安装基础工具：
   - 必装: git gh rg sqlite3 tmux curl wget golang
   - 能力工具: node npm uv
   - token 优化: rtk
9. 若 Claude 已可用：
   - 若本机已有插件/marketplace：直接继承
   - 若本机没有插件：安装指定官方 plugins + example-skills
   - 执行器默认走: $CLAUDE_WRAPPER
10. 安装 user systemd 单元：
   - 请求模式为 auto|systemd 时先体检
   - 仅在 user systemd 可用时写入 $SYSTEMD_USER_DIR
11. 任务仓库绑定：
   - none: 本轮不绑定
   - remote: clone 到 $TASK_CLONE_ROOT 下约定入口，并写入 config.toml
   - local: 接管已有本地仓库，并写入 config.toml
12. 升级策略：
   - upgrade.sh 只升级程序发布树与插件/marketplace
   - kb/**/CLAUDE.md 采用受管区块刷新，保留用户自定义区块
   - 不自动覆盖本体仓库记忆内容

== 交互项矩阵 ==
- 必填且敏感(.env): GH_TOKEN
- 可选且敏感(.env): ANTHROPIC_API_KEY, ANTHROPIC_BASE_URL, ANTHROPIC_AUTH_TOKEN, GREPTILE_API_KEY
- 必填但可默认(config.toml): control_repo
- 必填但可默认(config.toml): app_dir, home_repo, kb_dir, state_db, log_dir
- 本体仓库模式: init|remote|local
- 任务仓库模式: none|remote|local
- 调度器模式: auto|systemd|cron|none
- 可自动探查并默认继承: claude 路径、plugin marketplace、已装插件
FLOW
}

collect_inputs() {
  print_stage "阶段 1/4 安装拓扑"
  prompt_default APP_DIR "程序目录" "$APP_DIR"
  prompt_default CONTROL_REPO "控制仓库 owner/repo" "$CONTROL_REPO"
  prompt_mode HOME_REPO_MODE "本体仓库模式(init/remote/local)" "$HOME_REPO_MODE" "init remote local"
  prompt_default HOME_REPO "本体仓库目录" "$HOME_REPO"
  case "$HOME_REPO_MODE" in
    remote)
      prompt_default HOME_REPO_REMOTE "本体远程仓库(owner/repo 或 URL)" "$HOME_REPO_REMOTE"
      ;;
    local)
      prompt_default HOME_REPO "本地本体仓库路径" "$HOME_REPO"
      ;;
  esac

  print_stage "阶段 2/4 任务仓库绑定"
  prompt_mode TASK_REPO_MODE "任务仓库模式(none/remote/local)" "$TASK_REPO_MODE" "none remote local"
  case "$TASK_REPO_MODE" in
    remote)
      prompt_default TASK_REPO_REMOTE "任务远程仓库(owner/repo 或 URL)" "$TASK_REPO_REMOTE"
      if [[ -z "$TASK_REPO" ]]; then
        TASK_REPO="$(normalize_repo_slug "$TASK_REPO_REMOTE" 2>/dev/null || true)"
      fi
      if [[ -z "$TASK_REPO_PATH" && -n "$TASK_REPO" ]]; then
        TASK_REPO_PATH="$(task_clone_path_from_slug "$TASK_REPO")"
      fi
      prompt_default TASK_REPO "任务仓库 owner/repo" "$TASK_REPO"
      prompt_default TASK_REPO_PATH "任务仓库本地路径(固定入口位于 /opt/src/3claw)" "$TASK_REPO_PATH"
      prompt_default TASK_KB_PATH "任务仓库 kb 路径(可留空继承全局)" "$TASK_KB_PATH"
      ;;
    local)
      prompt_default TASK_REPO_LOCAL "本地任务仓库路径" "$TASK_REPO_LOCAL"
      if [[ -z "$TASK_REPO" && -d "$TASK_REPO_LOCAL/.git" ]]; then
        TASK_REPO="$(repo_slug_from_local_repo "$TASK_REPO_LOCAL" 2>/dev/null || true)"
      fi
      if [[ -n "$TASK_REPO" ]]; then
        log "已从本地仓库 origin 推断任务仓库: $TASK_REPO"
      else
        warn "未从 origin 推断出 owner/repo；运行时路由仍依赖该值，请手工输入。"
        prompt_default TASK_REPO "任务仓库 owner/repo" "$TASK_REPO"
      fi
      prompt_default TASK_KB_PATH "任务仓库 kb 路径(可留空继承全局)" "$TASK_KB_PATH"
      ;;
  esac

  print_stage "阶段 3/4 调度器"
  prompt_mode SCHEDULER "调度器模式(auto/systemd/cron/none)" "$SCHEDULER" "auto systemd cron none"
  refresh_paths
}

validate_inputs() {
  validate_mode "本体仓库模式" "$HOME_REPO_MODE" "init remote local"
  validate_mode "任务仓库模式" "$TASK_REPO_MODE" "none remote local"
  validate_mode "调度器模式" "$SCHEDULER" "auto systemd cron none"
  case "$HOME_REPO_MODE" in
    remote)
      [[ -n "$HOME_REPO_REMOTE" ]] || fail "remote 模式下必须提供 --home-repo-remote"
      ;;
    local)
      [[ -n "$HOME_REPO" ]] || fail "local 模式下必须提供本地本体仓库路径"
      ;;
  esac
  case "$TASK_REPO_MODE" in
    remote)
      [[ -n "$TASK_REPO_REMOTE" ]] || fail "remote 模式下必须提供 --task-repo-remote"
      if ! normalize_repo_slug "$TASK_REPO" >/dev/null 2>&1; then
        fail "无法识别任务仓库 owner/repo: $TASK_REPO"
      fi
      [[ -n "$TASK_REPO_PATH" ]] || fail "remote 模式下必须提供任务仓库本地路径"
      if ! path_is_within "$TASK_REPO_PATH" "$TASK_CLONE_ROOT"; then
        fail "remote 模式下任务仓库本地路径必须位于固定 clone 入口 $TASK_CLONE_ROOT"
      fi
      ;;
    local)
      [[ -n "$TASK_REPO_LOCAL" ]] || fail "local 模式下必须提供 --task-repo-local"
      if ! normalize_repo_slug "$TASK_REPO" >/dev/null 2>&1; then
        fail "local 模式下无法从仓库自动推断 owner/repo，且未提供有效 --task-repo"
      fi
      ;;
  esac
}

ensure_system_packages() {
  local missing_tools=()
  local missing_packages=()
  local required=(git gh rg sqlite3 tmux curl wget)
  local optional=(node npm uv)
  for tool in "${required[@]}"; do
    if ! have "$tool"; then
      missing_tools+=("$tool")
      missing_packages+=("$tool")
    fi
  done
  if ! have go; then
    missing_tools+=("go")
    missing_packages+=("golang-go")
  fi
  if [[ "${#missing_tools[@]}" -eq 0 ]]; then
    log "基础工具已齐全"
  elif [[ "$SKIP_DEPS" -eq 1 ]]; then
    warn "跳过依赖安装，当前缺失: ${missing_tools[*]}"
  elif have apt-get; then
    log "尝试用 apt-get 安装缺失工具: ${missing_tools[*]}"
    if [[ "$SIMULATE" -eq 1 ]]; then
      log "[simulate] sudo apt-get update && sudo apt-get install -y ${missing_packages[*]}"
    else
      sudo apt-get update
      sudo apt-get install -y "${missing_packages[@]}"
    fi
  else
    warn "未找到可用包管理器，请手工安装: ${missing_tools[*]}"
  fi
  for tool in "${optional[@]}"; do
    if have "$tool"; then
      log "已发现可选工具: $tool"
    fi
  done
}

claude_install_channel_reachable() {
  if have curl; then
    curl -fsSIL --connect-timeout 5 --max-time 15 https://claude.ai/install.sh >/dev/null 2>&1
    return $?
  fi
  if have wget; then
    wget -q --spider --timeout=15 https://claude.ai/install.sh
    return $?
  fi
  return 1
}

install_claude_official() {
  if have claude; then
    log "已发现 claude: $(command -v claude)"
    return 0
  fi
  if ! claude_install_channel_reachable; then
    fail "Claude 官方安装通道不可达，请先解决网络问题后重试；当前推荐命令: curl -fsSL https://claude.ai/install.sh | bash"
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] curl -fsSL https://claude.ai/install.sh | bash"
    return 0
  fi
  if [[ "$YES" -eq 1 && "$INSTALL_CLAUDE" -ne 1 ]]; then
    warn "非交互模式下未显式传入 --install-claude，跳过 Claude 自动安装"
    return 0
  fi
  if [[ "$YES" -ne 1 ]]; then
    local answer
    read -r -p "检测到未安装 Claude，是否执行官方安装脚本? [y/N]: " answer
    if [[ ! "$answer" =~ ^[Yy]$ ]]; then
      warn "已跳过 Claude 安装；后续请手工执行: curl -fsSL https://claude.ai/install.sh | bash"
      return 0
    fi
  fi
  log "执行 Claude 官方安装脚本"
  curl -fsSL https://claude.ai/install.sh | bash
}

install_rtk() {
  if have rtk; then
    log "rtk 已存在: $(command -v rtk)"
    return 0
  fi
  if [[ "$SKIP_DEPS" -eq 1 ]]; then
    warn "跳过 rtk 安装；请后续手工执行官方安装脚本"
    return 0
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh"
    return 0
  fi
  log "按官方 quick install 安装 rtk"
  curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh
}

create_claude_wrapper() {
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] write $CLAUDE_WRAPPER"
    return 0
  fi
  cat > "$CLAUDE_WRAPPER" <<'WRAP'
#!/usr/bin/env bash
set -euo pipefail
if command -v rtk >/dev/null 2>&1; then
  exec rtk proxy claude "$@"
fi
exec claude "$@"
WRAP
  chmod 755 "$CLAUDE_WRAPPER"
}

ensure_env_key() {
  local key="$1"
  if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    return 0
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] append ${key}= to $ENV_FILE"
    return 0
  fi
  printf '%s=\n' "$key" >> "$ENV_FILE"
}

create_env_file() {
  local gh_token="${GH_TOKEN:-}"
  local anthropic_key="${ANTHROPIC_API_KEY:-}"
  local anthropic_base_url="${ANTHROPIC_BASE_URL:-}"
  local anthropic_auth_token="${ANTHROPIC_AUTH_TOKEN:-}"
  local greptile_key="${GREPTILE_API_KEY:-}"
  local previous_umask
  if [[ -z "$gh_token" && -n "$GH_TOKEN_DETECTED" ]]; then
    gh_token="$GH_TOKEN_DETECTED"
    GH_TOKEN_SOURCE="gh auth token"
  fi
  if [[ -f "$ENV_FILE" ]]; then
    log "复用已有隐私配置: $ENV_FILE"
    ensure_env_key "GH_TOKEN"
    ensure_env_key "ANTHROPIC_API_KEY"
    ensure_env_key "ANTHROPIC_BASE_URL"
    ensure_env_key "ANTHROPIC_AUTH_TOKEN"
    ensure_env_key "GREPTILE_API_KEY"
    if [[ -n "$gh_token" && -z "$(read_env_value "$ENV_FILE" "GH_TOKEN")" ]]; then
      if [[ "$SIMULATE" -eq 1 ]]; then
        log "[simulate] 回填 GH_TOKEN 到 $ENV_FILE"
      else
        upsert_env_key "$ENV_FILE" "GH_TOKEN" "$gh_token"
      fi
    fi
    if [[ -n "$(read_env_value "$ENV_FILE" "GH_TOKEN")" ]]; then
      GH_TOKEN_SOURCE="已有 .env"
    elif [[ -n "$gh_token" ]]; then
      GH_TOKEN_SOURCE="gh auth token"
    else
      GH_TOKEN_SOURCE="留空"
    fi
    [[ -n "$(read_env_value "$ENV_FILE" "ANTHROPIC_API_KEY")" ]] && ANTHROPIC_API_KEY_SOURCE="已有 .env" || ANTHROPIC_API_KEY_SOURCE="留空"
    [[ -n "$(read_env_value "$ENV_FILE" "ANTHROPIC_BASE_URL")" ]] && ANTHROPIC_BASE_URL_SOURCE="已有 .env" || ANTHROPIC_BASE_URL_SOURCE="留空"
    [[ -n "$(read_env_value "$ENV_FILE" "ANTHROPIC_AUTH_TOKEN")" ]] && ANTHROPIC_AUTH_TOKEN_SOURCE="已有 .env" || ANTHROPIC_AUTH_TOKEN_SOURCE="留空"
    [[ -n "$(read_env_value "$ENV_FILE" "GREPTILE_API_KEY")" ]] && GREPTILE_API_KEY_SOURCE="已有 .env" || GREPTILE_API_KEY_SOURCE="留空"
    return 0
  fi
  print_stage "阶段 4/4 凭据复用与补录"
  if [[ -z "$gh_token" ]]; then
    prompt_default gh_token "GitHub Token (issues read/write)" "$gh_token" 1
    [[ -n "$gh_token" ]] && GH_TOKEN_SOURCE="本次输入" || GH_TOKEN_SOURCE="留空"
  fi
  if [[ "$GH_TOKEN_SOURCE" == "未写入" ]]; then
    [[ -n "$gh_token" ]] && GH_TOKEN_SOURCE="gh auth token" || GH_TOKEN_SOURCE="留空"
  fi
  if [[ "$REUSE_CLAUDE_AUTH" -eq 1 && "$CLAUDE_AUTH_READY" -eq 1 ]]; then
    log "已检测到 Claude 登录态($CLAUDE_AUTH_METHOD)，允许直接复用 CLI 会话；ANTHROPIC_API_KEY 可留空。"
  fi
  prompt_default anthropic_key "Anthropic API Key(可留空，若本机 claude 凭据已可用)" "$anthropic_key" 1
  prompt_default anthropic_base_url "Anthropic Base URL(代理模式可填，默认留空)" "$anthropic_base_url" 1
  prompt_default anthropic_auth_token "Anthropic Auth Token(代理模式可填，默认留空)" "$anthropic_auth_token" 1
  prompt_default greptile_key "Greptile API Key(可留空)" "$greptile_key" 1
  [[ -n "$anthropic_key" ]] && ANTHROPIC_API_KEY_SOURCE="本次输入" || ANTHROPIC_API_KEY_SOURCE="留空"
  [[ -n "$anthropic_base_url" ]] && ANTHROPIC_BASE_URL_SOURCE="本次输入" || ANTHROPIC_BASE_URL_SOURCE="留空"
  [[ -n "$anthropic_auth_token" ]] && ANTHROPIC_AUTH_TOKEN_SOURCE="本次输入" || ANTHROPIC_AUTH_TOKEN_SOURCE="留空"
  [[ -n "$greptile_key" ]] && GREPTILE_API_KEY_SOURCE="本次输入" || GREPTILE_API_KEY_SOURCE="留空"
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] write $ENV_FILE"
    return 0
  fi
  previous_umask="$(umask)"
  umask 177
  cat > "$ENV_FILE" <<ENV
# 所有会导致经济损失的隐私信息都在这里。
GH_TOKEN=${gh_token}
ANTHROPIC_API_KEY=${anthropic_key}
ANTHROPIC_BASE_URL=${anthropic_base_url}
ANTHROPIC_AUTH_TOKEN=${anthropic_auth_token}
GREPTILE_API_KEY=${greptile_key}
ENV
  umask "$previous_umask"
  chmod 600 "$ENV_FILE"
}

create_config_file() {
  if [[ -f "$CONFIG_FILE" ]]; then
    log "保留已有普通配置: $CONFIG_FILE"
    return 0
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] write $CONFIG_FILE"
    return 0
  fi
  cat > "$CONFIG_FILE" <<CFG
# default_target:
# - 留空时，运行时必须依赖 Issue body 中的 target_repo: owner/repo
# - 仅在存在多个 [[targets]] 时建议显式指定默认值
default_target = ""

# GitHub 控制面配置。
# control_repo 用于收 Issue、评论审批和权限判定，不等于实际干活的代码仓库。
[github]
control_repo = "$CONTROL_REPO"
issue_label = "ccclaw"
limit = 20

# 固定路径边界：
# - app_dir: 程序树
# - home_repo: 本体记忆仓库
# - kb_dir: 默认知识库目录
# - env_file: 所有敏感信息唯一入口
[paths]
app_dir = "$APP_DIR"
home_repo = "$HOME_REPO"
state_db = "$STATE_DB"
log_dir = "$LOG_DIR"
kb_dir = "$KB_DIR"
env_file = "$ENV_FILE"

# 执行器默认走 ccclaude 包装器，优先复用 rtk proxy，其次直连 claude。
[executor]
provider = "claude-code"
command = ["$CLAUDE_WRAPPER"]
timeout = "30m"

# 调度策略配置：
# - mode: 安装时请求的调度模式
# - systemd_user_dir: user systemd 单元写入目录
[scheduler]
mode = "$SCHEDULER"
systemd_user_dir = "$SYSTEMD_USER_DIR"

# 仅管理员 Issue 自动执行；非管理员需要管理员评论 /ccclaw approve。
[approval]
command = "/ccclaw approve"
minimum_permission = "admin"

# 任务仓库样例：
# [[targets]]
# repo = "owner/repo"
# local_path = "/opt/src/3claw/owner/repo"
# kb_path = "$KB_DIR"
# disabled = false
CFG
}

create_user_systemd_units() {
  local ingest_service="$SYSTEMD_USER_DIR/ccclaw-ingest.service"
  local ingest_timer="$SYSTEMD_USER_DIR/ccclaw-ingest.timer"
  local run_service="$SYSTEMD_USER_DIR/ccclaw-run.service"
  local run_timer="$SYSTEMD_USER_DIR/ccclaw-run.timer"
  local patrol_service="$SYSTEMD_USER_DIR/ccclaw-patrol.service"
  local patrol_timer="$SYSTEMD_USER_DIR/ccclaw-patrol.timer"
  local journal_service="$SYSTEMD_USER_DIR/ccclaw-journal.service"
  local journal_timer="$SYSTEMD_USER_DIR/ccclaw-journal.timer"
  if [[ "$SCHEDULER_EFFECTIVE" != "systemd" ]]; then
    log "跳过 user systemd 单元部署；当前调度模式: $SCHEDULER_EFFECTIVE"
    return 0
  fi
  ensure_dir "$SYSTEMD_USER_DIR"
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] write user systemd units into $SYSTEMD_USER_DIR"
    return 0
  fi
  cat > "$ingest_service" <<UNIT
[Unit]
Description=ccclaw ingest service
After=network-online.target

[Service]
Type=oneshot
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/bin/ccclaw ingest --config $CONFIG_FILE --env-file $ENV_FILE
UNIT
  cat > "$ingest_timer" <<UNIT
[Unit]
Description=Run ccclaw ingest every 5 minutes

[Timer]
OnCalendar=*:0/5
Persistent=true
Unit=ccclaw-ingest.service

[Install]
WantedBy=timers.target
UNIT
  cat > "$run_service" <<UNIT
[Unit]
Description=ccclaw run service
After=network-online.target

[Service]
Type=oneshot
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/bin/ccclaw run --config $CONFIG_FILE --env-file $ENV_FILE
UNIT
  cat > "$run_timer" <<UNIT
[Unit]
Description=Run ccclaw worker every 10 minutes

[Timer]
OnCalendar=*:0/10
Persistent=true
Unit=ccclaw-run.service

[Install]
WantedBy=timers.target
UNIT
  cat > "$patrol_service" <<UNIT
[Unit]
Description=ccclaw patrol service
After=network-online.target

[Service]
Type=oneshot
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/bin/ccclaw patrol --config $CONFIG_FILE --env-file $ENV_FILE
UNIT
  cat > "$patrol_timer" <<UNIT
[Unit]
Description=Run ccclaw patrol every 2 minutes

[Timer]
OnCalendar=*:0/2
Persistent=true
Unit=ccclaw-patrol.service

[Install]
WantedBy=timers.target
UNIT
  cat > "$journal_service" <<UNIT
[Unit]
Description=ccclaw journal service
After=network-online.target

[Service]
Type=oneshot
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/bin/ccclaw journal --config $CONFIG_FILE --env-file $ENV_FILE
UNIT
  cat > "$journal_timer" <<UNIT
[Unit]
Description=Run ccclaw journal every day at 23:50

[Timer]
OnCalendar=*-*-* 23:50:00
Persistent=true
Unit=ccclaw-journal.service

[Install]
WantedBy=timers.target
UNIT
}

copy_ops_tree() {
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] cp -R $DIST_DIR/ops/. $APP_DIR/ops/"
    return 0
  fi
  cp -R "$DIST_DIR/ops/." "$APP_DIR/ops/"
}

install_release_scripts() {
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] install $DIST_DIR/install.sh -> $APP_DIR/install.sh"
    log "[simulate] install $DIST_DIR/upgrade.sh -> $APP_DIR/upgrade.sh"
    return 0
  fi
  install -m 755 "$DIST_DIR/install.sh" "$APP_DIR/install.sh"
  install -m 755 "$DIST_DIR/upgrade.sh" "$APP_DIR/upgrade.sh"
}

create_app_layout() {
  ensure_dir "$APP_DIR/bin" "$APP_DIR/ops/config" "$APP_DIR/ops/systemd" "$APP_DIR/ops/scripts" "$APP_DIR/var" "$APP_DIR/log" "$HOME/.local/bin"
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] install $DIST_DIR/bin/ccclaw -> $APP_DIR/bin/ccclaw"
  else
    install -m 755 "$DIST_DIR/bin/ccclaw" "$APP_DIR/bin/ccclaw"
  fi
  install_release_scripts
  copy_ops_tree
  create_claude_wrapper
  create_env_file
  create_config_file
  create_user_systemd_units
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] ln -sf $APP_DIR/bin/ccclaw $BIN_LINK"
  else
    ln -sf "$APP_DIR/bin/ccclaw" "$BIN_LINK"
  fi
}

seed_home_repo_tree() {
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] seed kb/docs tree into $HOME_REPO"
    return 0
  fi
  mkdir -p "$HOME_REPO/kb/designs" "$HOME_REPO/kb/assay" "$HOME_REPO/kb/journal" "$HOME_REPO/kb/skills/L1" "$HOME_REPO/kb/skills/L2" "$HOME_REPO/docs/reports" "$HOME_REPO/docs/plans" "$HOME_REPO/docs/rfcs"
  cp -R -n "$DIST_DIR/kb/." "$HOME_REPO/kb/"
  merge_kb_contracts
  if [[ ! -f "$HOME_REPO/README.md" ]]; then
    cat > "$HOME_REPO/README.md" <<'README'
# ccclaw-home

这是本机 `ccclaw` 的封闭本体仓库。

- `kb/` 保存长期记忆与 skills
- `docs/` 保存本体级计划、报告与架构文档
- 该仓库不参与程序升级覆盖
README
  fi
  if [[ ! -f "$HOME_REPO/CLAUDE.md" ]]; then
    cat > "$HOME_REPO/CLAUDE.md" <<'CLAUDE'
# CLAUDE.md

此目录是 `ccclaw` 的本体记忆仓库，不是程序发布树。

- 允许持续追加 `kb/` 与 `docs/` 内容
- 升级程序时不要覆盖这里的用户记忆
- 所有变更优先保持可追溯、可提交、可回滚
CLAUDE
  fi
  touch "$HOME_REPO/.gitignore"
  append_line_if_missing "$HOME_REPO/.gitignore" ".DS_Store"
  append_line_if_missing "$HOME_REPO/.gitignore" "*.swp"
  append_line_if_missing "$HOME_REPO/.gitignore" "*.tmp"
}

commit_home_repo_seed() {
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] git add/commit seeded files in $HOME_REPO"
    return 0
  fi
  git -C "$HOME_REPO" add kb README.md CLAUDE.md .gitignore
  if git -C "$HOME_REPO" diff --cached --quiet; then
    return 0
  fi
  git -C "$HOME_REPO" -c user.name='ccclaw' -c user.email='ccclaw@local' commit -m 'seed ccclaw home repo'
}

init_or_attach_home_repo() {
  case "$HOME_REPO_MODE" in
    init)
      if [[ "$SIMULATE" -eq 1 ]]; then
        log "[simulate] initialize home repo at $HOME_REPO"
        return 0
      fi
      if [[ ! -d "$HOME_REPO" ]]; then
        run_maybe_sudo_for_path "$HOME_REPO" mkdir -p "$HOME_REPO"
      fi
      ensure_path_owner "$HOME_REPO"
      if [[ ! -d "$HOME_REPO/.git" ]]; then
        log "初始化本体仓库: $HOME_REPO"
        git -C "$HOME_REPO" init
      fi
      ;;
    remote)
      [[ -n "$HOME_REPO_REMOTE" ]] || fail "本体 remote 模式缺少远程仓库"
      if [[ "$SIMULATE" -eq 1 ]]; then
        log "[simulate] git clone $(clone_url_from_repo_input "$HOME_REPO_REMOTE") $HOME_REPO"
        return 0
      fi
      if [[ -e "$HOME_REPO" ]] && dir_has_entries "$HOME_REPO"; then
        fail "本体 remote 模式要求目标目录为空: $HOME_REPO"
      fi
      run_maybe_sudo_for_path "$HOME_REPO" mkdir -p "$(dirname "$HOME_REPO")"
      run_maybe_sudo_for_path "$HOME_REPO" git clone "$(clone_url_from_repo_input "$HOME_REPO_REMOTE")" "$HOME_REPO"
      ensure_path_owner "$HOME_REPO"
      ;;
    local)
      if [[ "$SIMULATE" -eq 1 ]]; then
        log "[simulate] attach local home repo at $HOME_REPO"
        return 0
      fi
      [[ -d "$HOME_REPO/.git" ]] || fail "本地本体仓库不是 git 仓库: $HOME_REPO"
      ensure_path_owner "$HOME_REPO"
      ;;
  esac
  if [[ "$SIMULATE" -eq 1 ]]; then
    return 0
  fi
  seed_home_repo_tree
  commit_home_repo_seed
}

prepare_task_clone_root() {
  if [[ "$TASK_REPO_MODE" != "remote" ]]; then
    return 0
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] ensure remote clone root $TASK_CLONE_ROOT"
    return 0
  fi
  run_maybe_sudo_for_path "$TASK_CLONE_ROOT" mkdir -p "$TASK_CLONE_ROOT"
  ensure_path_owner "$TASK_CLONE_ROOT"
}

bind_task_repo() {
  local target_args
  case "$TASK_REPO_MODE" in
    none)
      return 0
      ;;
    remote)
      prepare_task_clone_root
      if [[ "$SIMULATE" -eq 1 ]]; then
        log "[simulate] git clone $(clone_url_from_repo_input "$TASK_REPO_REMOTE") $TASK_REPO_PATH"
      else
        if [[ -e "$TASK_REPO_PATH" ]] && dir_has_entries "$TASK_REPO_PATH"; then
          fail "任务 remote 模式要求目标目录为空: $TASK_REPO_PATH"
        fi
        run_maybe_sudo_for_path "$TASK_REPO_PATH" mkdir -p "$(dirname "$TASK_REPO_PATH")"
        run_maybe_sudo_for_path "$TASK_REPO_PATH" git clone "$(clone_url_from_repo_input "$TASK_REPO_REMOTE")" "$TASK_REPO_PATH"
        ensure_path_owner "$TASK_REPO_PATH"
      fi
      ;;
    local)
      [[ -d "$TASK_REPO_LOCAL/.git" ]] || fail "本地任务仓库不是 git 仓库: $TASK_REPO_LOCAL"
      TASK_REPO_PATH="$TASK_REPO_LOCAL"
      ;;
  esac

  target_args=(target add --config "$CONFIG_FILE" --repo "$TASK_REPO" --path "$TASK_REPO_PATH")
  if [[ -n "$TASK_KB_PATH" ]]; then
    target_args+=(--kb-path "$TASK_KB_PATH")
  fi
  if ! config_has_targets; then
    target_args+=(--default)
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] $APP_DIR/bin/ccclaw ${target_args[*]}"
    return 0
  fi
  "$APP_DIR/bin/ccclaw" "${target_args[@]}"
}

ensure_marketplace() {
  local repo="$1"
  local name="$2"
  if ! have claude; then
    warn "未安装 claude，无法配置 marketplace: $repo"
    return 0
  fi
  if claude plugin marketplace list 2>/dev/null | grep -F "$name" >/dev/null 2>&1; then
    log "已存在 marketplace: $name"
    return 0
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] claude plugin marketplace add $repo"
    return 0
  fi
  claude plugin marketplace add "$repo"
}

configure_claude_assets() {
  if ! have claude; then
    warn "未找到 claude；跳过插件/skills 配置，请先完成 Claude 安装与 setup-token/proxy 初始化"
    return 0
  fi
  ensure_marketplace anthropics/skills anthropic-agent-skills
  ensure_marketplace anthropics/claude-plugins-official claude-plugins-official
  local installed_count
  installed_count="$(claude plugin list 2>/dev/null | grep -c '@' || true)"
  if [[ "$installed_count" -gt 0 ]]; then
    log "检测到本机已安装 Claude plugins，按决策直接继承，不追加默认集合"
  else
    local plugins=(
      "example-skills@anthropic-agent-skills"
      "github@claude-plugins-official"
      "greptile@claude-plugins-official"
      "claude-code-setup@claude-plugins-official"
      "claude-md-management@claude-plugins-official"
      "code-review@claude-plugins-official"
      "code-simplifier@claude-plugins-official"
      "commit-commands@claude-plugins-official"
      "gopls-lsp@claude-plugins-official"
      "hookify@claude-plugins-official"
      "plugin-dev@claude-plugins-official"
      "pyright-lsp@claude-plugins-official"
      "skill-creator@claude-plugins-official"
      "typescript-lsp@claude-plugins-official"
    )
    for plugin in "${plugins[@]}"; do
      if [[ "$SIMULATE" -eq 1 ]]; then
        log "[simulate] claude plugin install $plugin"
      else
        claude plugin install "$plugin" || warn "插件安装失败，可后续手工补装: $plugin"
      fi
    done
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] rtk init --global"
  elif have rtk; then
    rtk init --global || warn "rtk init --global 执行失败，请后续手工检查"
  fi
}

print_summary() {
  local installed_version="unknown"
  local task_summary="未绑定"
  local home_source_summary="$HOME_REPO_MODE"
  local gh_token_summary="$GH_TOKEN_SOURCE"
  local headline="安装完成。"
  local result_title="当前主机部署成果"
  local scheduler_step_6=""
  local scheduler_step_7=""
  if [[ -x "$APP_DIR/bin/ccclaw" ]]; then
    installed_version="$("$APP_DIR/bin/ccclaw" -V 2>/dev/null || echo unknown)"
  fi
  if [[ "$PREFLIGHT_ONLY" -eq 1 ]]; then
    headline="体检完成，未写入文件。"
    result_title="当前主机体检结果"
  elif [[ "$SIMULATE" -eq 1 ]]; then
    headline="模拟执行完成，未写入文件。"
    result_title="当前主机模拟结果"
  fi
  if [[ "$gh_token_summary" == "未写入" && -n "$GH_TOKEN_DETECTED" ]]; then
    gh_token_summary="待从 gh auth token 回填"
  fi
  case "$HOME_REPO_MODE" in
    remote) home_source_summary="remote -> $HOME_REPO_REMOTE" ;;
    local) home_source_summary="local -> $HOME_REPO" ;;
    init) home_source_summary="init -> $HOME_REPO" ;;
  esac
  case "$TASK_REPO_MODE" in
    remote) task_summary="$TASK_REPO @ $TASK_REPO_PATH (from $TASK_REPO_REMOTE)" ;;
    local) task_summary="$TASK_REPO @ $TASK_REPO_PATH (local)" ;;
  esac
  case "$SCHEDULER_EFFECTIVE" in
    systemd)
      scheduler_step_6="6. 按需启用用户定时器:
   systemctl --user daemon-reload
   systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer"
      scheduler_step_7="7. 若后续环境不适合 systemd --user，可改为手工写入 crontab 样板:
   */5 * * * * $APP_DIR/bin/ccclaw ingest --config $CONFIG_FILE --env-file $ENV_FILE
   */10 * * * * $APP_DIR/bin/ccclaw run --config $CONFIG_FILE --env-file $ENV_FILE"
      ;;
    cron)
      scheduler_step_6="6. 当前调度模式为 cron，请手工写入 crontab:
   */5 * * * * $APP_DIR/bin/ccclaw ingest --config $CONFIG_FILE --env-file $ENV_FILE
   */10 * * * * $APP_DIR/bin/ccclaw run --config $CONFIG_FILE --env-file $ENV_FILE"
      scheduler_step_7="7. 若后续切回 systemd --user，请先执行:
   systemctl --user daemon-reload
   systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer"
      ;;
    none|*)
      scheduler_step_6="6. 当前调度模式为 none；如需后台调度，可手工写入 crontab:
   */5 * * * * $APP_DIR/bin/ccclaw ingest --config $CONFIG_FILE --env-file $ENV_FILE
   */10 * * * * $APP_DIR/bin/ccclaw run --config $CONFIG_FILE --env-file $ENV_FILE"
      scheduler_step_7="7. 若后续修复好 user systemd，再执行:
   systemctl --user daemon-reload
   systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer ccclaw-patrol.timer ccclaw-journal.timer"
      ;;
  esac
  cat <<MSG
$headline

$result_title
- 版本: $installed_version
- 程序目录: $APP_DIR
- 本体仓库: $HOME_REPO
- 本体仓库来源: $home_source_summary
- 可执行文件: $APP_DIR/bin/ccclaw
- 执行器包装: $CLAUDE_WRAPPER
- 隐私配置: $ENV_FILE
- GH_TOKEN 来源: $gh_token_summary
- Claude 凭据态: $CLAUDE_AUTH_METHOD
- 普通配置: $CONFIG_FILE
- 本地命令链接: $BIN_LINK
- 任务仓库绑定: $task_summary
- 调度器: 请求=$SCHEDULER, 生效=$SCHEDULER_EFFECTIVE
- 调度器说明: $SCHEDULER_REASON
- user systemd 单元目录: $SYSTEMD_USER_DIR
- user systemd 单元(仅 systemd 模式写入):
  - ccclaw-ingest.service
  - ccclaw-ingest.timer
  - ccclaw-run.service
  - ccclaw-run.timer
  - ccclaw-patrol.service
  - ccclaw-patrol.timer
  - ccclaw-journal.service
  - ccclaw-journal.timer
- crontab: 默认不自动写入，仅提供样板

建议下一步
1. 检查 $ENV_FILE 中的敏感项是否齐全
2. 检查 $CONFIG_FILE 中的 repo/path 是否正确
3. 进入本体仓库: cd $HOME_REPO && git status
4. 运行一次体检:
   $APP_DIR/bin/ccclaw doctor --config $CONFIG_FILE --env-file $ENV_FILE
5. 若需要补绑任务仓库:
   $APP_DIR/bin/ccclaw target list --config $CONFIG_FILE
$scheduler_step_6
$scheduler_step_7
8. 若本体仓库目前还没有 upstream，可按需执行:
   git -C $HOME_REPO remote add origin <repo-url>
   git -C $HOME_REPO branch -M main
   git -C $HOME_REPO push -u origin main
9. 若任务仓库后续迁移目录，请同步修改 $CONFIG_FILE 中对应 target 的 local_path

日常使用流程
1. 绑定工作任务仓库
2. 在控制仓库创建或维护 Issue
3. 检验工作成果，并继续在 Issue 中回复交流

开源协作流程
1. 管理员创建的 Issue 会自动进入执行判定
2. 外部成员 Issue 默认只巡查与讨论，不自动执行
3. 值得接受的提案，由管理员评论 /ccclaw approve 后进入执行
MSG
}

main() {
  probe_claude
  collect_inputs
  validate_inputs
  if [[ "$TASK_REPO_MODE" == "local" && -z "$TASK_REPO_PATH" ]]; then
    TASK_REPO_PATH="$TASK_REPO_LOCAL"
  fi
  print_flow
  print_preflight
  if [[ "$PREFLIGHT_ONLY" -eq 1 ]]; then
    print_summary
    exit 0
  fi
  if [[ "$SIMULATE" -eq 1 ]]; then
    print_summary
    exit 0
  fi
  ensure_system_packages
  install_claude_official
  install_rtk
  create_app_layout
  init_or_attach_home_repo
  bind_task_repo
  configure_claude_assets
  print_summary
}

main "$@"
