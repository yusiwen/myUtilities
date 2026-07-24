package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type creditsData struct {
	TotalCredits float64 `json:"total_credits"`
	TotalUsage   float64 `json:"total_usage"`
}

type creditsResponse struct {
	Data creditsData `json:"data"`
}

type keyData struct {
	Usage    float64  `json:"usage"`
	Limit    *float64 `json:"limit"`
	Disabled bool     `json:"disabled"`
}

type keyResponse struct {
	Data keyData `json:"data"`
}

type openrouterProvider struct {
	client  *http.Client
	baseURL string
}

func NewOpenRouter() Provider {
	return &openrouterProvider{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://openrouter.ai",
	}
}

func newOpenRouterWithURL(baseURL string) *openrouterProvider {
	return &openrouterProvider{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
	}
}

func (p *openrouterProvider) Name() string {
	return "openrouter"
}

func (p *openrouterProvider) GetBalance(ctx context.Context, apiKey string) (*BalanceInfo, error) {
	info, err := p.tryCredits(ctx, apiKey)
	if err == nil {
		return info, nil
	}
	return p.tryKeyInfo(ctx, apiKey)
}

func (p *openrouterProvider) tryCredits(ctx context.Context, apiKey string) (*BalanceInfo, error) {
	url := p.baseURL + "/api/v1/credits"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("openrouter: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("openrouter: credits endpoint requires a management key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter: credits endpoint returned status %d", resp.StatusCode)
	}

	var body creditsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("openrouter: failed to parse credits response: %w", err)
	}

	return &BalanceInfo{
		Provider:  "openrouter",
		Currency:  "USD",
		Total:     body.Data.TotalCredits,
		Used:      body.Data.TotalUsage,
		Remaining: body.Data.TotalCredits - body.Data.TotalUsage,
		Extra:     make(map[string]string),
	}, nil
}

func (p *openrouterProvider) tryKeyInfo(ctx context.Context, apiKey string) (*BalanceInfo, error) {
	url := p.baseURL + "/api/v1/auth/key"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("openrouter: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("openrouter: authentication failed — check your API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter: unexpected status %d", resp.StatusCode)
	}

	var body keyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("openrouter: failed to parse key response: %w", err)
	}

	info := &BalanceInfo{
		Provider: "openrouter",
		Currency: "USD",
		Used:     body.Data.Usage,
		Extra:    make(map[string]string),
	}
	if body.Data.Limit != nil {
		info.Total = *body.Data.Limit
		info.Remaining = info.Total - info.Used
	} else {
		info.Remaining = -1
	}
	return info, nil
}
