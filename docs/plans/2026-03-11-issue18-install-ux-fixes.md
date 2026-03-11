# Issue #18 安装 UX 三项修复 实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 install.sh 的三个问题：prompt_mode 缩写化、local 模式路径即时验证、approval 废弃配置自动迁移

**Architecture:** 全部改动集中在 `src/dist/install.sh` 一个文件，涉及 `prompt_mode` 函数重写、`collect_inputs` local 分支重写、`create_app_layout` 追加迁移调用

**Tech Stack:** Bash (install.sh)

**决策记录（来自 sysNOTA）：**
1. 缩写大小写不敏感
2. 无 `.git` 直接 fail
3. approval 迁移失败 fail 停止安装
4. 不扫描候选，要求输入绝对路径
5. 所有 prompt_mode 调用统一改缩写

---

## 文件结构

- **Modify:** `src/dist/install.sh:195-205` — `prompt_mode()` 函数重写
- **Modify:** `src/dist/install.sh:834` — 本体仓库模式 prompt_mode 调用标签
- **Modify:** `src/dist/install.sh:846` — 任务仓库模式 prompt_mode 调用标签
- **Modify:** `src/dist/install.sh:860-872` — local 分支路径验证循环
- **Modify:** `src/dist/install.sh:876` — 调度器模式 prompt_mode 调用标签
- **Modify:** `src/dist/install.sh:1124-1127` — `create_config_file()` 追加迁移逻辑

---

## Chunk 1: prompt_mode 缩写化 + local 路径验证 + approval 自动迁移

### Task 1: 重写 prompt_mode 支持首字母缩写

**Files:**
- Modify: `src/dist/install.sh:195-205`

- [ ] **Step 1: 重写 `prompt_mode` 函数**

将 `install.sh:195-205` 替换为：

```bash
prompt_mode() {
  local var_name="$1" label="$2" default_value="$3" allowed="$4"
  local input default_short
  default_short="$(printf '%s' "${default_value:0:1}" | tr '[:lower:]' '[:upper:]')"
  while true; do
    prompt_default input "$label" "$default_short"
    local normalized
    normalized="$(printf '%s' "$input" | tr '[:upper:]' '[:lower:]')"
    # 完整单词精确匹配
    case " $allowed " in
      *" $normalized "*) printf -v "$var_name" '%s' "$normalized"; return 0 ;;
    esac
    # 首字母缩写展开
    local match="" word
    for word in $allowed; do
      if [[ "${word:0:1}" == "$normalized" ]]; then
        match="$word"
        break
      fi
    done
    if [[ -n "$match" ]]; then
      printf -v "$var_name" '%s' "$match"
      return 0
    fi
    warn "无效取值: $input；允许值: $allowed"
  done
}
```

关键变更：
- 默认值显示为大写首字母（如 `[N]`）
- 输入统一 `tr '[:upper:]' '[:lower:]'` 转小写
- 先尝试完整单词匹配，再尝试首字母展开
- 两者都不命中才 warn 重试

- [ ] **Step 2: 手动验证逻辑正确性**

确认以下映射对：
- `N/n/none/None` → `none`
- `R/r/remote/Remote` → `remote`
- `L/l/local/Local` → `local`
- `I/i/init/Init` → `init`
- `A/a/auto/Auto` → `auto`
- `S/s/systemd/Systemd` → `systemd`
- `C/c/cron/Cron` → `cron`

注意：调度器 `none` 和 `auto/systemd/cron/none` 中 `n` 映射到 `none`（for 循环第一个命中）；需确认 allowed 列表中 `none` 的位置。

当前调度器调用 allowed 为 `"auto systemd cron none"`，首字母 `n` → 命中 `none`（第 4 个），无歧义因为没有其他 `n` 开头的。`a` → `auto`，`s` → `systemd`，`c` → `cron`。均无冲突。

### Task 2: 更新 3 个 prompt_mode 调用标签

**Files:**
- Modify: `src/dist/install.sh:834`
- Modify: `src/dist/install.sh:846`
- Modify: `src/dist/install.sh:876`

- [ ] **Step 3: 更新本体仓库模式标签**

`install.sh:834`：
```
旧: prompt_mode HOME_REPO_MODE "本体仓库模式(init/remote/local)" "$HOME_REPO_MODE" "init remote local"
新: prompt_mode HOME_REPO_MODE "本体仓库模式 (I)nit/(R)emote/(L)ocal" "$HOME_REPO_MODE" "init remote local"
```

