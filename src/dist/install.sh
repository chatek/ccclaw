#!/usr/bin/env bash
set -euo pipefail

YES=0
SIMULATE=0
SKIP_DEPS=0
APP_DIR_DEFAULT="$HOME/.ccclaw"
HOME_REPO_DEFAULT="/opt/ccclaw"
CONTROL_REPO_DEFAULT="41490/ccclaw"
TARGET_PATH_DEFAULT="$(cd "$(dirname "$0")/../.." && pwd)"
TARGET_REPO_DEFAULT="$CONTROL_REPO_DEFAULT"
BIN_LINK_DEFAULT="$HOME/.local/bin/ccclaw"
SYSTEMD_USER_DIR_DEFAULT="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
DIST_DIR="$(cd "$(dirname "$0")" && pwd)"

APP_DIR="${APP_DIR:-$APP_DIR_DEFAULT}"
HOME_REPO="${HOME_REPO:-$HOME_REPO_DEFAULT}"
CONTROL_REPO="${CONTROL_REPO:-$CONTROL_REPO_DEFAULT}"
TARGET_REPO="${TARGET_REPO:-$TARGET_REPO_DEFAULT}"
TARGET_PATH="${TARGET_PATH:-$TARGET_PATH_DEFAULT}"
BIN_LINK="${BIN_LINK:-$BIN_LINK_DEFAULT}"
SYSTEMD_USER_DIR="$SYSTEMD_USER_DIR_DEFAULT"
ENV_FILE=""
CONFIG_FILE=""
STATE_DB=""
LOG_DIR=""
KB_DIR=""
CLAUDE_WRAPPER=""

expand_path() {
  local path="$1"
  if [[ "$path" == ~* ]]; then
    printf '%s\n' "${path/#\~/$HOME}"
  else
    printf '%s\n' "$path"
  fi
}

refresh_paths() {
  APP_DIR="$(expand_path "$APP_DIR")"
  HOME_REPO="$(expand_path "$HOME_REPO")"
  TARGET_PATH="$(expand_path "$TARGET_PATH")"
  BIN_LINK="$(expand_path "$BIN_LINK")"
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
  --yes                 非交互模式，尽量使用默认值
  --simulate            只打印安装流程与当前探查结果，不写入文件
  --skip-deps           跳过系统依赖安装
  --app-dir PATH        程序目录，默认 ~/.ccclaw
  --home-repo PATH      本体仓库目录，默认 /opt/ccclaw
  --control-repo REPO   控制仓库，默认 41490/ccclaw
  --target-repo REPO    首个目标仓库，默认等于 control repo
  --target-path PATH    首个目标仓库本地路径，默认当前源码仓库
  -h, --help            显示帮助
USAGE
}

while (($#)); do
  case "$1" in
    --yes) YES=1 ;;
    --simulate) SIMULATE=1 ;;
    --skip-deps) SKIP_DEPS=1 ;;
    --app-dir) shift; APP_DIR="$1" ;;
    --home-repo) shift; HOME_REPO="$1" ;;
    --control-repo) shift; CONTROL_REPO="$1" ;;
    --target-repo) shift; TARGET_REPO="$1" ;;
    --target-path) shift; TARGET_PATH="$1" ;;
    -h|--help) usage; exit 0 ;;
    *) fail "未知参数: $1" ;;
  esac
  shift
done

refresh_paths

have() { command -v "$1" >/dev/null 2>&1; }

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

