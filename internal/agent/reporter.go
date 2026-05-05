package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func WriteReport(reportPath string, result SkillResult) (string, error) {
	if err := os.MkdirAll(reportPath, 0755); err != nil {
		return "", err
	}

	safeName := strings.ReplaceAll(result.SkillName, " ", "-")
	file := filepath.Join(reportPath, fmt.Sprintf("%s-%s.md", time.Now().Format("20060102-150405"), safeName))

	var b strings.Builder
	b.WriteString("# Agent Skill Report\n\n")
	b.WriteString(fmt.Sprintf("- Time: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Skill: %s\n", result.SkillName))
	b.WriteString(fmt.Sprintf("- Status: %s\n", result.Status))
	b.WriteString(fmt.Sprintf("- Risk Level: %s\n", result.RiskLevel))
	b.WriteString(fmt.Sprintf("- Consent Required: %v\n\n", result.ConsentRequired))

	b.WriteString("## Findings\n")
	if len(result.Findings) == 0 {
		b.WriteString("- No findings.\n")
	} else {
		for _, f := range result.Findings {
			b.WriteString("- " + f + "\n")
		}
	}

	b.WriteString("\n## Recommended Action\n")
	if result.RecommendedAction == "" {
		b.WriteString("No action recommended.\n")
	} else {
		b.WriteString(result.RecommendedAction + "\n")
	}

	if result.LLMAnalysis != "" {
		b.WriteString("\n## LLM Analysis\n")
		b.WriteString(result.LLMAnalysis + "\n")
	}

	if len(result.ToolResults) > 0 {
		b.WriteString("\n## Tool Results\n")
		for _, tr := range result.ToolResults {
			b.WriteString(fmt.Sprintf("\n### %s\n", tr.Name))
			if tr.Error != "" {
				b.WriteString("Error: " + tr.Error + "\n")
			}
			if tr.Output != "" {
				b.WriteString("```\n")
				b.WriteString(limit(tr.Output, 4000))
				b.WriteString("\n```\n")
			}
		}
	}

	return file, os.WriteFile(file, []byte(b.String()), 0644)
}

func PrintResult(result SkillResult) {
	fmt.Println()
	fmt.Println("Skill Report")
	fmt.Println("============")
	fmt.Printf("Skill: %s\n", result.SkillName)
	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("Risk: %s\n", result.RiskLevel)
	fmt.Println()
	fmt.Println("Findings:")
	if len(result.Findings) == 0 {
		fmt.Println("- No findings")
	} else {
		for _, f := range result.Findings {
			fmt.Println("- " + f)
		}
	}
	fmt.Println()
	fmt.Println("Recommended Action:")
	if result.RecommendedAction == "" {
		fmt.Println("No action recommended.")
	} else {
		fmt.Println(result.RecommendedAction)
	}
	fmt.Println()
	fmt.Printf("Consent Required: %v\n", result.ConsentRequired)
}

func limit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... truncated ..."
}
