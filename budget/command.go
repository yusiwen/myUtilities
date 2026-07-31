package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	corebudget "github.com/yusiwen/myUtilities/core/budget"
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

func (o *BalanceOptions) Run() error {
	cfg, err := corebudget.LoadConfig("")
	if err != nil {
		return err
	}

	ctx := context.Background()

	if o.Provider != "" {
		info, err := corebudget.FetchBalance(ctx, o.Provider, o.Key, cfg)
		if err != nil {
			return err
		}
		printBalance(*info)
		return nil
	}
	return o.queryAll(ctx, cfg)
}

func (o *BalanceOptions) queryAll(ctx context.Context, cfg *corebudget.Config) error {
	allNames := corebudget.AllConfiguredProviders(cfg, o.Key)
	if len(allNames) == 0 {
		path, _ := corebudget.DefaultConfigPath()
		return fmt.Errorf(
			"no configured providers found\nCreate %s with your API keys:\n"+
				`  {"providers": {"deepseek": {"api_key": "sk-xxx"}, "openrouter": {"api_key": "sk-or-v1-xxx"}}}`,
			path,
		)
	}

	var errs []string
	for _, name := range allNames {
		info, err := corebudget.FetchBalance(ctx, name, o.Key, cfg)
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

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux, "")
	fmt.Printf("Budget server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

// RegisterHandlers registers the budget API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux, configPath string) {
	corebudget.RegisterHandlers(mux, configPath, corebudget.DebugLog)
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
