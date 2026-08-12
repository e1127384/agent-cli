package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-cli/internal/config"
)

const (
	StatusPassed       = "PASSED"
	StatusFailed       = "FAILED"
	StatusTimeoutError = "TIMEOUT_ERROR"
	StatusError        = "ERROR"
)

type Runner struct {
	cfg        *config.Config
	apiClient  *http.Client
	judgeClient *http.Client
}

type RunSummary struct {
	RunDirectory string            `json:"run_directory"`
	StartedAt    string            `json:"started_at"`
	CompletedAt  string            `json:"completed_at"`
	Cases        []CaseRunSummary  `json:"cases"`
	Totals       RunSummaryTotals  `json:"totals"`
}

type RunSummaryTotals struct {
	TotalCases    int `json:"total_cases"`
	Passed        int `json:"passed"`
	Failed        int `json:"failed"`
	TimeoutErrors int `json:"timeout_errors"`
	Errors        int `json:"errors"`
}

type CaseRunSummary struct {
	CaseID            string            `json:"case_id"`
	Status            string            `json:"status"`
	CaseDirectory     string            `json:"case_directory"`
	Assessment        *AssessmentResult `json:"assessment,omitempty"`
	Error             string            `json:"error,omitempty"`
}

type AssessmentResult struct {
	Faithfulness ScoreResult `json:"faithfulness"`
	Completeness ScoreResult `json:"completeness"`
	Anonymity    ScoreResult `json:"anonymity"`
}

type ScoreResult struct {
	Score     int    `json:"score"`
	Reasoning string `json:"reasoning"`
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	Token       string `json:"token"`
}

type chatRequest struct {
	Model       string      `json:"model"`
	Messages    []chatMsg   `json:"messages"`
	Temperature float64     `json:"temperature"`
	ResponseFmt responseFmt `json:"response_format"`
}

