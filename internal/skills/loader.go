package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name            string
	Version         string
	Status          string
	Risk            string
	RequiresConsent bool
	AllowedTools    []string
	Body            string
	FilePath        string
}

func LoadReleaseSkills(path string) (map[string]Skill, error) {
	result := map[string]Skill{}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		skill, err := LoadSkill(filepath.Join(path, e.Name()))
		if err != nil {
			return nil, err
		}
		if skill.Status != "release" {
			continue
		}
		result[skill.Name] = skill
	}
	return result, nil
}

func LoadSkill(path string) (Skill, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	text := string(b)
	if !strings.HasPrefix(text, "---") {
		return Skill{}, fmt.Errorf("skill %s missing YAML front matter", path)
	}

	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return Skill{}, fmt.Errorf("skill %s has invalid front matter", path)
	}

	s, err := parseFrontMatter(parts[1])
	if err != nil {
		return Skill{}, fmt.Errorf("parse skill %s: %w", path, err)
	}
	s.Body = strings.TrimSpace(parts[2])
	s.FilePath = path

	if s.Name == "" {
		return Skill{}, fmt.Errorf("skill %s missing name", path)
	}
	if s.Version == "" {
		s.Version = "0.0.1"
	}
	if s.Risk == "" {
		s.Risk = "low"
	}
	return s, nil
}

func parseFrontMatter(input string) (Skill, error) {
	s := Skill{}
	currentList := ""
	for _, raw := range strings.Split(input, "\n") {
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if strings.HasPrefix(trim, "- ") && currentList == "allowed_tools" {
			s.AllowedTools = append(s.AllowedTools, cleanValue(strings.TrimPrefix(trim, "- ")))
			continue
		}
		k, v, ok := splitKV(trim)
		if !ok {
			return s, fmt.Errorf("invalid front matter line: %s", trim)
		}
		currentList = ""
		if v == "" && k == "allowed_tools" {
			currentList = "allowed_tools"
			continue
		}
		switch k {
		case "name":
			s.Name = v
		case "version":
			s.Version = v
		case "status":
			s.Status = v
		case "risk":
			s.Risk = v
		case "requires_consent":
			s.RequiresConsent = strings.EqualFold(v, "true")
		}
	}
	return s, nil
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
