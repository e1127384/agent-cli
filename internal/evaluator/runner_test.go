package evaluator

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-cli/internal/config"
)

func TestReadCaseIDsFileFiltersCommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "case_ids.txt")
	content := "\n# full comment\nCASE-001\nCASE-002 # inline comment\n\n  CASE-003  \n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ids, err := ReadCaseIDsFile(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ids) != 3 {
		t.Fatalf("expected 3 case IDs, got %d", len(ids))
	}
	if ids[0] != "CASE-001" || ids[1] != "CASE-002" || ids[2] != "CASE-003" {
		t.Fatalf("unexpected parsed IDs: %#v", ids)
	}
}

func TestReadCaseIDsFileRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "case_ids.txt")
	if err := os.WriteFile(file, []byte("# only comment\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadCaseIDsFile(file); err == nil {
		t.Fatal("expected error for empty file after filtering")
	}
}

func TestDoWithRetryRetriesOn5xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"temporary"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cfg := &config.Config{}
	cfg.HTTP.ConnectTimeoutSeconds = 1
	cfg.HTTP.ReadTimeoutSeconds = 2
	cfg.HTTP.RetryMaxAttempts = 3
	cfg.HTTP.RetryInitialBackoffMS = 1
	cfg.HTTP.RetryMaxBackoffMS = 5
	cfg.LLMJudge.TimeoutSeconds = 2

	runner := NewRunner(cfg)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, body, err := runner.doWithRetry(context.Background(), runner.apiClient, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestMeetsThresholds(t *testing.T) {
	assessment := AssessmentResult{
		Faithfulness: ScoreResult{Score: 4},
		Completeness: ScoreResult{Score: 5},
		Anonymity:    ScoreResult{Score: 5},
	}
	thresholds := config.QualityThresholdsConfig{Faithfulness: 4, Completeness: 4, Anonymity: 5}
	if !meetsThresholds(assessment, thresholds) {
		t.Fatal("expected assessment to pass thresholds")
	}

	thresholds = config.QualityThresholdsConfig{Faithfulness: 5, Completeness: 4, Anonymity: 5}
	if meetsThresholds(assessment, thresholds) {
		t.Fatal("expected assessment to fail thresholds")
	}
}

func TestRunContinuesAfterSingleCaseFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/cases/CASE1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"CASE1","detail":"x"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/cases/CASE1/summary":
			_, _ = w.Write([]byte("summary text"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/cases/CASE2":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/judge/chat":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"faithfulness\":{\"score\":5,\"reasoning\":\"ok\"},\"completeness\":{\"score\":5,\"reasoning\":\"ok\"},\"anonymity\":{\"score\":5,\"reasoning\":\"ok\"}}"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	caseIDsFile := filepath.Join(dir, "case_ids.txt")
	if err := os.WriteFile(caseIDsFile, []byte("CASE1\nCASE2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Auth: config.AuthConfig{
			Endpoint:  server.URL + "/auth",
			GrantType: "password",
			ClientID:  "cid",
			Username:  "u",
			Password:  "p",
		},
		API: config.APIConfig{
			BaseURL:             server.URL + "/api",
			CaseDetailEndpoint:  "/cases/{case_id}",
			CaseSummaryEndpoint: "/cases/{case_id}/summary",
		},
		HTTP: config.HTTPConfig{
			ConnectTimeoutSeconds: 1,
			ReadTimeoutSeconds:    2,
			RetryMaxAttempts:      2,
			RetryInitialBackoffMS: 1,
			RetryMaxBackoffMS:     5,
		},
		Input: config.InputConfig{CaseIDsFile: caseIDsFile},
		Output: config.OutputConfig{BaseDir: filepath.Join(dir, "runs")},
		LLMJudge: config.LLMJudgeConfig{
			BaseURL:        server.URL + "/judge",
			EndpointPath:   "/chat",
			Model:          "local-model",
			Temperature:    0,
			TimeoutSeconds: 2,
		},
		QualityThresholds: config.QualityThresholdsConfig{Faithfulness: 4, Completeness: 4, Anonymity: 4},
	}

	runner := NewRunner(cfg)
	runDir, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}

	rawSummary, err := os.ReadFile(filepath.Join(runDir, "run_summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary RunSummary
	if err := json.Unmarshal(rawSummary, &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Cases) != 2 {
		t.Fatalf("expected 2 cases in summary, got %d", len(summary.Cases))
	}

	statuses := map[string]string{}
	for _, c := range summary.Cases {
		statuses[c.CaseID] = c.Status
	}
	if statuses["CASE1"] != StatusPassed {
		t.Fatalf("CASE1 expected status %s, got %s", StatusPassed, statuses["CASE1"])
	}
	if statuses["CASE2"] != StatusError {
		t.Fatalf("CASE2 expected status %s, got %s", StatusError, statuses["CASE2"])
	}

	if _, err := os.Stat(filepath.Join(runDir, "CASE1", "actual_case.json")); err != nil {
		t.Fatalf("missing CASE1 actual_case.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "CASE1", "summary.txt")); err != nil {
		t.Fatalf("missing CASE1 summary.txt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "CASE1", "assessment.json")); err != nil {
		t.Fatalf("missing CASE1 assessment.json: %v", err)
	}
}

func TestAuthenticateUsesFormEncodedPayload(t *testing.T) {
	var gotContentType string
	var gotForm url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		Auth: config.AuthConfig{
			Endpoint:     server.URL,
			GrantType:    "custom_password_grant",
			ClientID:     "my-client",
			ClientSecret: "my-secret",
			Username:     "alice",
			Password:     "s3cr3t",
		},
		HTTP: config.HTTPConfig{
			ConnectTimeoutSeconds: 1,
			ReadTimeoutSeconds:    2,
			RetryMaxAttempts:      1,
		},
		LLMJudge: config.LLMJudgeConfig{
			TimeoutSeconds: 2,
		},
	}

	runner := NewRunner(cfg)
	token, err := runner.authenticate(context.Background())
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if token != "tok" {
		t.Fatalf("expected token tok, got %q", token)
	}
	if !strings.HasPrefix(gotContentType, "application/x-www-form-urlencoded") {
		t.Fatalf("expected form content type, got %q", gotContentType)
	}
	if gotForm.Get("grant_type") != "custom_password_grant" {
		t.Fatalf("expected grant_type in form, got %q", gotForm.Get("grant_type"))
	}
	if gotForm.Get("client_id") != "my-client" {
		t.Fatalf("expected client_id in form, got %q", gotForm.Get("client_id"))
	}
	if gotForm.Get("username") != "alice" {
		t.Fatalf("expected username in form, got %q", gotForm.Get("username"))
	}
	if gotForm.Get("password") != "s3cr3t" {
		t.Fatalf("expected password in form, got %q", gotForm.Get("password"))
	}
	if gotForm.Get("client_secret") != "my-secret" {
		t.Fatalf("expected client_secret in form, got %q", gotForm.Get("client_secret"))
	}
}

func TestFetchersSupportConfigurableGraphQLPayloads(t *testing.T) {
	caseID := "CASE-42"
	var detailMethod, summaryMethod, detailContentType, summaryContentType string
	var detailBody, summaryBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graphql/detail":
			detailMethod = r.Method
			detailContentType = r.Header.Get("Content-Type")
			raw, _ := io.ReadAll(r.Body)
			detailBody = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"case":{"id":"CASE-42","detail":"x"}}}`))
		case "/graphql/summary":
			summaryMethod = r.Method
			summaryContentType = r.Header.Get("Content-Type")
			raw, _ := io.ReadAll(r.Body)
			summaryBody = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"case":{"summary":"graph summary"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	runner := NewRunner(&config.Config{
		API: config.APIConfig{
			BaseURL:                 server.URL,
			CaseDetailEndpoint:      "/graphql/detail",
			CaseSummaryEndpoint:     "/graphql/summary",
			CaseDetailMethod:        "POST",
			CaseSummaryMethod:       "POST",
			CaseDetailPayload:       `{"query":"query($id:String!){case(id:$id){id detail}}","variables":{"id":"{case_id}"}}`,
			CaseSummaryPayload:      `{"query":"query($id:String!){case(id:$id){summary}}","variables":{"id":"{case_id}"}}`,
			CaseDetailResponsePath:  "data.case",
			CaseSummaryResponsePath: "data.case.summary",
		},
		HTTP: config.HTTPConfig{
			ConnectTimeoutSeconds: 1,
			ReadTimeoutSeconds:    2,
			RetryMaxAttempts:      1,
		},
		LLMJudge: config.LLMJudgeConfig{
			TimeoutSeconds: 2,
		},
	})

	casePayload, err := runner.fetchCasePayload(context.Background(), "tok", caseID)
	if err != nil {
		t.Fatalf("fetchCasePayload failed: %v", err)
	}
	if !json.Valid(casePayload) {
		t.Fatalf("expected JSON payload, got %s", string(casePayload))
	}
	if detailMethod != http.MethodPost {
		t.Fatalf("expected detail POST, got %s", detailMethod)
	}
	if !strings.HasPrefix(detailContentType, "application/json") {
		t.Fatalf("expected detail content-type application/json, got %q", detailContentType)
	}
	if !strings.Contains(detailBody, `"variables":{"id":"CASE-42"}`) {
		t.Fatalf("expected case_id substitution in detail payload, got %s", detailBody)
	}

	summaryText, err := runner.fetchSummaryText(context.Background(), "tok", caseID)
	if err != nil {
		t.Fatalf("fetchSummaryText failed: %v", err)
	}
	if summaryText != "graph summary" {
		t.Fatalf("expected summary text 'graph summary', got %q", summaryText)
	}
	if summaryMethod != http.MethodPost {
		t.Fatalf("expected summary POST, got %s", summaryMethod)
	}
	if !strings.HasPrefix(summaryContentType, "application/json") {
		t.Fatalf("expected summary content-type application/json, got %q", summaryContentType)
	}
	if !strings.Contains(summaryBody, `"variables":{"id":"CASE-42"}`) {
		t.Fatalf("expected case_id substitution in summary payload, got %s", summaryBody)
	}
}

func TestExtractJSONPathRejectsEmptySegments(t *testing.T) {
	_, err := extractJSONPath([]byte(`{"data":{"case":{"id":"CASE-1"}}}`), "data..case")
	if err == nil {
		t.Fatal("expected error for invalid response path")
	}
	if !strings.Contains(err.Error(), "contains empty segment") {
		t.Fatalf("unexpected error: %v", err)
	}
}
