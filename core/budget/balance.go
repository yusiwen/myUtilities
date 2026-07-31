package budget

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yusiwen/myUtilities/core/budget/providers"
)

type BalanceResult struct {
	providers.BalanceInfo
	Error string `json:"error,omitempty"`
}

type packageProvider interface {
	GetPackages(ctx context.Context) ([]providers.PackageInstance, error)
}

// CreateProvider builds a provider instance for the given name.
func CreateProvider(name string, pc *ProviderConfig) providers.Provider {
	switch name {
	case "deepseek":
		return providers.NewDeepSeek()
	case "openrouter":
		return providers.NewOpenRouter()
	case "aliyun":
		if pc != nil && pc.AccessKeyID != "" && pc.AccessKeySecret != "" {
			return providers.NewAliyun(pc.AccessKeyID, pc.AccessKeySecret)
		}
		return nil
	default:
		return nil
	}
}

func GetProviderConfig(name string, cfg *Config) *ProviderConfig {
	if cfg == nil || cfg.Providers == nil {
		return nil
	}
	if pc, ok := cfg.Providers[name]; ok {
		return &pc
	}
	return nil
}

func AllConfiguredProviders(cfg *Config, flagKey string) []string {
	if flagKey != "" {
		return []string{"deepseek", "openrouter"}
	}
	if cfg == nil || cfg.Providers == nil || len(cfg.Providers) == 0 {
		return nil
	}
	var names []string
	for name := range cfg.Providers {
		if name == "deepseek" || name == "openrouter" || name == "aliyun" {
			names = append(names, name)
		}
	}
	return names
}

func applyTopUpURL(info *providers.BalanceInfo, pc *ProviderConfig) {
	if pc != nil && pc.TopUpURL != "" {
		if info.Extra == nil {
			info.Extra = make(map[string]string)
		}
		info.Extra["top_up_url"] = pc.TopUpURL
	}
}

func fetchPackages(ctx context.Context, info *providers.BalanceInfo, p providers.Provider) {
	if pp, ok := p.(packageProvider); ok {
		pkgs, err := pp.GetPackages(ctx)
		if err == nil && len(pkgs) > 0 {
			data, _ := json.Marshal(pkgs)
			if info.Extra == nil {
				info.Extra = make(map[string]string)
			}
			info.Extra["packages"] = string(data)
		}
	}
}

// FetchBalance queries a single provider's balance.
func FetchBalance(ctx context.Context, name, flagKey string, cfg *Config) (*providers.BalanceInfo, error) {
	pc := GetProviderConfig(name, cfg)
	if name == "aliyun" {
		p := CreateProvider(name, pc)
		if p == nil {
			return nil, fmt.Errorf("aliyun: access_key_id and access_key_secret required in config")
		}
		info, err := p.GetBalance(ctx, "")
		if err != nil {
			return nil, err
		}
		applyTopUpURL(info, pc)
		fetchPackages(ctx, info, p)
		return info, nil
	}

	key, err := ResolveAPIKey(name, flagKey, cfg)
	if err != nil {
		return nil, err
	}
	p := CreateProvider(name, nil)
	if p == nil {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	info, err := p.GetBalance(ctx, key)
	if err != nil {
		return nil, err
	}
	applyTopUpURL(info, pc)
	return info, nil
}

// FetchAllBalances queries all configured providers concurrently.
func FetchAllBalances(ctx context.Context, flagKey string, cfg *Config, debugf func(string, ...interface{})) []BalanceResult {
	allNames := AllConfiguredProviders(cfg, flagKey)
	if len(allNames) == 0 {
		allNames = []string{"deepseek", "openrouter"}
	}
	debugf("fetchBalances: start, names=%v", allNames)

	type indexedResult struct {
		name string
		r    BalanceResult
	}
	ch := make(chan indexedResult, len(allNames))
	for _, name := range allNames {
		go func(name string) {
			debugf("fetchBalances: goroutine [%s] start", name)
			pc := GetProviderConfig(name, cfg)

			var key string
			var p providers.Provider
			if name == "aliyun" {
				p = CreateProvider(name, pc)
				if p == nil {
					ch <- indexedResult{name, BalanceResult{
						BalanceInfo: providers.BalanceInfo{Provider: name},
						Error:       "aliyun: access_key_id and access_key_secret required in config",
					}}
					return
				}
			} else {
				var err error
				key, err = ResolveAPIKey(name, flagKey, cfg)
				if err != nil {
					debugf("fetchBalances: goroutine [%s] resolveAPIKey failed: %v", name, err)
					ch <- indexedResult{name, BalanceResult{
						BalanceInfo: providers.BalanceInfo{Provider: name},
						Error:       err.Error(),
					}}
					return
				}
				p = CreateProvider(name, nil)
			}
			if p == nil {
				debugf("fetchBalances: goroutine [%s] unknown provider", name)
				ch <- indexedResult{name, BalanceResult{
					BalanceInfo: providers.BalanceInfo{Provider: name},
					Error:       fmt.Sprintf("unknown provider: %s", name),
				}}
				return
			}
			info, err := p.GetBalance(ctx, key)
			if err != nil {
				debugf("fetchBalances: goroutine [%s] GetBalance failed: %v", name, err)
				ch <- indexedResult{name, BalanceResult{
					BalanceInfo: providers.BalanceInfo{Provider: name},
					Error:       err.Error(),
				}}
				return
			}
			applyTopUpURL(info, pc)
			if name == "aliyun" {
				fetchPackages(ctx, info, p)
			}
			ch <- indexedResult{name, BalanceResult{BalanceInfo: *info}}
		}(name)
	}

	var results []BalanceResult
	for i := range allNames {
		r := <-ch
		debugf("fetchBalances: received %d/%d (%s)", i+1, len(allNames), r.name)
		if r.r.Provider != "" {
			results = append(results, r.r)
		}
	}
	debugf("fetchBalances: done, %d results", len(results))
	return results
}
