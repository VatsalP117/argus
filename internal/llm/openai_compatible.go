package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type JSONRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
}

type ResponseMetadata struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
	ResponseFormat responseFormat `json:"response_format"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func NewClient(baseURL, apiKey string) Client {
	return Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c Client) CompleteJSON(ctx context.Context, req JSONRequest, out any) (ResponseMetadata, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return ResponseMetadata{}, fmt.Errorf("llm base url is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return ResponseMetadata{}, fmt.Errorf("llm api key is required")
	}
	if strings.TrimSpace(req.Model) == "" {
		return ResponseMetadata{}, fmt.Errorf("llm model is required")
	}
	if len(req.Messages) == 0 {
		return ResponseMetadata{}, fmt.Errorf("at least one message is required")
	}

	body := chatCompletionRequest{
		Model:          req.Model,
		Messages:       req.Messages,
		MaxTokens:      req.MaxTokens,
		Temperature:    req.Temperature,
		ResponseFormat: responseFormat{Type: "json_object"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ResponseMetadata{}, err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return ResponseMetadata{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return ResponseMetadata{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ResponseMetadata{}, err
	}

	if resp.StatusCode >= 400 {
		return ResponseMetadata{}, fmt.Errorf("llm request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return ResponseMetadata{}, fmt.Errorf("failed to decode llm response: %w: %s", err, string(responseBody))
	}
	if parsed.Error != nil {
		return ResponseMetadata{}, fmt.Errorf("llm error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return ResponseMetadata{}, fmt.Errorf("llm response returned no choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return ResponseMetadata{}, fmt.Errorf("llm response returned empty content")
	}
	if err := json.Unmarshal([]byte(content), out); err != nil {
		return ResponseMetadata{}, fmt.Errorf("failed to decode llm json content: %w: %s", err, content)
	}

	return ResponseMetadata{
		ID:      parsed.ID,
		Model:   parsed.Model,
		Content: content,
	}, nil
}
