# 260310_11_issue8_followup_planning

## 背景

用户要求：

- 使用 `gh` 提取 <https://github.com/41490/ccclaw/issues/8> 的最后优化建议
- 基于该建议新开 Issue，转入独立策划讨论

经核查，Issue #8 已关闭；其最后一条评论中明确给出下一步继续优化的 4 个方向，适合拆为新议题推进。

## Issue #8 最后优化建议

最后一条评论中的后续建议为：

1. 给 `install.sh` 补 shell 级自动化回归测试，至少覆盖首装、幂等重装、systemd 降级、local 无 `origin`、remote 路径越界五类场景
2. 在 `doctor` 中显式输出调度器降级状态，避免安装后还要靠用户自己猜当前是 `systemd`、`cron` 还是 `none`
3. 若后续要真正支持 `cron`，补自动生成或安装用户 crontab 的受控入口，而不只停留在样板提示
4. 继续收口 README 与 release 说明，确保“控制仓库 / 本体仓库 / 任务仓库”三类边界在安装前就讲清楚

## 本次操作

### 1. 去重核查

使用 `gh issue list --state open` 检查现有 open issue，未发现与安装链路二轮优化直接重复的议题。

### 2. 新建策划 Issue

已使用 `gh issue create` 新建：

- Issue #11 `plan: 安装链路第二轮优化策划`
- 链接：<https://github.com/41490/ccclaw/issues/11>

新 Issue 中已整理：

- 背景来源：承接 Issue #8 最后一条评论
- 分期范围：回归测试、doctor 输出、cron 受控落地、文档与发布说明收口
- 验收标准：可回归、可诊断、可降级、文档术语一致
- 待拍板点：测试框架形式、`cron` 写入策略、推进优先级

## 结论

Issue #8 的最后优化建议已成功提炼并转为独立策划议题，后续讨论可直接在 Issue #11 中集中拍板，不再回到已关闭的 Issue #8 混合滚动。
