package tools

import (
	"context"
	"os"
)

type FileReadTool struct{}

func (FileReadTool) Name() string { return "file.read" }

func (FileReadTool) Run(ctx context.Context, args map[string]string) ToolResult {
	path := args["path"]
	if path == "" {
		return ToolResult{Name: "file.read", Error: "missing path"}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Name: "file.read", Error: err.Error()}
	}
	return ToolResult{Name: "file.read", Output: string(b)}
}

type FileWriteTool struct{}

func (FileWriteTool) Name() string { return "file.write" }

func (FileWriteTool) Run(ctx context.Context, args map[string]string) ToolResult {
	path := args["path"]
	content := args["content"]
	if path == "" {
		return ToolResult{Name: "file.write", Error: "missing path"}
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return ToolResult{Name: "file.write", Error: err.Error()}
	}
	return ToolResult{Name: "file.write", Output: "file written: " + path}
}
