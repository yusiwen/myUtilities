package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAliyunGetBalanceSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("Action") != "QueryAccountBalance" {
			t.Errorf("expected QueryAccountBalance action")
		}
		if r.URL.Query().Get("Signature") == "" {
			t.Error("expected Signature parameter")
		}
		json.NewEncoder(w).Encode(aliyunBalanceResponse{
			Code: "Success",
			Data: struct {
				AvailableAmount     string `json:"AvailableAmount"`
				AvailableCashAmount string `json:"AvailableCashAmount"`
				CreditAmount        string `json:"CreditAmount"`
				MybankCreditAmount  string `json:"MybankCreditAmount"`
				Currency            string `json:"Currency"`
			}{
				AvailableAmount:     "100.50",
				AvailableCashAmount: "50.00",
				CreditAmount:        "0.00",
				MybankCreditAmount:  "0.00",
				Currency:            "CNY",
			},
		})
	}))
	defer ts.Close()

	p := newAliyunWithURL("test-id", "test-secret", ts.URL)
	info, err := p.GetBalance(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Total != 100.50 {
		t.Errorf("expected total 100.50, got %f", info.Total)
	}
	if info.Currency != "CNY" {
		t.Errorf("expected CNY, got %s", info.Currency)
	}
	if info.Extra["available_cash"] != "50.00" {
		t.Errorf("expected available_cash 50.00, got %s", info.Extra["available_cash"])
	}
}

func TestAliyunGetBalanceFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(aliyunBalanceResponse{
			Code:    "InvalidAccessKeyId.NotFound",
			Message: "Specified access key is not found.",
		})
	}))
	defer ts.Close()

	p := newAliyunWithURL("bad-id", "bad-secret", ts.URL)
	_, err := p.GetBalance(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for invalid access key")
	}
}

func TestAliyunGetPackagesSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("Action")
		if action == "QueryResourcePackageInstances" {
			json.NewEncoder(w).Encode(aliyunPackagesResponse{
				Code: "Success",
				Data: struct {
					PageNum    int `json:"PageNum"`
					PageSize   int `json:"PageSize"`
					TotalCount int `json:"TotalCount"`
					Instances  struct {
						Instance []aliyunPackageInstance `json:"Instance"`
					} `json:"Instances"`
				}{
					Instances: struct {
						Instance []aliyunPackageInstance `json:"Instance"`
					}{
						Instance: []aliyunPackageInstance{
							{
								ProductCode:         "cdn",
								PackageType:         "CDN流量包",
								TotalAmount:         "1024",
								TotalAmountUnit:     "GB",
								RemainingAmount:     "512",
								RemainingAmountUnit: "GB",
								Remark:              "中国内地",
								Status:              "Available",
							},
						},
					},
				},
			})
		}
	}))
	defer ts.Close()

	p := newAliyunWithURL("test-id", "test-secret", ts.URL)
	pkgs, err := p.GetPackages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].RemainingAmount != "512" {
		t.Errorf("expected remaining 512, got %s", pkgs[0].RemainingAmount)
	}
}

func TestAliyunSignIncludesRequiredParams(t *testing.T) {
	params := map[string]string{
		"Action":      "QueryAccountBalance",
		"AccessKeyId": "test-id",
	}
	signAliyun(params, "test-secret")

	required := []string{"SignatureMethod", "SignatureVersion", "SignatureNonce", "Timestamp", "Format", "Version", "Signature"}
	for _, r := range required {
		if params[r] == "" {
			t.Errorf("missing required param: %s", r)
		}
	}
	if params["SignatureMethod"] != "HMAC-SHA1" {
		t.Errorf("expected HMAC-SHA1 signature method")
	}
	if params["SignatureVersion"] != "1.0" {
		t.Errorf("expected 1.0 signature version")
	}
	if params["Format"] != "JSON" {
		t.Errorf("expected JSON format")
	}
}
