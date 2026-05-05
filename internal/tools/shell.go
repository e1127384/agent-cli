package tools

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type ShellReadonlyTool struct{}

func (ShellReadonlyTool) Name() string { return "shell.readonly" }

func (ShellReadonlyTool) Run(ctx context.Context, args map[string]string) ToolResult {
	cmdLine := strings.TrimSpace(args["command"])
	if cmdLine == "" {
		return ToolResult{Name: "shell.readonly", Error: "missing command"}
	}

	if !isReadonlyCommand(cmdLine) {
		return ToolResult{Name: "shell.readonly", Error: "command blocked by readonly allowlist"}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sh", "-c", cmdLine).CombinedOutput()
	res := ToolResult{Name: "shell.readonly", Output: string(out)}
	if err != nil {
		res.Error = err.Error()
	}
	return res
}

func isReadonlyCommand(cmdLine string) bool {
	blocked := []string{" rm ", "rm -", " mv ", " chmod ", " chown ", " kill ", " reboot", " shutdown", " systemctl restart", "docker stop", "podman stop", ">", "| sh", "| bash"}
	padded := " " + strings.ToLower(cmdLine) + " "
	for _, b := range blocked {
		if strings.Contains(padded, b) {
			return false
		}
	}

	allowedPrefixes := []string{"ls", "pwd", "cat", "grep", "tail", "head", "df", "du", "ps", "whoami", "date", "uptime", "curl -s", "curl --silent", "echo"}
	fields := strings.Fields(cmdLine)
	if len(fields) == 0 {
		return false
	}
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(cmdLine, p) {
			return true
		}
	}
	return false
}
