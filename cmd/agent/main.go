package main

import (
	"context"
	"fmt"
	"os"

	"agent-cli/internal/config"
	"agent-cli/internal/evaluator"
)

func main() {
	cfgPath := "config.yaml"
	args := os.Args[1:]

	for len(args) >= 2 {
		if args[0] == "--config" {
			cfgPath = args[1]
			args = args[2:]
			continue
		}
		break
	}

	if len(args) == 0 {
		runEvaluation(cfgPath, "")
		return
	}

	switch args[0] {
	case "evaluate":
		caseFileOverride := ""
		if len(args) == 3 && args[1] == "--cases" {
			caseFileOverride = args[2]
		} else if len(args) != 1 {
			exitErr(fmt.Errorf("usage: agent [--config config.yaml] evaluate [--cases /path/to/case_ids.txt]"))
		}
		runEvaluation(cfgPath, caseFileOverride)
	case "--help", "help", "-h":
		printHelp()
	default:
		exitErr(fmt.Errorf("unknown command: %s", args[0]))
	}
}

func runEvaluation(cfgPath, caseFileOverride string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		exitErr(err)
	}
	if caseFileOverride != "" {
		cfg.Input.CaseIDsFile = caseFileOverride
	}

	runner := evaluator.NewRunner(cfg)
	runDir, err := runner.Run(context.Background())
	if err != nil {
		exitErr(err)
	}

	fmt.Println("Evaluation completed.")
	fmt.Println("Artifacts saved in:", runDir)
	fmt.Println("Run summary:", runDir+"/run_summary.json")
}

func printHelp() {
	fmt.Print(`Automated LLM-as-a-Judge Summary Quality Evaluation Framework

Usage:
  agent [--config config.yaml]
  agent [--config config.yaml] evaluate
  agent [--config config.yaml] evaluate --cases /absolute/path/case_ids.txt

Description:
  Runs non-interactive batch evaluation of whistleblowing case summaries:
  - Authenticates to API
  - Fetches case JSON and generated summaries per case ID
  - Evaluates summary quality with a locally hosted LLM judge
  - Stores per-case artifacts and run_summary.json
`)
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
