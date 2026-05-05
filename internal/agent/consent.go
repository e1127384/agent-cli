package agent

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func RequiresConsent(action Action) bool {
	risk := strings.ToLower(action.Risk)
	if risk == "high" || risk == "critical" {
		return true
	}

	riskyTypes := map[string]bool{
		"shell.write":   true,
		"email.send":    true,
		"file.delete":   true,
		"file.write":    true,
		"config.update": true,
		"http.post":     true,
	}
	return riskyTypes[strings.ToLower(action.Type)]
}

func AskConsent(action Action) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println()
	fmt.Println("Manual intervention required.")
	fmt.Printf("Action: %s\n", action.Description)
	fmt.Printf("Risk: %s\n", action.Risk)
	if action.Tool != "" {
		fmt.Printf("Tool: %s\n", action.Tool)
	}
	if len(action.Args) > 0 {
		fmt.Println("Arguments:")
		for k, v := range action.Args {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
	fmt.Println()
	fmt.Println("Approve this action?")
	fmt.Println("[1] Approve once")
	fmt.Println("[2] Reject")
	fmt.Print("> ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	return input == "1" || strings.EqualFold(input, "yes") || strings.EqualFold(input, "y")
}
