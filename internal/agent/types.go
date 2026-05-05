package agent

import "agent-cli/internal/tools"

type Action struct {
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Risk        string            `json:"risk"`
	Tool        string            `json:"tool"`
	Args        map[string]string `json:"args"`
}

type SkillResult struct {
	SkillName         string             `json:"skill_name"`
	Status            string             `json:"status"`
	Findings          []string           `json:"findings"`
	RiskLevel         string             `json:"risk_level"`
	RecommendedAction string             `json:"recommended_action"`
	ConsentRequired   bool               `json:"consent_required"`
	ProposedAction    *Action            `json:"proposed_action,omitempty"`
	ToolResults       []tools.ToolResult `json:"tool_results,omitempty"`
	LLMAnalysis       string             `json:"llm_analysis,omitempty"`
}
