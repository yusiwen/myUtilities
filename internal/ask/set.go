package ask

import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/core/config"
	"github.com/yusiwen/myUtilities/internal/core/llm"
)

type askSetter struct{}

func init() {
	config.Register(&askSetter{})
}

func (s *askSetter) Name() string {
	return "ask"
}

func (s *askSetter) Set(args []string) error {
	opts := &SetCmd{}
	parser, err := kong.New(opts,
		kong.Name("mu set ask"),
		kong.Description("Update ask module configuration (ask-config.json)."))
	if err != nil {
		return err
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		return err
	}
	return ctx.Run()
}

type SetCmd struct {
	Flat     FlatSetCmd   `cmd:"" name:"flat" help:"Update flat (legacy) configuration fields."`
	Provider ProviderCmd  `cmd:"" name:"provider" help:"Manage LLM providers."`
	Module   ModuleSetCmd `cmd:"" name:"module" help:"Update ask module settings."`
}

// defaultAskConfigPath mirrors the default location used by
// llm.LoadConfig/SaveConfig so a custom --config rounds-trips correctly.
const defaultAskConfigPath = "~/.config/mu/ask-config.json"

/* ─── Flat (legacy) fields ─── */

type FlatSetCmd struct {
	BaseURL   string `help:"Base URL of the AI service."`
	Model     string `help:"Model name."`
	APIKey    string `help:"API key for the AI service."`
	SearchKey string `help:"Brave Search API key." name:"search-key"`
	Path      string `name:"config" help:"Config file path. Default: ~/.config/mu/ask-config.json"`
}

func (o *FlatSetCmd) configPath() string {
	if o.Path != "" {
		return o.Path
	}
	return defaultAskConfigPath
}

func (o *FlatSetCmd) Run() error {
	cfg, err := llm.LoadConfigFromPath(o.configPath())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.BaseURL == "" && o.Model == "" && o.APIKey == "" && o.SearchKey == "" {
		return fmt.Errorf("at least one of --base-url, --model, --api-key, --search-key is required")
	}

	if o.BaseURL != "" {
		cfg.BaseURL = o.BaseURL
	}
	if o.Model != "" {
		cfg.Model = o.Model
	}
	if o.APIKey != "" {
		cfg.APIKey = o.APIKey
	}
	if o.SearchKey != "" {
		cfg.SearchAPIKey = o.SearchKey
	}

	if err := llm.SaveConfigToPath(o.configPath(), cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Config saved to %s\n", o.configPath())
	return nil
}

/* ─── Provider management ─── */

type ProviderCmd struct {
	Add  ProviderAddCmd  `cmd:"" name:"add" help:"Add a new LLM provider."`
	Set  ProviderSetCmd  `cmd:"" name:"set" help:"Set provider name(s) for the ask module (comma-separated for fallback)."`
	Rm   ProviderRmCmd   `cmd:"" name:"rm" help:"Remove a named provider."`
	List ProviderListCmd `cmd:"" name:"list" help:"List all configured providers."`
}

type ProviderAddCmd struct {
	Name    string `help:"Provider name (e.g. default, advanced)." required:""`
	BaseURL string `help:"Base URL of the AI service." required:""`
	APIKey  string `help:"API key for the AI service." required:""`
	Model   string `help:"Model name (e.g. gpt-4o-mini, deepseek-chat)."`
}

func (o *ProviderAddCmd) Run() error {
	cfg, err := llm.LoadConfig("ask")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	for _, p := range cfg.Providers {
		if p.Name == o.Name {
			return fmt.Errorf("provider %q already exists", o.Name)
		}
	}
	cfg.Providers = append(cfg.Providers, llm.Provider{
		Name:    o.Name,
		BaseURL: o.BaseURL,
		APIKey:  o.APIKey,
		Model:   o.Model,
	})
	if err := llm.SaveConfig("ask", cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("Added provider %q\n", o.Name)
	return nil
}

type ProviderSetCmd struct {
	Names string `arg:"" help:"Provider name(s), comma-separated for fallback (e.g. 'default,backup')."`
}

func (o *ProviderSetCmd) Run() error {
	cfg, err := llm.LoadConfig("ask")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	names, err := llm.ParseProviderList(o.Names)
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, err := llm.ResolveProvider(cfg, name); err != nil {
			return err
		}
	}
	cfg.Provider = names
	if err := llm.SaveConfig("ask", cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("ask module → provider(s): %s\n", strings.Join(names, ", "))
	return nil
}

type ProviderRmCmd struct {
	Name string `arg:"" help:"Provider name to remove."`
}

func (o *ProviderRmCmd) Run() error {
	cfg, err := llm.LoadConfig("ask")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	idx := -1
	for i, p := range cfg.Providers {
		if p.Name == o.Name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("provider %q not found", o.Name)
	}

	// Check if the ask module references this provider.
	if cfg.Provider.Contains(o.Name) {
		return fmt.Errorf("cannot remove provider %q: module 'ask' is using it", o.Name)
	}

	cfg.Providers = append(cfg.Providers[:idx], cfg.Providers[idx+1:]...)
	if err := llm.SaveConfig("ask", cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("Removed provider %q\n", o.Name)
	return nil
}

type ProviderListCmd struct{}

func (o *ProviderListCmd) Run() error {
	cfg, err := llm.LoadConfig("ask")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.Providers) == 0 {
		fmt.Println("No providers configured.")
		return nil
	}

	fmt.Fprintln(os.Stdout, "Providers:")
	for _, p := range cfg.Providers {
		fmt.Fprintf(os.Stdout, "  %s → %s (model: %s)\n", p.Name, p.BaseURL, p.Model)
	}

	if len(cfg.Provider) > 0 {
		fmt.Fprintf(os.Stderr, "\nModule references: ask → %s\n", strings.Join(cfg.Provider.Names(), ", "))
	}

	return nil
}

/* ─── Module config ─── */

type ModuleSetCmd struct {
	Provider  string `help:"Provider name(s) to use, comma-separated for fallback."`
	SearchKey string `help:"Brave Search API key." name:"search-key"`
	Path      string `name:"config" help:"Config file path. Default: ~/.config/mu/ask-config.json"`
}

func (o *ModuleSetCmd) configPath() string {
	if o.Path != "" {
		return o.Path
	}
	return defaultAskConfigPath
}

func (o *ModuleSetCmd) Run() error {
	cfg, err := llm.LoadConfigFromPath(o.configPath())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.Provider != "" {
		names, err := llm.ParseProviderList(o.Provider)
		if err != nil {
			return err
		}
		for _, name := range names {
			if _, err := llm.ResolveProvider(cfg, name); err != nil {
				return err
			}
		}
		cfg.Provider = names
	}

	if o.SearchKey != "" {
		cfg.SearchAPIKey = o.SearchKey
	}

	return llm.SaveConfigToPath(o.configPath(), cfg)
}
