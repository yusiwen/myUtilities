package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeepSeekGetBalanceSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/user/balance" {
			t.Errorf("expected /user/balance, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(deepseekBalanceResponse{
			IsAvailable: true,
			BalanceInfos: []struct {
				Currency        string `json:"currency"`
				TotalBalance    string `json:"total_balance"`
				GrantedBalance  string `json:"granted_balance"`
				ToppedUpBalance string `json:"topped_up_balance"`
			}{
				{
					Currency:        "CNY",
					TotalBalance:    "120.50",
					GrantedBalance:  "20.50",
					ToppedUpBalance: "100.00",
				},
			},
		})
	}))
	defer ts.Close()

	p := newDeepSeekWithURL(ts.URL)
	info, err := p.GetBalance(context.Background(), "fake-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Total != 120.50 {
		t.Errorf("expected total 120.50, got %f", info.Total)
	}
	if info.Currency != "CNY" {
		t.Errorf("expected CNY, got %s", info.Currency)
	}
	if !info.IsAvailable {
		t.Errorf("expected available true")
	}
	if info.Extra["granted_balance"] != "20.50" {
		t.Errorf("expected granted 20.50, got %s", info.Extra["granted_balance"])
	}
}

func TestDeepSeekAuthFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	p := newDeepSeekWithURL(ts.URL)
	_, err := p.GetBalance(context.Background(), "bad-key")
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestDeepSeekServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := newDeepSeekWithURL(ts.URL)
	_, err := p.GetBalance(context.Background(), "fake-key")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestOpenRouterCreditsSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/credits" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(creditsResponse{
			Data: creditsData{
				TotalCredits: 50.0,
				TotalUsage:   23.45,
			},
		})
	}))
	defer ts.Close()

	p := newOpenRouterWithURL(ts.URL)
	info, err := p.GetBalance(context.Background(), "fake-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Total != 50.0 {
		t.Errorf("expected total 50.0, got %f", info.Total)
	}
	if info.Used != 23.45 {
		t.Errorf("expected used 23.45, got %f", info.Used)
	}
	if info.Remaining != 26.55 {
		t.Errorf("expected remaining 26.55, got %f", info.Remaining)
	}
}

func TestOpenRouterCreditsFallbackToKeyInfo(t *testing.T) {
	limit := 10.0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/credits" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/v1/auth/key" {
			json.NewEncoder(w).Encode(keyResponse{
				Data: keyData{
					Usage: 3.5,
					Limit: &limit,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	p := newOpenRouterWithURL(ts.URL)
	info, err := p.GetBalance(context.Background(), "fake-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Used != 3.5 {
		t.Errorf("expected used 3.5, got %f", info.Used)
	}
	if info.Remaining != 6.5 {
		t.Errorf("expected remaining 6.5, got %f", info.Remaining)
	}
}

func TestOpenRouterKeyInfoNoLimit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/credits" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/v1/auth/key" {
			json.NewEncoder(w).Encode(keyResponse{
				Data: keyData{
					Usage: 5.0,
					Limit: nil,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	p := newOpenRouterWithURL(ts.URL)
	info, err := p.GetBalance(context.Background(), "fake-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Total != 0 {
		t.Errorf("expected total 0 when no limit, got %f", info.Total)
	}
	if info.Remaining != -1 {
		t.Errorf("expected remaining -1 (unknown), got %f", info.Remaining)
	}
}

func TestOpenRouterBothEndpointsFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	p := newOpenRouterWithURL(ts.URL)
	_, err := p.GetBalance(context.Background(), "fake-key")
	if err == nil {
		t.Fatal("expected error when both endpoints fail")
	}
}
