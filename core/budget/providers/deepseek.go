package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type deepseekBalanceResponse struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

type deepseekProvider struct {
	client  *http.Client
	baseURL string
}

func NewDeepSeek() Provider {
	return &deepseekProvider{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://api.deepseek.com",
	}
}

func newDeepSeekWithURL(baseURL string) *deepseekProvider {
	return &deepseekProvider{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
	}
}

func (p *deepseekProvider) Name() string {
	return "deepseek"
}

func (p *deepseekProvider) GetBalance(ctx context.Context, apiKey string) (*BalanceInfo, error) {
	url := p.baseURL + "/user/balance"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("deepseek: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("deepseek: authentication failed — check your API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek: unexpected status %d", resp.StatusCode)
	}

	var body deepseekBalanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("deepseek: failed to parse response: %w", err)
	}

	info := &BalanceInfo{
		Provider:    "deepseek",
		IsAvailable: body.IsAvailable,
		Extra:       make(map[string]string),
	}

	if len(body.BalanceInfos) > 0 {
		b := body.BalanceInfos[0]
		info.Currency = b.Currency
		info.Total = parseFloat(b.TotalBalance)
		info.Remaining = info.Total
		info.Extra["granted_balance"] = b.GrantedBalance
		info.Extra["topped_up_balance"] = b.ToppedUpBalance
	}

	return info, nil
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
