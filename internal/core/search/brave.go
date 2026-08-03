package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const braveAPIBase = "https://api.search.brave.com/res/v1"

type braveResponse struct {
	Query     string `json:"query"`
	Grounding *struct {
		Generic []braveResult `json:"generic"`
	} `json:"grounding"`
}

type braveResult struct {
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Snippets []string `json:"snippets"`
}

type BraveSearch struct {
	APIKey string
	Client *http.Client
}

func NewBraveSearch(apiKey string) *BraveSearch {
	return &BraveSearch{
		APIKey: apiKey,
		Client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (b *BraveSearch) Search(ctx context.Context, query string, count int) ([]Result, error) {
	if count < 1 || count > 10 {
		count = 5
	}

	u, _ := url.Parse(braveAPIBase + "/llm/context")
	u.RawQuery = url.Values{
		"q":     {query},
		"count": {fmt.Sprintf("%d", count)},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Subscription-Token", b.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Brave API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var braveResp braveResponse
	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if braveResp.Grounding == nil || len(braveResp.Grounding.Generic) == 0 {
		return nil, nil
	}

	results := make([]Result, 0, len(braveResp.Grounding.Generic))
	for _, r := range braveResp.Grounding.Generic {
		snippet := ""
		if len(r.Snippets) > 0 {
			snippet = r.Snippets[0]
		}
		results = append(results, Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
		})
	}

	return results, nil
}
