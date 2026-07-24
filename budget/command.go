package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yusiwen/myUtilities/core/budget/providers"
)

type Options struct {
	Balance BalanceOptions `cmd:"" name:"balance" aliases:"b" help:"Query API usage balance."`
	Serve   ServeOptions   `cmd:"" name:"serve" help:"Start Budget HTTP server."`
}

type BalanceOptions struct {
	Provider string `help:"Provider name (deepseek, openrouter). Leave empty to query all configured providers." short:"p"`
	Key      string `help:"API key override." name:"key" short:"k"`
}

type ServeOptions struct {
	Port int `help:"Port to listen on." default:"8095"`
}

type balanceResult struct {
	providers.BalanceInfo
	Error string `json:"error,omitempty"`
}

type packageProvider interface {
	GetPackages(ctx context.Context) ([]providers.PackageInstance, error)
}

func (o *BalanceOptions) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx := context.Background()

	if o.Provider != "" {
		return queryOne(ctx, o.Provider, o.Key, cfg)
	}
	return queryAll(ctx, o.Key, cfg)
}

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("Budget server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func RegisterHandlers(mux *http.ServeMux) {
	debugLog("RegisterHandlers: registering GET /api/budget/balance")
	mux.HandleFunc("/api/budget/balance", handleBalance)
}

