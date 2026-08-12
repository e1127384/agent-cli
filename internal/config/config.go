package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Auth              AuthConfig              `yaml:"auth"`
	API               APIConfig               `yaml:"api"`
	HTTP              HTTPConfig              `yaml:"http"`
	Input             InputConfig             `yaml:"input"`
	Output            OutputConfig            `yaml:"output"`
	LLMJudge          LLMJudgeConfig          `yaml:"llm_judge"`
	QualityThresholds QualityThresholdsConfig `yaml:"quality_thresholds"`
}

type AuthConfig struct {
	Endpoint     string `yaml:"endpoint"`
	GrantType    string `yaml:"grant_type"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
}

type APIConfig struct {
	BaseURL             string `yaml:"base_url"`
	CaseDetailEndpoint  string `yaml:"case_detail_endpoint"`
	CaseSummaryEndpoint string `yaml:"case_summary_endpoint"`
}

type HTTPConfig struct {
	ConnectTimeoutSeconds int `yaml:"connect_timeout_seconds"`
	ReadTimeoutSeconds    int `yaml:"read_timeout_seconds"`
	RetryMaxAttempts      int `yaml:"retry_max_attempts"`
	RetryInitialBackoffMS int `yaml:"retry_initial_backoff_ms"`
	RetryMaxBackoffMS     int `yaml:"retry_max_backoff_ms"`
}

type InputConfig struct {
	CaseIDsFile string `yaml:"case_ids_file"`
}

type OutputConfig struct {
	BaseDir string `yaml:"base_dir"`
}

type LLMJudgeConfig struct {
	BaseURL        string  `yaml:"base_url"`
	EndpointPath   string  `yaml:"endpoint_path"`
	Model          string  `yaml:"model"`
	Temperature    float64 `yaml:"temperature"`
	TimeoutSeconds int     `yaml:"timeout_seconds"`
}

type QualityThresholdsConfig struct {
	Faithfulness int `yaml:"faithfulness"`
	Completeness int `yaml:"completeness"`
	Anonymity    int `yaml:"anonymity"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	expanded := os.ExpandEnv(string(b))

	cfg := Config{}
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}
	applyDefaults(&cfg)
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Auth.GrantType == "" {
		cfg.Auth.GrantType = "password"
	}

	if cfg.HTTP.ConnectTimeoutSeconds <= 0 {
		cfg.HTTP.ConnectTimeoutSeconds = 10
	}
	if cfg.HTTP.ReadTimeoutSeconds <= 0 {
		cfg.HTTP.ReadTimeoutSeconds = 30
	}
	if cfg.HTTP.RetryMaxAttempts <= 0 {
		cfg.HTTP.RetryMaxAttempts = 3
	}
	if cfg.HTTP.RetryInitialBackoffMS <= 0 {
		cfg.HTTP.RetryInitialBackoffMS = 500
	}
	if cfg.HTTP.RetryMaxBackoffMS <= 0 {
		cfg.HTTP.RetryMaxBackoffMS = 5000
	}

	if cfg.Output.BaseDir == "" {
		cfg.Output.BaseDir = "./runs"
	}

	if cfg.LLMJudge.EndpointPath == "" {
		cfg.LLMJudge.EndpointPath = "/v1/chat/completions"
	}
	if cfg.LLMJudge.TimeoutSeconds <= 0 {
		cfg.LLMJudge.TimeoutSeconds = cfg.HTTP.ReadTimeoutSeconds
	}
}

func validate(cfg *Config) error {
	if cfg.Auth.Endpoint == "" {
		return fmt.Errorf("auth.endpoint is required")
	}
	if cfg.Auth.GrantType == "" {
		return fmt.Errorf("auth.grant_type is required")
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Auth.GrantType), "password") {
		missing := make([]string, 0, 3)
		if cfg.Auth.ClientID == "" {
			missing = append(missing, "auth.client_id")
		}
		if cfg.Auth.Username == "" {
			missing = append(missing, "auth.username")
		}
		if cfg.Auth.Password == "" {
			missing = append(missing, "auth.password")
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s are required when auth.grant_type is password", strings.Join(missing, ", "))
		}
	}
	if cfg.API.BaseURL == "" {
		return fmt.Errorf("api.base_url is required")
	}
	if cfg.API.CaseDetailEndpoint == "" || cfg.API.CaseSummaryEndpoint == "" {
		return fmt.Errorf("api.case_detail_endpoint and api.case_summary_endpoint are required")
	}
	if cfg.Input.CaseIDsFile == "" {
		return fmt.Errorf("input.case_ids_file is required")
	}
	if cfg.LLMJudge.BaseURL == "" {
		return fmt.Errorf("llm_judge.base_url is required")
	}
	if cfg.LLMJudge.Model == "" {
		return fmt.Errorf("llm_judge.model is required")
	}
	if err := validateThreshold("quality_thresholds.faithfulness", cfg.QualityThresholds.Faithfulness); err != nil {
		return err
	}
	if err := validateThreshold("quality_thresholds.completeness", cfg.QualityThresholds.Completeness); err != nil {
		return err
	}
	if err := validateThreshold("quality_thresholds.anonymity", cfg.QualityThresholds.Anonymity); err != nil {
		return err
	}

	return nil
}

func validateThreshold(name string, v int) error {
	if v < 1 || v > 5 {
		return fmt.Errorf("%s must be between 1 and 5", name)
	}
	return nil
}
