// Package llm exposes the `mu set llm` command family, which edits the shared
// provider catalog (~/.config/mu/llm-config.json) that every module reads.
package llm

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/core/config"
	coregit "github.com/yusiwen/myUtilities/internal/core/git"
	corellm "github.com/yusiwen/myUtilities/internal/core/llm"
)

type llmSetter struct{}

func init() {
	config.Register(&llmSetter{})
}

func (s *llmSetter) Name() string {
	return "llm"
}

func (s *llmSetter) Set(args []string) error {
	opts := &SetCmd{}
	parser, err := kong.New(opts,
		kong.Name("mu set llm"),
		kong.Description("Manage the shared LLM provider catalog (llm-config.json)."))
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
	Provider ProviderCmd `cmd:"" name:"provider" help:"Manage the shared provider catalog."`
}

type ProviderCmd struct {
	Add  ProviderAddCmd  `cmd:"" name:"add" help:"Add a provider every module can reference."`
	Rm   ProviderRmCmd   `cmd:"" name:"rm" help:"Remove a shared provider."`
	List ProviderListCmd `cmd:"" name:"list" help:"List shared providers and the modules referencing them."`
}

type ProviderAddCmd struct {
	Name    string `help:"Provider name (e.g. fast, backup)." required:""`
	BaseURL string `help:"Base URL of the AI service." required:""`
	APIKey  string `help:"API key for the AI service." required:""`
	Model   string `help:"Model name (e.g. gpt-4o-mini, deepseek-chat)."`
	Path    string `name:"llm-config" help:"Shared catalog path. Default: ~/.config/mu/llm-config.json"`
}

func (o *ProviderAddCmd) Run() error {
	catalog, err := corellm.LoadGlobalCatalog(o.Path)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", corellm.GlobalConfigFileName, err)
	}

	if err := catalog.Add(corellm.Provider{
		Name:    o.Name,
		BaseURL: o.BaseURL,
		APIKey:  o.APIKey,
		Model:   o.Model,
	}); err != nil {
		return err
	}
	if err := corellm.SaveGlobalCatalog(o.Path, catalog); err != nil {
		return err
	}
	fmt.Printf("Added provider %q to %s\n", o.Name, corellm.GlobalConfigFileName)
	return nil
}

type ProviderRmCmd struct {
	Name  string `arg:"" help:"Provider name to remove."`
	Force bool   `help:"Remove the provider even while a module still references it."`
	Path  string `name:"llm-config" help:"Shared catalog path. Default: ~/.config/mu/llm-config.json"`
}

func (o *ProviderRmCmd) Run() error {
	catalog, err := corellm.LoadGlobalCatalog(o.Path)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", corellm.GlobalConfigFileName, err)
	}

	refs, err := moduleReferences()
	if err != nil {
		return err
	}
	if err := catalog.Remove(o.Name, o.Force, refs...); err != nil {
		return err
	}
	if err := corellm.SaveGlobalCatalog(o.Path, catalog); err != nil {
		return err
	}
	fmt.Printf("Removed provider %q from %s\n", o.Name, corellm.GlobalConfigFileName)
	return nil
}

type ProviderListCmd struct {
	Path string `name:"llm-config" help:"Shared catalog path. Default: ~/.config/mu/llm-config.json"`
}

func (o *ProviderListCmd) Run() error {
	catalog, err := corellm.LoadGlobalCatalog(o.Path)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", corellm.GlobalConfigFileName, err)
	}

	if len(catalog.Providers) == 0 {
		fmt.Printf("No providers configured in %s.\n", corellm.GlobalConfigFileName)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Name\tBase URL\tModel")
	fmt.Fprintln(w, "----\t---------\t-----")
	for _, p := range catalog.Providers {
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, p.BaseURL, p.Model)
	}
	w.Flush()

	refs, err := moduleReferences()
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		fmt.Fprintln(os.Stderr, "\nModules depending on this catalog:")
		for _, ref := range refs {
			fmt.Fprintf(os.Stderr, "  %s → %s\n", ref.Module, strings.Join(ref.Names.Names(), ", "))
		}
	}
	return nil
}

// moduleReferences collects the provider references the modules cannot satisfy
// from their own config, i.e. the ones that need the shared catalog. It reads the
// default module config locations: a per-module --config override is invisible
// from here, which is exactly why `rm` also takes --force.
func moduleReferences() ([]corellm.Reference, error) {
	var refs []corellm.Reference

	askCfg, err := corellm.LoadConfig("ask")
	if err != nil {
		return nil, fmt.Errorf("failed to load ask config: %w", err)
	}
	refs = append(refs, corellm.DependentReferences(&askCfg.Catalog, []corellm.Reference{
		{Module: "ask", Names: askCfg.Provider},
	})...)

	gitCfg, err := coregit.LoadGitConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load git config: %w", err)
	}
	return append(refs, corellm.DependentReferences(&gitCfg.Catalog, gitCfg.References())...), nil
}
