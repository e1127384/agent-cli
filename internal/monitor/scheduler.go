package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"agent-cli/internal/agent"
	"agent-cli/internal/config"
)

type Task struct {
	Name     string
	Skill    string
	Enabled  bool
	Interval time.Duration
	Risk     string
	LastRun  time.Time
	NextRun  time.Time
	Running  bool
	mu       sync.Mutex
}

func BuildTasks(cfgTasks []config.TaskConfig) ([]*Task, error) {
	var tasks []*Task
	now := time.Now()
	for _, ct := range cfgTasks {
		d, err := time.ParseDuration(ct.Interval)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, &Task{
			Name:     ct.Name,
			Skill:    ct.Skill,
			Enabled:  ct.Enabled,
			Interval: d,
			Risk:     ct.Risk,
			NextRun:  now,
		})
	}
	return tasks, nil
}

func Start(ctx context.Context, rt *agent.Runtime, tasks []*Task, tick time.Duration) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	fmt.Println("Agent monitor started.")
	fmt.Println("Loaded tasks:")
	for _, t := range tasks {
		if t.Enabled {
			fmt.Printf("- %-30s every %s\n", t.Name, t.Interval)
		}
	}
	fmt.Println("Press Ctrl+C to stop.")

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Agent monitor stopped.")
			return
		case <-ticker.C:
			now := time.Now()
			for _, task := range tasks {
				if !task.Enabled {
					continue
				}
				task.mu.Lock()
				due := !task.Running && (now.After(task.NextRun) || now.Equal(task.NextRun))
				if due {
					task.Running = true
				}
				task.mu.Unlock()

				if due {
					go runTask(ctx, rt, task)
				}
			}
		}
	}
}

func RunOnce(ctx context.Context, rt *agent.Runtime, tasks []*Task) {
	for _, t := range tasks {
		if t.Enabled {
			runTask(ctx, rt, t)
		}
	}
}

func RunSingle(ctx context.Context, rt *agent.Runtime, tasks []*Task, name string) error {
	for _, t := range tasks {
		if t.Name == name || t.Skill == name {
			runTask(ctx, rt, t)
			return nil
		}
	}
	return fmt.Errorf("monitor task not found: %s", name)
}

func PrintStatus(tasks []*Task) {
	fmt.Println("Monitor task status:")
	for _, t := range tasks {
		status := "disabled"
		if t.Enabled {
			status = "enabled"
		}
		if t.Running {
			status = "running"
		}
		fmt.Printf("- %-30s %-10s interval=%s last=%s next=%s\n", t.Name, status, t.Interval, formatTime(t.LastRun), formatTime(t.NextRun))
	}
}

func runTask(ctx context.Context, rt *agent.Runtime, task *Task) {
	start := time.Now()
	fmt.Printf("[%s] Running %s...\n", start.Format("15:04:05"), task.Name)

	defer func() {
		task.mu.Lock()
		task.Running = false
		task.LastRun = time.Now()
		task.NextRun = task.LastRun.Add(task.Interval)
		next := task.NextRun
		task.mu.Unlock()
		fmt.Printf("[%s] Next run for %s: %s\n", time.Now().Format("15:04:05"), task.Name, next.Format("15:04:05"))
	}()

	result, err := rt.RunSkill(ctx, task.Skill)
	if err != nil {
		fmt.Printf("[%s] ERROR %s: %v\n", time.Now().Format("15:04:05"), task.Name, err)
		return
	}
	agent.PrintResult(result)
	file, err := agent.WriteReport(rt.Config.Monitor.ReportPath, result)
	if err != nil {
		fmt.Printf("Report write error: %v\n", err)
		return
	}
	fmt.Printf("Report saved: %s\n", file)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("15:04:05")
}
