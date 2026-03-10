# 260310_NA_readme_restructure

## 背景

本次变更不是补代码能力，而是按当前仓库真实状态，重构根目录 README。

明确输入约束来自：

- 仓库治理文件 `AGENTS.md`
- 已落地实现与发布资产
- 已公开的项目决策 Issue：
  - #3 `RFC: 以开源发布为目标重构 CCClaw 架构、阶段路线与目录布局`
  - #6 `phase0.3: 项目状态清查、治理文件同步与统一 release 发布流`
  - #7 `phase0.4: 首个可正确下载安装 release 的交付收口`

额外核对到的当前事实：

- 当前仓库已经存在 release：`26.3.10.1142`
- 当前 release 安装流以 `src/dist/` 为打包源
- `install.sh` 已支持本体仓库 `init|remote|local`
- `install.sh` 已支持任务仓库 `none|remote|local`
- 默认安装目录仍为 `~/.ccclaw`
- 默认本体仓库仍为 `/opt/ccclaw`

## 参考材料

本次只借鉴其它 `*claw` 项目的 README 结构组织方式，不借用其项目事实或功能描述。

参考了以下公开 README 的常见组织方式：

1. `jamesrochabrun/Claw`
   - 首屏一句话说明产品定位
   - 紧接 Requirements 与 Features
2. `openclaw/clawhub`
   - 首屏入口导航清晰
   - 采用标准的 `What / How / CLI / Environment / Repo layout` 结构
3. `HKUDS/ClawWork`
   - 首屏突出差异化价值
   - 在长 README 中明确拆分 `Quick Start / Architecture / Configuration / Troubleshooting / Contributing`

## 改写目标

根 README 需要同时满足以下几点：

1. 中英分离
   - `README.md` 为中文主版本
   - `README_en.md` 为英文版本
2. 使用标准开源项目自述结构
   - 项目简介
   - 核心特点
   - 适用场景
   - 安装前准备
   - 下载与安装
   - 配置
   - 日常使用
   - 升级与发布
   - 已知边界
   - 许可证
3. 反映当前真实阶段
   - 不再沿用旧版 README 中过时的 `phase0.2` 表述
   - 改为按 `phase0.4` 的 release 与安装能力描述
4. 强调 `ccclaw` 的差异化
   - GitHub Issue 驱动
   - systemd --user 调度
   - 记忆仓库与程序树分离
   - 节省 token 消耗
   - 管理员 gate
5. 详细说明下载、校验、解压、安装、绑定、启用流程
6. 显式列出用户要求的安装前准备
   - GitHub 帐号
   - `gh` token
   - Claude Code 订阅与登录路径
   - 或 Claude 代理 API 的 URL + token
   - 地区与网络条件

## 关键取舍

### 1. 保留“地区/网络条件”，但不提供绕过方法

用户要求 README 提前写明 Claude Code 的地区与代理前提。

本次处理方式：

- 明确写出“推荐位于 Claude Code 官方允许使用的国家/地区”
- 明确写出“若不在允许范围内，需自行确认网络出口、账号使用方式和服务条款是否合规”
- 不写任何“绕过区域校验”的操作细节

原因：

- 这能满足安装准备信息披露的目标
- 同时避免 README 变成规避平台限制的操作手册

### 2. README 只写已经落地的能力

尽管 #3 中已经讨论了 phase1/phase2 的多 provider、Matrix 等方向，本次 README 不把这些路线图混入“当前能力”。

只保留：

- 已实现的 release
- 已实现的安装脚本参数与模式
- 已实现的 `.env + .toml`
- 已实现的 `systemd --user`
- 已实现的 gate 规则

### 3. 英文版与中文版结构对齐，但以中文为主

受用户要求影响，本次新增 `README_en.md`。

考虑到仓库治理默认偏中文，本次没有将英文版扩展得比中文版更长，而是保持章节结构和技术口径一致，避免双语内容漂移。

## 本次改动

### 1. 重写根 `README.md`

新 README 新增或强化了以下内容：

- 用户指定的 ASCII logo
- 英文版跳转链接
- `CCClaw / 3Claw` 的缩写解释
- “专注节省 token 消耗”的首屏定位
- 标准开源 README 章节结构
- 当前 `phase0.4` 状态说明
- 详细的 release 下载、校验、解压、安装说明
- Claude Code 账号、代理、地区、网络前置条件
- 真实安装参数示例
- 配置文件位置与职责
- 开源协作门禁流程
- 升级与发布入口

### 2. 新增 `README_en.md`

英文版包含与中文版一致的核心章节：

- Overview
- Key Strengths
- Best Fit
- Current Status
- Prerequisites
- Download and Install
- Configuration
- Daily Workflow
- Upgrade and Release
- Current Boundaries

### 3. 保持现有代码与脚本不变

本轮只改文档，不涉及：

- Go 代码
- 安装脚本逻辑
- Makefile 逻辑
- release 资产内容

## 结果

根目录 README 现在更接近一个标准开源项目应有的入口文档：

- 首屏能快速回答“这是什么、为什么存在、适合谁”
- 安装章节能直接支撑 release 下载安装
- 不再把旧阶段描述误写为当前状态
- 双语文档拆分清晰，中文默认入口明确

## 后续建议

1. 若下一轮 release tag 继续推进，建议在 README 中增加一个稳定的“最新 release 安装”短链或脚本片段
2. 若后续进入 phase1，多 provider 支持应放到独立章节，避免冲淡当前 Claude Code-first 的安装说明
3. 若仓库后续公开，建议再补：
   - `CONTRIBUTING.md`
   - `SECURITY.md`
   - 更完整的 `docs/` 导航页
