package tools

import (
	"context"
	"fmt"
)

type ToolRequest struct {
	Name string            `json:"name"`
	Args map[string]string `json:"args"`
}

type ToolResult struct {
	Name   string `json:"name"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

type Tool interface {
	Name() string
	Run(ctx context.Context, args map[string]string) ToolResult
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

func (r *Registry) Run(ctx context.Context, req ToolRequest) ToolResult {
	t, ok := r.tools[req.Name]
	if !ok {
		return ToolResult{Name: req.Name, Error: fmt.Sprintf("tool not registered: %s", req.Name)}
	}
	return t.Run(ctx, req.Args)
}

func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(ShellReadonlyTool{})
	r.Register(HTTPGetTool{})
	r.Register(FileReadTool{})
	r.Register(FileWriteTool{})
	return r
}