func handleBalance(w http.ResponseWriter, r *http.Request) {
	debugLog("handleBalance: called, method=%s", r.Method)
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		debugLog("handleBalance: loadConfig failed: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	debugLog("handleBalance: config loaded, providers=%d, debug_log=%v", len(cfg.Providers), cfg.DebugLog)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	results := fetchBalances(ctx, "", cfg)
	elapsed := time.Since(start)

	debugLog("handleBalance: fetchBalances done, %d results, elapsed=%v", len(results), elapsed)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
	debugLog("handleBalance: response written, done")
}

func fetchBalances(ctx context.Context, flagKey string, cfg *BudgetConfig) []balanceResult {
	allNames := allConfiguredProviders(cfg, flagKey)
	if len(allNames) == 0 {
		allNames = []string{"deepseek", "openrouter"}
	}
	logd(cfg, "fetchBalances: start, names=%v", allNames)

	type indexedResult struct {
		name string
		r    balanceResult
	}
	ch := make(chan indexedResult, len(allNames))
	for _, name := range allNames {
		go func(name string) {
			logd(cfg, "fetchBalances: goroutine [%s] start", name)
			pc := getProviderConfig(name, cfg)

			var key string
			var p providers.Provider
			if name == "aliyun" {
				p = createProvider(name, pc)
				if p == nil {
					ch <- indexedResult{name, balanceResult{
						BalanceInfo: providers.BalanceInfo{Provider: name},
						Error:       fmt.Sprintf("aliyun: access_key_id and access_key_secret required in config"),
					}}
					return
				}
			} else {
				var err error
				key, err = resolveAPIKey(name, flagKey, cfg)
				if err != nil {
					logd(cfg, "fetchBalances: goroutine [%s] resolveAPIKey failed: %v", name, err)
					ch <- indexedResult{name, balanceResult{
						BalanceInfo: providers.BalanceInfo{Provider: name},
						Error:       err.Error(),
					}}
					return
				}
				logd(cfg, "fetchBalances: goroutine [%s] key resolved", name)
				p = createProvider(name, nil)
			}
			if p == nil {
				logd(cfg, "fetchBalances: goroutine [%s] unknown provider", name)
				ch <- indexedResult{name, balanceResult{
					BalanceInfo: providers.BalanceInfo{Provider: name},
					Error:       fmt.Sprintf("unknown provider: %s", name),
				}}
				return
			}
			logd(cfg, "fetchBalances: goroutine [%s] calling GetBalance", name)
			info, err := p.GetBalance(ctx, key)
			if err != nil {
				logd(cfg, "fetchBalances: goroutine [%s] GetBalance failed: %v", name, err)
				ch <- indexedResult{name, balanceResult{
					BalanceInfo: providers.BalanceInfo{Provider: name},
					Error:       err.Error(),
				}}
				return
			}
			logd(cfg, "fetchBalances: goroutine [%s] GetBalance ok, total=%.2f", name, info.Total)
			if name == "aliyun" {
				if pp, ok := p.(packageProvider); ok {
					pkgs, pkgErr := pp.GetPackages(ctx)
					if pkgErr != nil {
						logd(cfg, "fetchBalances: goroutine [%s] GetPackages failed: %v", name, pkgErr)
					} else {
						logd(cfg, "fetchBalances: goroutine [%s] GetPackages ok, %d packages", name, len(pkgs))
						if len(pkgs) > 0 {
							data, _ := json.Marshal(pkgs)
							info.Extra["packages"] = string(data)
						}
					}
				}
			}
			ch <- indexedResult{name, balanceResult{BalanceInfo: *info}}
		}(name)
	}

	var results []balanceResult
	for i := range allNames {
		r := <-ch
		logd(cfg, "fetchBalances: received %d/%d (%s)", i+1, len(allNames), r.name)
		if r.r.Provider != "" {
			results = append(results, r.r)
		}
	}
	logd(cfg, "fetchBalances: done, %d results", len(results))
	return results
}

func logd(cfg *BudgetConfig, format string, args ...interface{}) {
	if cfg != nil && cfg.DebugLog {
		debugLog(format, args...)
	}
}

func queryOne(ctx context.Context, name string, flagKey string, cfg *BudgetConfig) error {
	pc := getProviderConfig(name, cfg)
	if name == "aliyun" {
		p := createProvider(name, pc)
		if p == nil {
			return fmt.Errorf("aliyun: access_key_id and access_key_secret required in config")
		}
		info, err := p.GetBalance(ctx, "")
		if err != nil {
			return err
		}
		fetchPackages(ctx, info, p)
		printBalance(*info)
	} else {
		key, err := resolveAPIKey(name, flagKey, cfg)
		if err != nil {
			return err
		}
		p := createProvider(name, nil)
		if p == nil {
			return fmt.Errorf("unknown provider: %s", name)
		}
		info, err := p.GetBalance(ctx, key)
		if err != nil {
			return err
		}
		printBalance(*info)
	}
	return nil
}

func queryAll(ctx context.Context, flagKey string, cfg *BudgetConfig) error {
	allNames := allConfiguredProviders(cfg, flagKey)
	if len(allNames) == 0 {
		path, _ := configFilePath()
		return fmt.Errorf(
			"no configured providers found\nCreate %s with your API keys:\n"+
				`  {"providers": {"deepseek": {"api_key": "sk-xxx"}, "openrouter": {"api_key": "sk-or-v1-xxx"}}}`,
			path,
		)
	}

	var errs []string
	for _, name := range allNames {
		pc := getProviderConfig(name, cfg)
		var info *providers.BalanceInfo
		var err error
		if name == "aliyun" {
			p := createProvider(name, pc)
			if p == nil {
				errs = append(errs, fmt.Sprintf("%s: access_key_id and access_key_secret required", name))
				continue
			}
			info, err = p.GetBalance(ctx, "")
			if err == nil {
				fetchPackages(ctx, info, p)
			}
		} else {
			key, keyErr := resolveAPIKey(name, flagKey, cfg)
			if keyErr != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", name, keyErr))
				continue
			}
			p := createProvider(name, nil)
			if p == nil {
				continue
			}
			info, err = p.GetBalance(ctx, key)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		printBalance(*info)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}

func fetchPackages(ctx context.Context, info *providers.BalanceInfo, p providers.Provider) {
	if pp, ok := p.(packageProvider); ok {
		pkgs, err := pp.GetPackages(ctx)
		if err == nil && len(pkgs) > 0 {
			data, _ := json.Marshal(pkgs)
			info.Extra["packages"] = string(data)
		}
	}
}

func createProvider(name string, pc *ProviderConfig) providers.Provider {
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

func getProviderConfig(name string, cfg *BudgetConfig) *ProviderConfig {
	if cfg == nil || cfg.Providers == nil {
		return nil
	}
	if pc, ok := cfg.Providers[name]; ok {
		return &pc
	}
	return nil
}

func allConfiguredProviders(cfg *BudgetConfig, flagKey string) []string {
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

func printBalance(info providers.BalanceInfo) {
	cur := info.Currency
	switch info.Provider {
	case "deepseek":
		fmt.Printf("\nDeepSeek Balance:\n")
		fmt.Printf("  Total:        %s%.2f", currencySymbol(cur), info.Total)
		if info.Extra["topped_up_balance"] != "" && info.Extra["granted_balance"] != "" {
			fmt.Printf(" (topped_up: %s + granted: %s)", info.Extra["topped_up_balance"], info.Extra["granted_balance"])
		}
		fmt.Println()
		status := "yes"
		if !info.IsAvailable {
			status = "no"
		}
		fmt.Printf("  Available:    %s\n", status)

	case "openrouter":
		fmt.Printf("\nOpenRouter Credits:\n")
		if info.Total > 0 {
			fmt.Printf("  Purchased:    %s%.2f\n", currencySymbol(cur), info.Total)
		}
		fmt.Printf("  Used:         %s%.2f\n", currencySymbol(cur), info.Used)
		if info.Remaining >= 0 {
			fmt.Printf("  Remaining:    %s%.2f\n", currencySymbol(cur), info.Remaining)
		} else {
			fmt.Printf("  Remaining:    unknown (no key limit)\n")
		}

	case "aliyun":
		fmt.Printf("\nAliyun Balance:\n")
		fmt.Printf("  Cash Balance: %s%s\n", currencySymbol(cur), info.Extra["available_cash"])
		fmt.Printf("  Credit:       %s%s\n", currencySymbol(cur), info.Extra["credit_amount"])
		fmt.Printf("  Available:    %s%.2f\n", currencySymbol(cur), info.Total)
		if info.Extra["packages"] != "" {
			var pkgs []providers.PackageInstance
			if err := json.Unmarshal([]byte(info.Extra["packages"]), &pkgs); err == nil && len(pkgs) > 0 {
				fmt.Printf("\n  Packages:\n")
				for _, pkg := range pkgs {
					pct := pctStr(pkg.RemainingAmount, pkg.TotalAmount, pkg.RemainingUnit, pkg.TotalUnit)
					fmt.Printf("    %-24s %s%s / %s%s (%s)   expires %s\n",
						truncate(pkg.PackageType, 24),
						pkg.RemainingAmount, pkg.RemainingUnit,
						pkg.TotalAmount, pkg.TotalUnit,
						pct,
						pkg.ExpiryTime[:min(10, len(pkg.ExpiryTime))])
					if pkg.Remark != "" {
						fmt.Printf("      %s\n", pkg.Remark)
					}
				}
			}
		}
	}
}

func pctStr(remaining, total, rUnit, tUnit string) string {
	if rUnit != tUnit {
		return "-"
	}
	r, _ := strconv.ParseFloat(remaining, 64)
	t, _ := strconv.ParseFloat(total, 64)
	if t <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", r/t*100)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func currencySymbol(code string) string {
	switch code {
	case "CNY":
		return "¥"
	case "USD":
		return "$"
	default:
		return ""
	}
}

func init() {
	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}
}

var noColor bool