- [ ] **Step 4: 更新任务仓库模式标签**

`install.sh:846`：
```
旧: prompt_mode TASK_REPO_MODE "任务仓库模式(none/remote/local)" "$TASK_REPO_MODE" "none remote local"
新: prompt_mode TASK_REPO_MODE "任务仓库模式 (N)one/(R)emote/(L)ocal" "$TASK_REPO_MODE" "none remote local"
```

- [ ] **Step 5: 更新调度器模式标签**

`install.sh:876`：
```
旧: prompt_mode SCHEDULER "调度器模式(auto/systemd/cron/none)" "$SCHEDULER" "auto systemd cron none"
新: prompt_mode SCHEDULER "调度器模式 (A)uto/(S)ystemd/(C)ron/(N)one" "$SCHEDULER" "auto systemd cron none"
```

### Task 3: local 模式路径即时验证循环

**Files:**
- Modify: `src/dist/install.sh:860-872`

- [ ] **Step 6: 重写 local 分支为验证循环**

将 `install.sh:860-872` 替换为：

```bash
    local)
      while true; do
        prompt_default TASK_REPO_LOCAL "本地任务仓库绝对路径" "$TASK_REPO_LOCAL"
        if [[ -z "$TASK_REPO_LOCAL" ]]; then
          warn "路径不可为空，请输入本地已 clone 的任务仓库绝对路径"
          continue
        fi
        if [[ "$TASK_REPO_LOCAL" != /* ]]; then
          warn "必须为绝对路径: $TASK_REPO_LOCAL"
          continue
        fi
        if [[ ! -d "$TASK_REPO_LOCAL" ]]; then
          warn "目录不存在: $TASK_REPO_LOCAL"
          continue
        fi
        if [[ ! -d "$TASK_REPO_LOCAL/.git" ]]; then
          fail "不是 git 仓库（无 .git 目录）: $TASK_REPO_LOCAL"
        fi
        break
      done
      if [[ -z "$TASK_REPO" ]]; then
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
```

关键变更：
- 空输入 → warn + 重试（不再延迟到 validate_inputs）
- 非绝对路径 → warn + 重试
- 目录不存在 → warn + 重试
- 无 `.git` → 直接 `fail` 终止（决策 #2）
- 通过验证后才继续 owner/repo 推断

- [ ] **Step 7: 确认 validate_inputs 的 local 分支仍有效**

`install.sh:904-909` 中的 `validate_inputs` local 分支校验仍保留作为兜底：
- `[[ -n "$TASK_REPO_LOCAL" ]]` — 仍有意义（防 `--yes` + 空默认值）
- `normalize_repo_slug "$TASK_REPO"` — 仍有意义

无需修改 `validate_inputs`。

### Task 4: approval 废弃配置自动迁移

**Files:**
- Modify: `src/dist/install.sh:1124-1127`

- [ ] **Step 8: 在 create_config_file 保留已有配置后追加自动迁移**

将 `install.sh:1124-1127` 替换为：

```bash
create_config_file() {
  if [[ -f "$CONFIG_FILE" ]]; then
    log "保留已有普通配置: $CONFIG_FILE"
    if [[ -x "$APP_DIR/bin/ccclaw" ]]; then
      local migrate_out
      migrate_out="$("$APP_DIR/bin/ccclaw" --config "$CONFIG_FILE" config migrate-approval 2>&1)" \
        || fail "自动迁移废弃 approval 配置失败: $migrate_out"
      log "$migrate_out"
    fi
    return 0
  fi
```

关键变更：
- 保留已有配置后，立即检查 ccclaw 二进制是否可用
- 调用 `ccclaw config migrate-approval`
- 失败 → `fail` 终止安装（决策 #3）
- 成功或无需迁移 → log 输出并继续

- [ ] **Step 9: 确认执行顺序**

`create_app_layout()` 调用链：
1. `install -m 755 "$DIST_DIR/bin/ccclaw" "$APP_DIR/bin/ccclaw"` — 先安装二进制 ✓
2. `create_config_file` — 此时二进制已就位，可调用 migrate-approval ✓

顺序正确，无需调整。

### Task 5: 提交

- [ ] **Step 10: 运行 shellcheck（如可用）**

```bash
shellcheck src/dist/install.sh || true
```

- [ ] **Step 11: 提交**

```bash
git add src/dist/install.sh
git commit -m "fix(install): prompt_mode 缩写化 + local 路径即时验证 + approval 自动迁移

closes #18"
```
