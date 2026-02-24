// ccclaw status — 查询任务状态
package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/NOTAschool/ccclaw/internal/task"
)

func main() {
	stateDir := envOr("CCCLAW_STATE_DIR", "/var/lib/ccclaw")

	store, err := task.NewStore(stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化状态存储失败: %v\n", err)
		os.Exit(1)
	}

	tasks, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取任务列表失败: %v\n", err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		fmt.Println("暂无任务")
		return
	}

	// 统计
	counts := map[task.State]int{}
	for _, t := range tasks {
		counts[t.State]++
	}

	fmt.Printf("任务总数: %d\n", len(tasks))
	for _, s := range []task.State{
		task.StateNew, task.StateTriaged, task.StateRunning,
		task.StateBlocked, task.StateFailed, task.StateDone,
		task.StateDead, task.StateArchived,
	} {
		if n := counts[s]; n > 0 {
			fmt.Printf("  %-10s %d\n", s, n)
		}
	}
	fmt.Println()

	// 详细列表
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ISSUE\tSTATE\tRISK\tRETRY\tTITLE")
	for _, t := range tasks {
		title := t.IssueTitle
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		fmt.Fprintf(w, "#%d\t%s\t%s\t%d\t%s\n",
			t.IssueNumber, t.State, t.RiskLevel, t.RetryCount, title)
	}
	_ = w.Flush()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
