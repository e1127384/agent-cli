# Automated LLM-as-a-Judge Summary Quality Evaluation Framework

A non-interactive CLI framework that evaluates AI-generated whistleblowing case summaries using a **locally hosted LLM** as an automated auditor.

## What it does

For each Case ID from an input text file, the CLI:

1. Authenticates against the configured API
2. Fetches:
   - raw case JSON
   - generated summary text
3. Sends both to a local LLM judge
4. Scores three dimensions (1-5):
   - Faithfulness
   - Completeness
   - Anonymity/Safety
5. Applies configured quality thresholds
6. Writes run artifacts to a timestamped run directory

## Configuration

All runtime settings are in `config.yaml` (no hardcoded URLs/credentials/thresholds):

- auth endpoint + credentials
- API base URL + endpoint routes
- configurable API request methods/payload templates (supports GraphQL payloads with `{case_id}` substitution)
- timeout/retry policy
- input case-id file
- output base directory
- local LLM endpoint/model/temperature
- quality gate thresholds

Use environment variables for secrets (supported via `${VAR_NAME}` expansion in `config.yaml`), for example:

```yaml
auth:
  grant_type: "password"
  client_id: "${API_CLIENT_ID}"
  client_secret: "${API_CLIENT_SECRET}" # optional, if required by your auth server
  username: "${API_USERNAME}"
  password: "${API_PASSWORD}"

api:
  base_url: "http://localhost:9000"
  case_detail_endpoint: "/graphql"
  case_summary_endpoint: "/graphql"
  case_detail_method: "POST"
  case_summary_method: "POST"
  case_detail_payload: '{"query":"query GetCase($caseId: String!) { case(id: $caseId) { id title details } }","variables":{"caseId":"{case_id}"}}'
  case_summary_payload: '{"query":"query GetCaseSummary($caseId: String!) { case(id: $caseId) { summary } }","variables":{"caseId":"{case_id}"}}'
  case_detail_response_path: "data.case"
  case_summary_response_path: "data.case.summary"
```

## Case ID input format

`input.case_ids_file` points to a plain text file with one Case ID per line.

- blank lines are ignored
- comments prefixed with `#` are ignored
- inline comments are supported (`CASE-1 # note`)

## Run

```bash
go mod tidy
go run ./cmd/agent --config ./config.yaml
# or
# go run ./cmd/agent --config ./config.yaml evaluate
```

## Output artifacts

Each run creates:

- `runs/run_YYYYMMDD_HHMMSS/`
  - `<CASE_ID>/actual_case.json`
  - `<CASE_ID>/summary.txt`
  - `<CASE_ID>/assessment.json`
  - `run_summary.json`

`run_summary.json` includes per-case status: `PASSED`, `FAILED`, `TIMEOUT_ERROR`, or `ERROR`.