need_sudo() {
  [[ "$HOME_REPO" == /opt/* || "$HOME_REPO" == /srv/* || "$HOME_REPO" == /var/* ]]
}

run_maybe_sudo() {
  if need_sudo && [[ ! -w "$(dirname "$HOME_REPO")" && ! -w "$HOME_REPO" ]]; then
    sudo "$@"
  else
    "$@"
  fi
}

ensure_home_repo_owner() {
  if need_sudo && [[ ! -w "$HOME_REPO" ]]; then
    sudo chown -R "$(id -un)":"$(id -gn)" "$HOME_REPO"
  fi
}

probe_claude() {
  local claude_bin claude_ver plugins marketplaces skills_market
  claude_bin="$(command -v claude 2>/dev/null || true)"
  claude_ver="$(claude --version 2>/dev/null || true)"
  plugins="$(claude plugin list 2>/dev/null || true)"
  marketplaces="$(claude plugin marketplace list 2>/dev/null || true)"
  skills_market="$(printf '%s' "$marketplaces" | grep -F 'anthropic-agent-skills' || true)"
  cat <<INFO
== Claude 探查 ==
- claude_bin: ${claude_bin:-<missing>}
- claude_version: ${claude_ver:-<missing>}
- credentials_file: $( [[ -f "$HOME/.claude/.credentials.json" ]] && echo present || echo missing )
- settings_json: $( [[ -f "$HOME/.claude/settings.json" ]] && echo present || echo missing )
- installed_plugins: $(printf '%s' "$plugins" | grep -c '@' || true)
- official_marketplace: $(printf '%s' "$marketplaces" | grep -F 'claude-plugins-official' >/dev/null && echo present || echo missing)
- skills_marketplace: $( [[ -n "$skills_market" ]] && echo present || echo missing )
INFO
}

print_flow() {
  cat <<FLOW
== 模拟安装流程 ==
1. 探查现有环境：claude / gh / rg / sqlite3 / rtk / git / node / npm / uv
2. 决定程序目录与本体仓库目录：
   - 程序目录: $APP_DIR
   - 本体仓库: $HOME_REPO
3. 生成固定隐私配置：
   - .env: $ENV_FILE
   - 仅记录会造成经济损失的敏感信息
4. 生成普通配置：
   - config.toml: $CONFIG_FILE
   - 记录 repo/path/执行器/systemd 等非敏感配置
5. 初始化本体仓库：
   - 若目录为空：git init + 写入 kb/docs/README/CLAUDE
   - 若目录已是 git 仓库：接管并检查结构
6. 安装程序文件：
   - $APP_DIR/bin/ccclaw
   - $APP_DIR/bin/ccclaude
   - $APP_DIR/ops/systemd/*.service|*.timer
7. 安装基础工具：
   - 必装: git gh rg sqlite3 curl wget
   - 能力工具: node npm uv
   - token 优化: rtk
8. Claude 适配：
   - 若本机已有插件/marketplace：直接继承
   - 若本机没有插件：安装指定官方 plugins + example-skills
   - 执行器默认走: $CLAUDE_WRAPPER
9. 安装 user systemd 单元：
   - $SYSTEMD_USER_DIR/ccclaw-ingest.service
   - $SYSTEMD_USER_DIR/ccclaw-run.service
10. 升级策略：
   - upgrade.sh 只升级程序发布树与插件/marketplace
   - 不自动覆盖本体仓库记忆内容

== 交互项矩阵 ==
- 必填且敏感(.env): GH_TOKEN
- 可选且敏感(.env): ANTHROPIC_API_KEY, GREPTILE_API_KEY
- 必填但可默认(config.toml): control_repo, target_repo, target_path
- 必填但可默认(config.toml): app_dir, home_repo, kb_dir, state_db, log_dir
- 可自动探查并默认继承: claude 路径、plugin marketplace、已装插件
FLOW
}

collect_inputs() {
  prompt_default APP_DIR "程序目录" "$APP_DIR"
  prompt_default HOME_REPO "本体仓库目录" "$HOME_REPO"
  prompt_default CONTROL_REPO "控制仓库 owner/repo" "$CONTROL_REPO"
  prompt_default TARGET_REPO "首个目标仓库 owner/repo" "$TARGET_REPO"
  prompt_default TARGET_PATH "首个目标仓库本地路径" "$TARGET_PATH"
  refresh_paths
}

ensure_dir() {
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] mkdir -p $*"
    return 0
  fi
  mkdir -p "$@"
}

ensure_system_packages() {
  local missing=()
  local required=(git gh rg sqlite3 curl wget)
  local optional=(node npm uv)
  for tool in "${required[@]}"; do
    have "$tool" || missing+=("$tool")
  done
  if [[ "${#missing[@]}" -eq 0 ]]; then
    log "基础工具已齐全"
  elif [[ "$SKIP_DEPS" -eq 1 ]]; then
    warn "跳过依赖安装，当前缺失: ${missing[*]}"
  elif have apt-get; then
    log "尝试用 apt-get 安装缺失工具: ${missing[*]}"
    if [[ "$SIMULATE" -eq 1 ]]; then
      log "[simulate] sudo apt-get update && sudo apt-get install -y ${missing[*]}"
    else
      sudo apt-get update
      sudo apt-get install -y "${missing[@]}"
    fi
  else
    warn "未找到可用包管理器，请手工安装: ${missing[*]}"
  fi
  for tool in "${optional[@]}"; do
    if have "$tool"; then
      log "已发现可选工具: $tool"
    fi
  done
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

create_env_file() {
  local gh_token="${GH_TOKEN:-}"
  local anthropic_key="${ANTHROPIC_API_KEY:-}"
  local greptile_key="${GREPTILE_API_KEY:-}"
  if [[ -f "$ENV_FILE" ]]; then
    log "复用已有隐私配置: $ENV_FILE"
    return 0
  fi
  prompt_default gh_token "GitHub Token (issues read/write)" "$gh_token" 1
  prompt_default anthropic_key "Anthropic API Key(可留空，若本机 claude 凭据已可用)" "$anthropic_key" 1
  prompt_default greptile_key "Greptile API Key(可留空)" "$greptile_key" 1
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] write $ENV_FILE"
    return 0
  fi
  umask 177
  cat > "$ENV_FILE" <<ENV
# 所有会导致经济损失的隐私信息都在这里。
GH_TOKEN=${gh_token}
ANTHROPIC_API_KEY=${anthropic_key}
GREPTILE_API_KEY=${greptile_key}
ENV
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
[github]
control_repo = "$CONTROL_REPO"
issue_label = "ccclaw"
limit = 20

[paths]
app_dir = "$APP_DIR"
home_repo = "$HOME_REPO"
state_db = "$STATE_DB"
log_dir = "$LOG_DIR"
kb_dir = "$KB_DIR"
env_file = "$ENV_FILE"

[executor]
provider = "claude-code"
command = ["$CLAUDE_WRAPPER"]
timeout = "30m"

[approval]
command = "/ccclaw approve"
minimum_permission = "write"

[[targets]]
repo = "$TARGET_REPO"
local_path = "$TARGET_PATH"
kb_path = "$KB_DIR"
CFG
}

create_user_systemd_units() {
  local ingest_service="$SYSTEMD_USER_DIR/ccclaw-ingest.service"
  local ingest_timer="$SYSTEMD_USER_DIR/ccclaw-ingest.timer"
  local run_service="$SYSTEMD_USER_DIR/ccclaw-run.service"
  local run_timer="$SYSTEMD_USER_DIR/ccclaw-run.timer"
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
}

create_app_layout() {
  ensure_dir "$APP_DIR/bin" "$APP_DIR/ops/config" "$APP_DIR/ops/systemd" "$APP_DIR/ops/scripts" "$APP_DIR/var" "$APP_DIR/log" "$HOME/.local/bin"
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] install $DIST_DIR/bin/ccclaw -> $APP_DIR/bin/ccclaw"
  else
    install -m 755 "$DIST_DIR/bin/ccclaw" "$APP_DIR/bin/ccclaw"
  fi
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

init_home_repo() {
  if [[ "$SIMULATE" -eq 1 ]]; then
    log "[simulate] initialize home repo at $HOME_REPO"
    return 0
  fi
  if [[ ! -d "$HOME_REPO" ]]; then
    run_maybe_sudo mkdir -p "$HOME_REPO"
  fi
  ensure_home_repo_owner
  if [[ ! -d "$HOME_REPO/.git" ]]; then
    log "初始化本体仓库: $HOME_REPO"
    git init "$HOME_REPO"
  fi
  mkdir -p "$HOME_REPO/kb/designs" "$HOME_REPO/kb/assay" "$HOME_REPO/kb/journal" "$HOME_REPO/kb/skills/L1" "$HOME_REPO/kb/skills/L2" "$HOME_REPO/docs/reports" "$HOME_REPO/docs/plans" "$HOME_REPO/docs/rfcs"
  cp -R "$DIST_DIR/kb/." "$HOME_REPO/kb/"
  cat > "$HOME_REPO/README.md" <<'README'
# ccclaw-home

这是本机 `ccclaw` 的封闭本体仓库。

- `kb/` 保存长期记忆与 skills
- `docs/` 保存本体级计划、报告与架构文档
- 该仓库不参与程序升级覆盖
README
  cat > "$HOME_REPO/CLAUDE.md" <<'CLAUDE'
# CLAUDE.md

此目录是 `ccclaw` 的本体记忆仓库，不是程序发布树。

- 允许持续追加 `kb/` 与 `docs/` 内容
- 升级程序时不要覆盖这里的用户记忆
- 所有变更优先保持可追溯、可提交、可回滚
CLAUDE
  cat > "$HOME_REPO/.gitignore" <<'GITIGNORE'
.DS_Store
*.swp
*.tmp
GITIGNORE
  if ! git -C "$HOME_REPO" rev-parse HEAD >/dev/null 2>&1; then
    git -C "$HOME_REPO" add .
    git -C "$HOME_REPO" -c user.name='ccclaw' -c user.email='ccclaw@local' commit -m 'init ccclaw home repo'
  fi
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
    warn "未找到 claude；跳过插件/skills 配置，请先手工安装 Claude Code"
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
  cat <<MSG
安装完成。

- 程序目录: $APP_DIR
- 本体仓库: $HOME_REPO
- 可执行文件: $APP_DIR/bin/ccclaw
- 执行器包装: $CLAUDE_WRAPPER
- 隐私配置: $ENV_FILE
- 普通配置: $CONFIG_FILE
- 本地命令链接: $BIN_LINK
- user systemd: $SYSTEMD_USER_DIR

建议下一步:
1. 检查 $ENV_FILE 中的敏感项是否齐全
2. 检查 $CONFIG_FILE 中的 repo/path 是否正确
3. 进入本体仓库: cd $HOME_REPO && git status
4. 启用用户定时器:
   systemctl --user daemon-reload
   systemctl --user enable --now ccclaw-ingest.timer ccclaw-run.timer
5. 运行一次体检:
   $APP_DIR/bin/ccclaw doctor --config $CONFIG_FILE --env-file $ENV_FILE
MSG
}

main() {
  print_flow
  probe_claude
  collect_inputs
  if [[ "$SIMULATE" -eq 1 ]]; then
    print_summary
    exit 0
  fi
  ensure_system_packages
  install_rtk
  create_app_layout
  init_home_repo
  configure_claude_assets
  print_summary
}

main "$@"
