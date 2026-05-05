package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	BaseURL     string
	Model       string
	Temperature float64
	HTTP        *http.Client
}

func New(baseURL, model string, temperature float64, timeoutSeconds int) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		Model:       model,
		Temperature: temperature,
		HTTP: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := chatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: c.Temperature,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm API status: %s", resp.Status)
	}

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm response had no choices")
	}
	return out.Choices[0].Message.Content, nil
}
