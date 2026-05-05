package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Agent   AgentConfig
	LLM     LLMConfig
	Skills  SkillsConfig
	Tools   ToolsConfig
	Monitor MonitorConfig
	Audit   AuditConfig
	Tasks   []TaskConfig
}

type AgentConfig struct {
	Name                      string
	RequireConsentForHighRisk bool
}

type LLMConfig struct {
	BaseURL        string
	Model          string
	Temperature    float64
	TimeoutSeconds int
}

type SkillsConfig struct {
	ReleasePath         string
	DraftPath           string
	AllowDraftExecution bool
}

type ToolsConfig struct {
	Shell ShellToolConfig
	HTTP  HTTPToolConfig
	File  FileToolConfig
}

type ShellToolConfig struct {
	Enabled               bool
	ReadonlyOnlyByDefault bool
}

type HTTPToolConfig struct {
	Enabled bool
}

type FileToolConfig struct {
	Enabled              bool
	WriteRequiresConsent bool
}

type MonitorConfig struct {
	Enabled     bool
	TickSeconds int
	ReportPath  string
}

type AuditConfig struct {
	LogPath string
}

type TaskConfig struct {
	Name     string
	Skill    string
	Enabled  bool
	Interval string
	Risk     string
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := Config{}
	applyDefaults(&cfg)
	if err := parseSimpleYAML(string(b), &cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = "http://localhost:8000/v1"
	}
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = "qwen3"
	}
	if cfg.LLM.TimeoutSeconds == 0 {
		cfg.LLM.TimeoutSeconds = 120
	}
	if cfg.Skills.ReleasePath == "" {
		cfg.Skills.ReleasePath = "./skills/release"
	}
	if cfg.Monitor.TickSeconds == 0 {
		cfg.Monitor.TickSeconds = 10
	}
	if cfg.Monitor.ReportPath == "" {
		cfg.Monitor.ReportPath = "./reports"
	}
	if cfg.Audit.LogPath == "" {
		cfg.Audit.LogPath = "./logs/agent.log"
	}
}

func validate(cfg *Config) error {
	for _, t := range cfg.Tasks {
		if t.Name == "" || t.Skill == "" {
			return fmt.Errorf("task must have name and skill")
		}
		if _, err := time.ParseDuration(t.Interval); err != nil {
			return fmt.Errorf("invalid interval for task %s: %w", t.Name, err)
		}
	}
	return nil
}

func parseSimpleYAML(input string, cfg *Config) error {
	section := ""
	subsection := ""
	var currentTask *TaskConfig

	for lineNo, raw := range strings.Split(input, "\n") {
		line := stripComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := countIndent(line)
		trim := strings.TrimSpace(line)

		if indent == 0 && strings.HasSuffix(trim, ":") {
			section = strings.TrimSuffix(trim, ":")
			subsection = ""
			currentTask = nil
			continue
		}

		if section == "tasks" {
			if strings.HasPrefix(trim, "- ") {
				task := TaskConfig{}
				cfg.Tasks = append(cfg.Tasks, task)
				currentTask = &cfg.Tasks[len(cfg.Tasks)-1]
				kv := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
				if kv != "" {
					k, v, ok := splitKV(kv)
					if !ok {
						return fmt.Errorf("invalid task line %d", lineNo+1)
					}
					setTaskField(currentTask, k, v)
				}
				continue
			}
			if currentTask == nil {
				return fmt.Errorf("task field before task item line %d", lineNo+1)
			}
			k, v, ok := splitKV(trim)
			if !ok {
				return fmt.Errorf("invalid task field line %d", lineNo+1)
			}
			setTaskField(currentTask, k, v)
			continue
		}

		if indent == 2 && strings.HasSuffix(trim, ":") {
			subsection = strings.TrimSuffix(trim, ":")
			continue
		}

		k, v, ok := splitKV(trim)
		if !ok {
			return fmt.Errorf("invalid config line %d: %s", lineNo+1, trim)
		}
		if indent == 2 {
			setSectionField(cfg, section, k, v)
		} else if indent == 4 {
			setSubsectionField(cfg, section, subsection, k, v)
		}
	}
	return nil
}

func stripComment(s string) string {
	inQuote := false
	for i, r := range s {
		if r == '"' {
			inQuote = !inQuote
		}
		if r == '#' && !inQuote {
			return s[:i]
		}
	}
	return s
}

func countIndent(s string) int {
	count := 0
	for _, r := range s {
		if r == ' ' {
			count++
		} else {
			break
		}
	}
	return count
}

func splitKV(s string) (string, string, bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), cleanValue(parts[1]), true
}

func cleanValue(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"")
	s = strings.Trim(s, "'")
	return s
}

func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}

func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func setSectionField(cfg *Config, section, key, value string) {
	switch section {
	case "agent":
		switch key {
		case "name":
			cfg.Agent.Name = value
		case "require_consent_for_high_risk":
			cfg.Agent.RequireConsentForHighRisk = parseBool(value)
		}
	case "llm":
		switch key {
		case "base_url":
			cfg.LLM.BaseURL = value
		case "model":
			cfg.LLM.Model = value
		case "temperature":
			cfg.LLM.Temperature = parseFloat(value)
		case "timeout_seconds":
			cfg.LLM.TimeoutSeconds = parseInt(value)
		}
	case "skills":
		switch key {
		case "release_path":
			cfg.Skills.ReleasePath = value
		case "draft_path":
			cfg.Skills.DraftPath = value
		case "allow_draft_execution":
			cfg.Skills.AllowDraftExecution = parseBool(value)
		}
	case "monitor":
		switch key {
		case "enabled":
			cfg.Monitor.Enabled = parseBool(value)
		case "tick_seconds":
			cfg.Monitor.TickSeconds = parseInt(value)
		case "report_path":
			cfg.Monitor.ReportPath = value
		}
	case "audit":
		if key == "log_path" {
			cfg.Audit.LogPath = value
		}
	}
}

func setSubsectionField(cfg *Config, section, subsection, key, value string) {
	if section != "tools" {
		return
	}
	switch subsection {
	case "shell":
		switch key {
		case "enabled":
			cfg.Tools.Shell.Enabled = parseBool(value)
		case "readonly_only_by_default":
			cfg.Tools.Shell.ReadonlyOnlyByDefault = parseBool(value)
		}
	case "http":
		if key == "enabled" {
			cfg.Tools.HTTP.Enabled = parseBool(value)
		}
	case "file":
		switch key {
		case "enabled":
			cfg.Tools.File.Enabled = parseBool(value)
		case "write_requires_consent":
			cfg.Tools.File.WriteRequiresConsent = parseBool(value)
		}
	}
}

func setTaskField(t *TaskConfig, key, value string) {
	switch key {
	case "name":
		t.Name = value
	case "skill":
		t.Skill = value
	case "enabled":
		t.Enabled = parseBool(value)
	case "interval":
		t.Interval = value
	case "risk":
		t.Risk = value
	}
}
