package tools

import (
	"context"
	"io"
	"net/http"
	"time"
)

type HTTPGetTool struct{}

func (HTTPGetTool) Name() string { return "http.get" }

func (HTTPGetTool) Run(ctx context.Context, args map[string]string) ToolResult {
	url := args["url"]
	if url == "" {
		return ToolResult{Name: "http.get", Error: "missing url"}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ToolResult{Name: "http.get", Error: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ToolResult{Name: "http.get", Error: err.Error()}
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return ToolResult{Name: "http.get", Error: err.Error()}
	}
	return ToolResult{Name: "http.get", Output: string(b)}
}