type responseFmt struct {
	Type string `json:"type"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMsg `json:"message"`
	} `json:"choices"`
}

func NewRunner(cfg *config.Config) *Runner {
	connectTimeout := time.Duration(cfg.HTTP.ConnectTimeoutSeconds) * time.Second
	apiTransport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: connectTimeout}).DialContext,
	}
	judgeTransport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: connectTimeout}).DialContext,
	}
	apiClient := &http.Client{
		Transport: apiTransport,
		Timeout:   time.Duration(cfg.HTTP.ReadTimeoutSeconds) * time.Second,
	}
	judgeClient := &http.Client{
		Transport: judgeTransport,
		Timeout:   time.Duration(cfg.LLMJudge.TimeoutSeconds) * time.Second,
	}
	return &Runner{cfg: cfg, apiClient: apiClient, judgeClient: judgeClient}
}

func (r *Runner) Run(ctx context.Context) (string, error) {
	caseIDs, err := ReadCaseIDsFile(r.cfg.Input.CaseIDsFile)
	if err != nil {
		return "", err
	}

	runDir, err := createRunDirectory(r.cfg.Output.BaseDir, time.Now())
	if err != nil {
		return "", fmt.Errorf("create run directory: %w", err)
	}

	token, err := r.authenticate(ctx)
	if err != nil {
		return "", err
	}

	summary := RunSummary{
		RunDirectory: runDir,
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		Cases:        make([]CaseRunSummary, 0, len(caseIDs)),
	}

	for _, caseID := range caseIDs {
		caseResult := r.processCase(ctx, runDir, token, caseID)
		summary.Cases = append(summary.Cases, caseResult)
	}

	summary.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	summary.Totals = summarizeTotals(summary.Cases)

	summaryPath := filepath.Join(runDir, "run_summary.json")
	if err := writeJSONFile(summaryPath, summary); err != nil {
		return "", fmt.Errorf("write run summary: %w", err)
	}

	return runDir, nil
}

func ReadCaseIDsFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read case id file: %w", err)
	}

	var ids []string
	for _, rawLine := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line != "" {
			ids = append(ids, line)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("case id file is empty after filtering comments and blank lines")
	}
	return ids, nil
}

func createRunDirectory(base string, now time.Time) (string, error) {
	runDir := filepath.Join(base, "run_"+now.Format("20060102_150405"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", err
	}
	return runDir, nil
}

func (r *Runner) authenticate(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("grant_type", r.cfg.Auth.GrantType)
	form.Set("client_id", r.cfg.Auth.ClientID)
	form.Set("username", r.cfg.Auth.Username)
	form.Set("password", r.cfg.Auth.Password)
	if r.cfg.Auth.ClientSecret != "" {
		form.Set("client_secret", r.cfg.Auth.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.Auth.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, body, err := r.doWithRetry(ctx, r.apiClient, req)
	if err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("authentication failed: status=%s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}

	token, err := parseToken(body)
	if err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("authentication failed: empty bearer token")
	}
	return token, nil
}

func parseToken(body []byte) (string, error) {
	var standard authResponse
	if err := json.Unmarshal(body, &standard); err == nil {
		if standard.AccessToken != "" {
			return standard.AccessToken, nil
		}
		if standard.Token != "" {
			return standard.Token, nil
		}
	}

	var generic map[string]any
	if err := json.Unmarshal(body, &generic); err != nil {
		return "", fmt.Errorf("invalid auth response JSON: %w", err)
	}
	for _, key := range []string{"access_token", "token", "bearer_token"} {
		if value, ok := generic[key].(string); ok && value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("token not found in auth response")
}

func (r *Runner) processCase(ctx context.Context, runDir, token, caseID string) CaseRunSummary {
	caseDir := filepath.Join(runDir, safeCaseDir(caseID))
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		return CaseRunSummary{CaseID: caseID, Status: StatusError, CaseDirectory: caseDir, Error: err.Error()}
	}

	result := CaseRunSummary{CaseID: caseID, CaseDirectory: caseDir}

	casePayload, err := r.fetchCasePayload(ctx, token, caseID)
	if err != nil {
		result.Status = classifyError(err)
		result.Error = err.Error()
		_ = writeErrorAssessment(caseDir, result.Error)
		return result
	}
	if err := writeRawJSON(caseDir, "actual_case.json", casePayload); err != nil {
		result.Status = StatusError
		result.Error = err.Error()
		_ = writeErrorAssessment(caseDir, result.Error)
		return result
	}

	summaryText, err := r.fetchSummaryText(ctx, token, caseID)
	if err != nil {
		result.Status = classifyError(err)
		result.Error = err.Error()
		_ = writeErrorAssessment(caseDir, result.Error)
		return result
	}
	if err := os.WriteFile(filepath.Join(caseDir, "summary.txt"), []byte(summaryText), 0o644); err != nil {
		result.Status = StatusError
		result.Error = err.Error()
		_ = writeErrorAssessment(caseDir, result.Error)
		return result
	}

	assessment, err := r.evaluateCase(ctx, casePayload, summaryText)
	if err != nil {
		result.Status = classifyError(err)
		result.Error = err.Error()
		_ = writeErrorAssessment(caseDir, result.Error)
		return result
	}

	if err := writeJSONFile(filepath.Join(caseDir, "assessment.json"), assessment); err != nil {
		result.Status = StatusError
		result.Error = err.Error()
		_ = writeErrorAssessment(caseDir, result.Error)
		return result
	}

	result.Assessment = &assessment
	if meetsThresholds(assessment, r.cfg.QualityThresholds) {
		result.Status = StatusPassed
	} else {
		result.Status = StatusFailed
	}
	return result
}

func safeCaseDir(caseID string) string {
	safe := strings.TrimSpace(caseID)
	safe = strings.ReplaceAll(safe, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	if safe == "" {
		return "unknown_case"
	}
	return safe
}

func (r *Runner) fetchCasePayload(ctx context.Context, token, caseID string) ([]byte, error) {
	req, err := buildAPIRequest(ctx, r.cfg.API.BaseURL, r.cfg.API.CaseDetailEndpoint, r.cfg.API.CaseDetailMethod, r.cfg.API.CaseDetailPayload, token, caseID)
	if err != nil {
		return nil, fmt.Errorf("build case request: %w", err)
	}

	resp, body, err := r.doWithRetry(ctx, r.apiClient, req)
	if err != nil {
		return nil, fmt.Errorf("fetch case %s: %w", caseID, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch case %s: status=%s", caseID, resp.Status)
	}

	if !json.Valid(body) {
		return nil, fmt.Errorf("fetch case %s: response is not valid JSON", caseID)
	}
	if r.cfg.API.CaseDetailResponsePath != "" {
		selected, err := extractJSONPath(body, r.cfg.API.CaseDetailResponsePath)
		if err != nil {
			return nil, fmt.Errorf("fetch case %s: %w", caseID, err)
		}
		out, err := json.Marshal(selected)
		if err != nil {
			return nil, fmt.Errorf("fetch case %s: marshal selected case payload: %w", caseID, err)
		}
		return out, nil
	}
	return body, nil
}

func (r *Runner) fetchSummaryText(ctx context.Context, token, caseID string) (string, error) {
	req, err := buildAPIRequest(ctx, r.cfg.API.BaseURL, r.cfg.API.CaseSummaryEndpoint, r.cfg.API.CaseSummaryMethod, r.cfg.API.CaseSummaryPayload, token, caseID)
	if err != nil {
		return "", fmt.Errorf("build summary request: %w", err)
	}

	resp, body, err := r.doWithRetry(ctx, r.apiClient, req)
	if err != nil {
		return "", fmt.Errorf("fetch summary %s: %w", caseID, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch summary %s: status=%s", caseID, resp.Status)
	}
	if r.cfg.API.CaseSummaryResponsePath != "" {
		selected, err := extractJSONPath(body, r.cfg.API.CaseSummaryResponsePath)
		if err != nil {
			return "", fmt.Errorf("fetch summary %s: %w", caseID, err)
		}
		s, ok := selected.(string)
		if !ok {
			return "", fmt.Errorf("fetch summary %s: selected response value at %q is not a string", caseID, r.cfg.API.CaseSummaryResponsePath)
		}
		return strings.TrimSpace(s), nil
	}

	return strings.TrimSpace(string(body)), nil
}

func buildURL(base, endpointTemplate, caseID string) string {
	replaced := strings.ReplaceAll(endpointTemplate, "{case_id}", caseID)
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(replaced, "/")
}

func buildAPIRequest(ctx context.Context, baseURL, endpoint, method, payloadTemplate, token, caseID string) (*http.Request, error) {
	httpMethod := strings.ToUpper(strings.TrimSpace(method))
	if httpMethod == "" {
		httpMethod = http.MethodGet
	}

	url := buildURL(baseURL, endpoint, caseID)
	switch httpMethod {
	case http.MethodGet:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	case http.MethodPost:
		payload, err := renderCasePayload(payloadTemplate, caseID)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	default:
		return nil, fmt.Errorf("unsupported API method %q", method)
	}
}

func renderCasePayload(payloadTemplate, caseID string) (string, error) {
	if strings.TrimSpace(payloadTemplate) == "" {
		return "", fmt.Errorf("request payload is empty")
	}
	if !strings.Contains(payloadTemplate, "{case_id}") {
		return "", fmt.Errorf("request payload must include {case_id} placeholder")
	}
	return strings.ReplaceAll(payloadTemplate, "{case_id}", caseID), nil
}

func extractJSONPath(raw []byte, path string) (any, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("response is not valid JSON: %w", err)
	}

	current := decoded
	for _, key := range strings.Split(path, ".") {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("response path %q not found", path)
		}
		next, ok := obj[key]
		if !ok {
			return nil, fmt.Errorf("response path %q not found", path)
		}
		current = next
	}
	return current, nil
}

func buildServiceURL(base, endpoint string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(endpoint, "/")
}

func (r *Runner) evaluateCase(ctx context.Context, casePayload []byte, summary string) (AssessmentResult, error) {
	prompt := fmt.Sprintf(`You are a strict whistleblowing summary quality auditor. Evaluate the generated summary against the source case data.

Return ONLY valid JSON with this exact schema:
{
  "faithfulness": {"score": 1-5, "reasoning": "..."},
  "completeness": {"score": 1-5, "reasoning": "..."},
  "anonymity": {"score": 1-5, "reasoning": "..."}
}

Scoring criteria:
- faithfulness: factual alignment with source case and zero hallucinations.
- completeness: coverage of allegations, evidence, roles, and timeline.
- anonymity: no unintended PII or non-disclosable identities.

Source case JSON:
%s

Generated summary:
%s`, string(casePayload), summary)

	body := chatRequest{
		Model:       r.cfg.LLMJudge.Model,
		Temperature: r.cfg.LLMJudge.Temperature,
		Messages: []chatMsg{
			{Role: "system", Content: "You are an objective compliance auditor."},
			{Role: "user", Content: prompt},
		},
		ResponseFmt: responseFmt{Type: "json_object"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("marshal judge request: %w", err)
	}

	url := buildServiceURL(r.cfg.LLMJudge.BaseURL, r.cfg.LLMJudge.EndpointPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("build judge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, raw, err := r.doWithRetry(ctx, r.judgeClient, req)
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("judge request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AssessmentResult{}, fmt.Errorf("judge request failed: status=%s", resp.Status)
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return AssessmentResult{}, fmt.Errorf("parse judge API response: %w", err)
	}
	if len(out.Choices) == 0 {
		return AssessmentResult{}, fmt.Errorf("judge API response had no choices")
	}

	content := cleanJSONBlock(out.Choices[0].Message.Content)
	var assessment AssessmentResult
	if err := json.Unmarshal([]byte(content), &assessment); err != nil {
		return AssessmentResult{}, fmt.Errorf("parse judge JSON content: %w", err)
	}

	if err := validateAssessment(assessment); err != nil {
		return AssessmentResult{}, err
	}
	return assessment, nil
}

func cleanJSONBlock(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func validateAssessment(a AssessmentResult) error {
	for _, dim := range []struct {
		name  string
		score int
	}{
		{name: "faithfulness", score: a.Faithfulness.Score},
		{name: "completeness", score: a.Completeness.Score},
		{name: "anonymity", score: a.Anonymity.Score},
	} {
		if dim.score < 1 || dim.score > 5 {
			return fmt.Errorf("judge score for %s must be 1-5", dim.name)
		}
	}
	return nil
}

func meetsThresholds(a AssessmentResult, t config.QualityThresholdsConfig) bool {
	return a.Faithfulness.Score >= t.Faithfulness &&
		a.Completeness.Score >= t.Completeness &&
		a.Anonymity.Score >= t.Anonymity
}

func summarizeTotals(cases []CaseRunSummary) RunSummaryTotals {
	t := RunSummaryTotals{TotalCases: len(cases)}
	for _, c := range cases {
		switch c.Status {
		case StatusPassed:
			t.Passed++
		case StatusFailed:
			t.Failed++
		case StatusTimeoutError:
			t.TimeoutErrors++
		default:
			t.Errors++
		}
	}
	return t
}

func (r *Runner) doWithRetry(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, []byte, error) {
	attempts := r.cfg.HTTP.RetryMaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 1 && req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return nil, nil, fmt.Errorf("request body is not replayable for retries")
	}

	initial := time.Duration(r.cfg.HTTP.RetryInitialBackoffMS) * time.Millisecond
	if initial <= 0 {
		initial = 100 * time.Millisecond
	}
	maxBackoff := time.Duration(r.cfg.HTTP.RetryMaxBackoffMS) * time.Millisecond
	if maxBackoff <= 0 {
		maxBackoff = 5 * time.Second
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		clone := req.Clone(ctx)
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, nil, err
			}
			clone.Body = body
		}

		resp, err := client.Do(clone)
		if err != nil {
			if attempt == attempts || !isRetryableNetworkError(err) {
				return nil, nil, err
			}
			time.Sleep(backoffDuration(initial, maxBackoff, attempt))
			continue
		}

		originalBody := resp.Body
		body, err := io.ReadAll(originalBody)
		closeErr := originalBody.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			if attempt == attempts {
				return nil, nil, err
			}
			time.Sleep(backoffDuration(initial, maxBackoff, attempt))
			continue
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))

		if resp.StatusCode >= 500 && attempt < attempts {
			resp.Body.Close()
			time.Sleep(backoffDuration(initial, maxBackoff, attempt))
			continue
		}
		return resp, body, nil
	}

	return nil, nil, fmt.Errorf("request failed after retries")
}

func isRetryableNetworkError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func backoffDuration(initial, max time.Duration, attempt int) time.Duration {
	if attempt <= 1 {
		return initial
	}
	power := math.Pow(2, float64(attempt-1))
	d := time.Duration(float64(initial) * power)
	if d > max {
		return max
	}
	return d
}

func classifyError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return StatusTimeoutError
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return StatusTimeoutError
	}
	if strings.Contains(strings.ToLower(err.Error()), "timeout") {
		return StatusTimeoutError
	}
	return StatusError
}

func writeRawJSON(caseDir, filename string, raw []byte) error {
	var indented bytes.Buffer
	if err := json.Indent(&indented, raw, "", "  "); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(caseDir, filename), indented.Bytes(), 0o644)
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeErrorAssessment(caseDir, msg string) error {
	content := map[string]string{"error": msg}
	return writeJSONFile(filepath.Join(caseDir, "assessment.json"), content)
}
