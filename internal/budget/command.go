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

	corebudget "github.com/yusiwen/myUtilities/internal/core/budget"
	"github.com/yusiwen/myUtilities/internal/core/budget/providers"
	"github.com/yusiwen/myUtilities/internal/core/httpserver"
)

type Options struct {
	Balance BalanceOptions `cmd:"" name:"balance" aliases:"b" help:"Query API usage balance."`
	Status  StatusOptions  `cmd:"" name:"status" aliases:"s" help:"Check whether the DeepSeek service is operational."`
	Serve   ServeOptions   `cmd:"" name:"serve" help:"Start Budget HTTP server."`
}

type BalanceOptions struct {
	Provider string `help:"Provider name (deepseek, openrouter). Leave empty to query all configured providers." short:"p"`
	Key      string `help:"API key override." name:"key" short:"k"`
}

type ServeOptions struct {
	Port      int    `help:"Port to listen on." default:"8095"`
	StatusURL string `help:"Status widget URL override (defaults to the official DeepSeek endpoint)." name:"status-url"`
}

type StatusOptions struct {
	URL   string `help:"Status widget URL (defaults to the official DeepSeek endpoint)." name:"url"`
	JSON  bool   `short:"j" name:"json" help:"Print the raw status result as JSON."`
	Check bool   `help:"Exit non-zero unless DeepSeek is fully operational."`
}

func (o *StatusOptions) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := corebudget.NewStatusClient(o.URL).Status(ctx)
	if err != nil {
		return err
	}

	if o.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
	} else {
		printStatus(result)
	}

	if o.Check && !result.Health.OK {
		return fmt.Errorf("deepseek: %s — %s", result.Health.Headline, result.Health.Detail)
	}
	return nil
}

func printStatus(res *corebudget.StatusResult) {
	h := res.Health
	fmt.Printf("\nDeepSeek Service Status\n")
	fmt.Printf("  Status:     %s (%s)\n", orDash(h.OverallStatus), h.Level)
	fmt.Printf("  Verdict:    %s\n", h.Headline)
	if h.Detail != "" {
		fmt.Printf("  Detail:     %s\n", h.Detail)
	}
	fmt.Printf("  Checked:    %s\n", h.CheckedAt.Local().Format("2006-01-02 15:04:05 MST"))
	if !h.GeneratedAt.IsZero() {
		note := ""
		if h.Stale {
			note = fmt.Sprintf(" (stale: %s old, upstream max_stale_seconds=%d)", formatAge(h.DataAgeSeconds), h.MaxStaleSeconds)
		}
		fmt.Printf("  Snapshot:   %s UTC%s\n", h.GeneratedAt.UTC().Format("2006-01-02 15:04:05"), note)
	}
	if res.ValidatedAt != "" {
		fmt.Printf("  Validated:  %s\n", res.ValidatedAt)
	}

	printIncidents(res.Summary.OngoingIncidents)
	printMaintenances("In-progress maintenance", res.Summary.InProgressMaintenances)
	printMaintenances("Scheduled maintenance", res.Summary.ScheduledMaintenances)
}

func printIncidents(incidents []corebudget.StatusIncident) {
	if len(incidents) == 0 {
		return
	}
	fmt.Printf("\n  Ongoing incidents:\n")
	for _, inc := range incidents {
		title := inc.Name
		if title == "" {
			title = "Unnamed incident"
		}
		var tags []string
		if inc.Impact != "" {
			tags = append(tags, inc.Impact)
		}
		if inc.Phase != "" {
			tags = append(tags, inc.Phase)
		}
		if len(tags) > 0 {
			fmt.Printf("    - [%s] %s\n", strings.Join(tags, " / "), title)
		} else {
			fmt.Printf("    - %s\n", title)
		}
		if msg := strings.TrimSpace(inc.LastUpdate.Message); msg != "" {
			fmt.Printf("      latest: %s\n", msg)
		}
		if len(inc.AffectedComponents) > 0 {
			parts := make([]string, 0, len(inc.AffectedComponents))
			for _, c := range inc.AffectedComponents {
				label := c.Name
				if label == "" {
					label = c.ID
				}
				if c.Status != "" {
					label += " (" + c.Status + ")"
				}
				parts = append(parts, label)
			}
			fmt.Printf("      affected: %s\n", strings.Join(parts, ", "))
		}
	}
}

func printMaintenances(title string, items []corebudget.StatusMaintenance) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("\n  %s:\n", title)
	for _, m := range items {
		name := m.Name
		if name == "" {
			name = "Unnamed maintenance"
		}
		if m.Status != "" {
			fmt.Printf("    - %s [%s]\n", name, m.Status)
		} else {
			fmt.Printf("    - %s\n", name)
		}
		start, end := m.Window()
		if !start.IsZero() || !end.IsZero() {
			fmt.Printf("      %s → %s\n", formatStatusTime(start), formatStatusTime(end))
		}
		if desc := strings.TrimSpace(m.Description); desc != "" {
			fmt.Printf("      %s\n", desc)
		}
	}
}

func formatStatusTime(t corebudget.StatusTime) string {
	if t.IsZero() {
		return "?"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatAge(seconds int64) string {
	if seconds < 0 {
		return "unknown"
	}
	d := time.Duration(seconds) * time.Second
	if d >= 24*time.Hour {
		days := int(d / (24 * time.Hour))
		hours := int((d % (24 * time.Hour)) / time.Hour)
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if d >= time.Hour {
		return d.Round(time.Minute).String()
	}
	return d.Round(time.Second).String()
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
	RegisterHandlers(mux, "", o.StatusURL)
	fmt.Printf("Budget server listening on :%d\n", o.Port)
	return httpserver.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

// RegisterHandlers registers the budget API routes on the given mux. An empty
// statusURL uses the official DeepSeek status endpoint.
func RegisterHandlers(mux *http.ServeMux, configPath, statusURL string) {
	corebudget.RegisterHandlers(mux, configPath, statusURL, corebudget.DebugLog)
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
