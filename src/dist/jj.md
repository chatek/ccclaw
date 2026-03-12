# Jujutsu (jj) 速查

## 官方资源

- 官网: https://jj-vcs.github.io/jj/
- GitHub: https://github.com/jj-vcs/jj
- 教程: https://jj-vcs.github.io/jj/latest/tutorial/

## 日常命令对照

| git | jj |
| --- | --- |
| `git status` | `jj status` / `jj st` |
| `git diff` | `jj diff` |
| `git add <path>` | `jj file track <path>` |
| `git commit -m "msg"` | `jj commit -m "msg"` |
| `git log --oneline` | `jj log` |
| `git pull --rebase` | `jj git fetch` + `jj rebase -d main@origin` |
| `git push origin main` | `jj git push --remote origin --bookmark main` |
| `git branch -a` | `jj bookmark list` |

## 冲突处理

1. `jj git fetch --remote origin`
2. `jj rebase -d main@origin`
3. `jj log -r 'conflicts()' --count --no-graph`
4. 若有冲突，编辑文件后执行 `jj resolve`
5. 复核 `jj diff --summary`
6. 完成后执行 `jj git push --remote origin --bookmark main`
