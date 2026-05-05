package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"agent-cli/internal/agent"
	"agent-cli/internal/config"
	"agent-cli/internal/monitor"
	"agent-cli/internal/skills"
)

func main() {
	cfgPath := "config.yaml"
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "--config" {
		cfgPath = args[1]
		args = args[2:]
	}

	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		printHelp()
		return
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		exitErr(err)
	}
	loadedSkills, err := skills.LoadReleaseSkills(cfg.Skills.ReleasePath)
	if err != nil {
		exitErr(err)
	}
	rt := agent.NewRuntime(cfg, loadedSkills)

	ctx := context.Background()

	switch args[0] {
	case "skill":
		handleSkill(ctx, rt, args[1:])
	case "run":
		if len(args) < 2 {
			exitErr(fmt.Errorf("usage: agent run \"your request\""))
		}
		request := strings.Join(args[1:], " ")
		result, err := rt.RunNatural(ctx, request)
		if err != nil {
			exitErr(err)
		}
		agent.PrintResult(result)
		file, err := agent.WriteReport(cfg.Monitor.ReportPath, result)
		if err != nil {
			exitErr(err)
		}
		fmt.Println("Report saved:", file)
	case "monitor":
		handleMonitor(rt, cfg, args[1:])
	default:
		printHelp()
	}
}

func handleSkill(ctx context.Context, rt *agent.Runtime, args []string) {
	if len(args) == 0 {
		exitErr(fmt.Errorf("usage: agent skill list | agent skill run <skill-name>"))
	}
	switch args[0] {
	case "list":
		fmt.Println("Available release skills:")
		for name, s := range rt.Skills {
			fmt.Printf("- %s version=%s risk=%s file=%s\n", name, s.Version, s.Risk, s.FilePath)
		}
	case "run":
		if len(args) < 2 {
			exitErr(fmt.Errorf("usage: agent skill run <skill-name>"))
		}
		result, err := rt.RunSkill(ctx, args[1])
		if err != nil {
			exitErr(err)
		}
		agent.PrintResult(result)
		file, err := agent.WriteReport(rt.Config.Monitor.ReportPath, result)
		if err != nil {
			exitErr(err)
		}
		fmt.Println("Report saved:", file)
	default:
		exitErr(fmt.Errorf("unknown skill command: %s", args[0]))
	}
}

func handleMonitor(rt *agent.Runtime, cfg *config.Config, args []string) {
	if len(args) == 0 {
		exitErr(fmt.Errorf("usage: agent monitor start | run-once | run <task> | status"))
	}
	tasks, err := monitor.BuildTasks(cfg.Tasks)
	if err != nil {
		exitErr(err)
	}

	switch args[0] {
	case "start":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		tick := time.Duration(cfg.Monitor.TickSeconds) * time.Second
		monitor.Start(ctx, rt, tasks, tick)
	case "run-once":
		monitor.RunOnce(context.Background(), rt, tasks)
	case "run":
		if len(args) < 2 {
			exitErr(fmt.Errorf("usage: agent monitor run <task-name>"))
		}
		if err := monitor.RunSingle(context.Background(), rt, tasks, args[1]); err != nil {
			exitErr(err)
		}
	case "status":
		monitor.PrintStatus(tasks)
	default:
		exitErr(fmt.Errorf("unknown monitor command: %s", args[0]))
	}
}

func printHelp() {
	fmt.Print(`Local CLI Agent

Usage:
  agent [--config config.yaml] skill list
  agent [--config config.yaml] skill run <skill-name>
  agent [--config config.yaml] run "natural language request"
  agent [--config config.yaml] monitor start
  agent [--config config.yaml] monitor run-once
  agent [--config config.yaml] monitor run <task-name>
  agent [--config config.yaml] monitor status

Examples:
  agent skill list
  agent skill run mq-health-check
  agent run "check MQ health"
  agent monitor start
`)
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
