package providers

import "context"

type BalanceInfo struct {
	Provider    string            `json:"provider"`
	Currency    string            `json:"currency"`
	Total       float64           `json:"total"`
	Used        float64           `json:"used"`
	Remaining   float64           `json:"remaining"`
	IsAvailable bool              `json:"is_available"`
	Extra       map[string]string `json:"extra"`
}

type PackageInstance struct {
	ProductCode     string `json:"product_code"`
	PackageType     string `json:"package_type"`
	TotalAmount     string `json:"total_amount"`
	TotalUnit       string `json:"total_unit"`
	RemainingAmount string `json:"remaining_amount"`
	RemainingUnit   string `json:"remaining_unit"`
	Remark          string `json:"remark"`
	ExpiryTime      string `json:"expiry_time"`
}

type Provider interface {
	Name() string
	GetBalance(ctx context.Context, apiKey string) (*BalanceInfo, error)
}
