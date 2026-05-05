package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"agent-cli/internal/audit"
	"agent-cli/internal/config"
	"agent-cli/internal/llm"
	"agent-cli/internal/skills"
	"agent-cli/internal/tools"
)

type Runtime struct {
	Config *config.Config
	LLM    *llm.Client
	Skills map[string]skills.Skill
	Tools  *tools.Registry
	Audit  *audit.Logger
}

func NewRuntime(cfg *config.Config, loadedSkills map[string]skills.Skill) *Runtime {
	return &Runtime{
		Config: cfg,
		LLM:    llm.New(cfg.LLM.BaseURL, cfg.LLM.Model, cfg.LLM.Temperature, cfg.LLM.TimeoutSeconds),
		Skills: loadedSkills,
		Tools:  tools.DefaultRegistry(),
		Audit:  audit.New(cfg.Audit.LogPath),
	}
}

func (r *Runtime) RunNatural(ctx context.Context, userRequest string) (SkillResult, error) {
	skillName, err := r.SelectSkill(ctx, userRequest)
	if err != nil {
		return SkillResult{}, err
	}
	return r.RunSkill(ctx, skillName)
}

func (r *Runtime) SelectSkill(ctx context.Context, userRequest string) (string, error) {
	var names []string
	for name := range r.Skills {
		names = append(names, name)
	}
	prompt := fmt.Sprintf(`You are a CLI agent skill router.
Available skills: %s
User request: %s
Return only the exact skill name that best matches.`, strings.Join(names, ", "), userRequest)

	out, err := r.LLM.Chat(ctx, []llm.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return "", err
	}
	selected := strings.TrimSpace(out)
	selected = strings.Trim(selected, "` \n\t\"")
	if _, ok := r.Skills[selected]; ok {
		return selected, nil
	}

	for _, n := range names {
		if strings.Contains(selected, n) {
			return n, nil
		}
	}
	return "", fmt.Errorf("LLM selected unknown skill: %s", selected)
}

func (r *Runtime) RunSkill(ctx context.Context, skillName string) (SkillResult, error) {
	skill, ok := r.Skills[skillName]
	if !ok {
		return SkillResult{}, fmt.Errorf("skill not found: %s", skillName)
	}

	r.Audit.Log("skill.start", "skill execution started", map[string]interface{}{"skill": skillName})

	prompt := fmt.Sprintf(`You are a local CLI agent.
You must run this skill safely and produce JSON only.

Skill metadata:
Name: %s
Risk: %s
Requires consent: %v
Allowed tools: %s

Skill body:
%s

Return JSON matching this structure:
{
  "status": "OK|Warning|Critical",
  "findings": ["finding 1"],
  "risk_level": "low|medium|high|critical",
  "recommended_action": "clear next action",
  "consent_required": false,
  "tool_requests": [
    {"name":"shell.readonly", "args":{"command":"date"}}
  ],
  "proposed_action": {
    "type":"shell.write",
    "description":"Restart service X",
    "risk":"high",
    "tool":"shell.write",
    "args":{"command":"systemctl restart x"}
  }
}

Rules:
- Only request tools listed in allowed_tools.
- Prefer safe read-only tools.
- If a write/risky action is needed, put it in proposed_action and set consent_required true.
- Do not invent tool names.
- JSON only.`, skill.Name, skill.Risk, skill.RequiresConsent, strings.Join(skill.AllowedTools, ", "), skill.Body)

	out, err := r.LLM.Chat(ctx, []llm.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return SkillResult{}, err
	}

	plan, err := parsePlan(out)
	if err != nil {
		return SkillResult{
			SkillName:         skill.Name,
			Status:            "Warning",
			Findings:          []string{"LLM response could not be parsed as JSON."},
			RiskLevel:         skill.Risk,
			RecommendedAction: "Review the LLM output manually.",
			ConsentRequired:   false,
			LLMAnalysis:       out,
		}, nil
	}

	result := SkillResult{
		SkillName:         skill.Name,
		Status:            plan.Status,
		Findings:          plan.Findings,
		RiskLevel:         plan.RiskLevel,
		RecommendedAction: plan.RecommendedAction,
		ConsentRequired:   plan.ConsentRequired || skill.RequiresConsent,
		LLMAnalysis:       out,
	}

	for _, req := range plan.ToolRequests {
		if !toolAllowed(req.Name, skill.AllowedTools) {
			result.ToolResults = append(result.ToolResults, tools.ToolResult{Name: req.Name, Error: "tool not allowed by skill"})
			continue
		}
		tr := r.Tools.Run(ctx, req)
		result.ToolResults = append(result.ToolResults, tr)
		r.Audit.Log("tool.run", "tool executed", map[string]interface{}{"skill": skill.Name, "tool": req.Name, "error": tr.Error})
	}

	if plan.ProposedAction != nil {
		result.ProposedAction = plan.ProposedAction
		if RequiresConsent(*plan.ProposedAction) {
			result.ConsentRequired = true
			approved := AskConsent(*plan.ProposedAction)
			r.Audit.Log("consent.decision", "user consent decision", map[string]interface{}{"skill": skill.Name, "approved": approved, "action": plan.ProposedAction.Description})
			if approved {
				if plan.ProposedAction.Tool != "" && r.Tools.Has(plan.ProposedAction.Tool) {
					tr := r.Tools.Run(ctx, tools.ToolRequest{Name: plan.ProposedAction.Tool, Args: plan.ProposedAction.Args})
					result.ToolResults = append(result.ToolResults, tr)
				} else {
					result.Findings = append(result.Findings, "Approved action has no executable registered tool; saved as manual recommendation.")
				}
			} else {
				result.Findings = append(result.Findings, "User rejected proposed manual action.")
			}
		}
	}

	r.Audit.Log("skill.end", "skill execution completed", map[string]interface{}{"skill": skill.Name, "status": result.Status})
	return result, nil
}

type llmPlan struct {
	Status            string              `json:"status"`
	Findings          []string            `json:"findings"`
	RiskLevel         string              `json:"risk_level"`
	RecommendedAction string              `json:"recommended_action"`
	ConsentRequired   bool                `json:"consent_required"`
	ToolRequests      []tools.ToolRequest `json:"tool_requests"`
	ProposedAction    *Action             `json:"proposed_action"`
}

func parsePlan(s string) (llmPlan, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	var p llmPlan
	err := json.Unmarshal([]byte(s), &p)
	if p.Status == "" {
		p.Status = "OK"
	}
	if p.RiskLevel == "" {
		p.RiskLevel = "low"
	}
	return p, err
}

func toolAllowed(name string, allowed []string) bool {
	for _, a := range allowed {
		if a == name {
			return true
		}
	}
	return false
}
