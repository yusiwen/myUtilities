package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type aliyunBalanceResponse struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
	Data    struct {
		AvailableAmount     string `json:"AvailableAmount"`
		AvailableCashAmount string `json:"AvailableCashAmount"`
		CreditAmount        string `json:"CreditAmount"`
		MybankCreditAmount  string `json:"MybankCreditAmount"`
		Currency            string `json:"Currency"`
	} `json:"Data"`
}

type aliyunPackageInstance struct {
	InstanceID          string `json:"InstanceId"`
	ProductCode         string `json:"ProductCode"`
	PackageType         string `json:"PackageType"`
	TotalAmount         string `json:"TotalAmount"`
	TotalAmountUnit     string `json:"TotalAmountUnit"`
	RemainingAmount     string `json:"RemainingAmount"`
	RemainingAmountUnit string `json:"RemainingAmountUnit"`
	Remark              string `json:"Remark"`
	EffectiveTime       string `json:"EffectiveTime"`
	ExpiryTime          string `json:"ExpiryTime"`
	Status              string `json:"Status"`
}

type aliyunPackagesResponse struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
	Data    struct {
		PageNum    int `json:"PageNum"`
		PageSize   int `json:"PageSize"`
		TotalCount int `json:"TotalCount"`
		Instances  struct {
			Instance []aliyunPackageInstance `json:"Instance"`
		} `json:"Instances"`
	} `json:"Data"`
}

type aliyunProvider struct {
	client          *http.Client
	accessKeyID     string
	accessKeySecret string
	baseURL         string
}

func NewAliyun(accessKeyID, accessKeySecret string) Provider {
	return &aliyunProvider{
		client:          &http.Client{Timeout: 10 * time.Second},
		accessKeyID:     accessKeyID,
		accessKeySecret: accessKeySecret,
	}
}

func newAliyunWithURL(accessKeyID, accessKeySecret, baseURL string) *aliyunProvider {
	return &aliyunProvider{
		client:          &http.Client{Timeout: 10 * time.Second},
		accessKeyID:     accessKeyID,
		accessKeySecret: accessKeySecret,
		baseURL:         baseURL,
	}
}

func (p *aliyunProvider) Name() string {
	return "aliyun"
}

func (p *aliyunProvider) GetBalance(ctx context.Context, apiKey string) (*BalanceInfo, error) {
	_ = apiKey

	params := map[string]string{
		"Action":      "QueryAccountBalance",
		"AccessKeyId": p.accessKeyID,
	}
	signAliyun(params, p.accessKeySecret)
	url := buildAliyunURL(params)
	if p.baseURL != "" {
		url = strings.Replace(url, "https://business.aliyuncs.com/?", p.baseURL+"/?", 1)
	}

	body, err := p.doGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp aliyunBalanceResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("aliyun: failed to parse balance response: %w", err)
	}
	if resp.Code != "Success" && resp.Code != "200" {
		return nil, fmt.Errorf("aliyun: balance query failed: %s", resp.Message)
	}

	info := &BalanceInfo{
		Provider:    "aliyun",
		Currency:    resp.Data.Currency,
		Total:       parseFloat(resp.Data.AvailableAmount),
		Remaining:   parseFloat(resp.Data.AvailableAmount),
		IsAvailable: true,
		Extra:       make(map[string]string),
	}
	info.Extra["available_cash"] = resp.Data.AvailableCashAmount
	info.Extra["credit_amount"] = resp.Data.CreditAmount
	info.Extra["mybank_credit"] = resp.Data.MybankCreditAmount

	return info, nil
}

func (p *aliyunProvider) GetPackages(ctx context.Context) ([]PackageInstance, error) {
	params := map[string]string{
		"Action":      "QueryResourcePackageInstances",
		"AccessKeyId": p.accessKeyID,
		"PageSize":    "100",
	}
	signAliyun(params, p.accessKeySecret)
	url := buildAliyunURL(params)
	if p.baseURL != "" {
		url = strings.Replace(url, "https://business.aliyuncs.com/?", p.baseURL+"/?", 1)
	}

	body, err := p.doGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp aliyunPackagesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("aliyun: failed to parse packages response: %w", err)
	}
	if resp.Code != "Success" && resp.Code != "200" {
		return nil, fmt.Errorf("aliyun: packages query failed: %s", resp.Message)
	}

	var result []PackageInstance
	for _, inst := range resp.Data.Instances.Instance {
		result = append(result, PackageInstance{
			ProductCode:     inst.ProductCode,
			PackageType:     inst.PackageType,
			TotalAmount:     inst.TotalAmount,
			TotalUnit:       inst.TotalAmountUnit,
			RemainingAmount: inst.RemainingAmount,
			RemainingUnit:   inst.RemainingAmountUnit,
			Remark:          inst.Remark,
			ExpiryTime:      inst.ExpiryTime,
		})
	}
	return result, nil
}

func (p *aliyunProvider) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("aliyun: failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aliyun: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aliyun: unexpected status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
