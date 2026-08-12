package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsAuthGrantTypeToPassword(t *testing.T) {
	cfgPath := writeTestConfigFile(t, `
auth:
  endpoint: "http://localhost:9000/auth/token"
  client_id: "my-client"
  username: "user"
  password: "pass"
api:
  base_url: "http://localhost:9000/api"
  case_detail_endpoint: "/cases/{case_id}"
  case_summary_endpoint: "/cases/{case_id}/summary"
http:
  connect_timeout_seconds: 1
  read_timeout_seconds: 1
input:
  case_ids_file: "./case_ids.txt"
llm_judge:
  base_url: "http://localhost:11434"
  model: "local-model"
quality_thresholds:
  faithfulness: 4
  completeness: 4
  anonymity: 4
`)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if cfg.Auth.GrantType != "password" {
		t.Fatalf("expected default grant_type=password, got %q", cfg.Auth.GrantType)
	}
}

func TestLoadValidatesRequiredPasswordGrantFields(t *testing.T) {
	cfgPath := writeTestConfigFile(t, `
auth:
  endpoint: "http://localhost:9000/auth/token"
  grant_type: "password"
api:
  base_url: "http://localhost:9000/api"
  case_detail_endpoint: "/cases/{case_id}"
  case_summary_endpoint: "/cases/{case_id}/summary"
http:
  connect_timeout_seconds: 1
  read_timeout_seconds: 1
input:
  case_ids_file: "./case_ids.txt"
llm_judge:
  base_url: "http://localhost:11434"
  model: "local-model"
quality_thresholds:
  faithfulness: 4
  completeness: 4
  anonymity: 4
`)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	for _, field := range []string{"auth.client_id", "auth.username", "auth.password"} {
		if !strings.Contains(msg, field) {
			t.Fatalf("expected error to mention %s, got: %s", field, msg)
		}
	}
}

func writeTestConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
