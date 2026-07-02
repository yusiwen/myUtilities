package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL         string
	APIKey          string
	Model           string
	HTTPClient      *http.Client
	DebugWriter     io.Writer
	DisableThinking bool
}

type ChatResult struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type chatRequest struct {
	Model    string             `json:"model"`
	Messages []chatMessage      `json:"messages"`
	Thinking *map[string]string `json:"thinking,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

func NewClient(baseURL, apiKey, model string) *Client {
	c := &Client{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
	if strings.Contains(strings.ToLower(baseURL), "deepseek") {
		c.DisableThinking = true
	}
	return c
}

func (c *Client) ChatCompletion(systemPrompt, userPrompt string) (*ChatResult, error) {
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	if c.DisableThinking {
		reqBody.Thinking = &map[string]string{"type": "disabled"}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.DebugWriter != nil {
		pretty, _ := json.MarshalIndent(reqBody, "", "  ")
		fmt.Fprintln(c.DebugWriter, "─── Request JSON ───")
		fmt.Fprintln(c.DebugWriter, string(pretty))
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if c.DebugWriter != nil {
		fmt.Fprintln(c.DebugWriter, "─── Response JSON ───")
		fmt.Fprintln(c.DebugWriter, string(respBytes))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from API")
	}

	msg := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	msg = strings.Trim(msg, "\"'`")

	result := &ChatResult{Content: msg}
	if chatResp.Usage != nil {
		result.PromptTokens = chatResp.Usage.PromptTokens
		result.CompletionTokens = chatResp.Usage.CompletionTokens
		result.TotalTokens = chatResp.Usage.TotalTokens
	}

	return result, nil
}
