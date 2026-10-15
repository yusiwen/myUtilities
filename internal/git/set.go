package git

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/core/config"
	coregit "github.com/yusiwen/myUtilities/internal/core/git"
	"github.com/yusiwen/myUtilities/internal/core/llm"
)

type gitSetter struct{}

func init() {
	config.Register(&gitSetter{})
}

func (s *gitSetter) Name() string {
	return "git"
}

func (s *gitSetter) Set(args []string) error {
	opts := &GitSetCmd{}
	parser, err := kong.New(opts,
		kong.Name("mu set git"),
		kong.Description("Update git module configuration (git-config.json)."))
	if err != nil {
		return err
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		return err
	}
	return ctx.Run()
}

type GitSetCmd struct {
	Provider ProviderCmd     `cmd:"" name:"provider" help:"Manage LLM providers."`
	Commit   CommitModuleCmd `cmd:"" name:"commit" help:"Configure commit module settings."`
	Review   ReviewModuleCmd `cmd:"" name:"review" help:"Configure review module settings."`
}

/* ─── Provider Subcommands ─── */

type ProviderCmd struct {
	Add  ProviderAddCmd  `cmd:"" name:"add" help:"Add a new LLM provider."`
	Rm   ProviderRmCmd   `cmd:"" name:"rm" help:"Remove an LLM provider."`
	List ProviderListCmd `cmd:"" name:"list" help:"List all configured providers."`
}

type ProviderAddCmd struct {
	Name      string `help:"Provider name (e.g. default, advanced)." required:""`
	BaseURL   string `help:"Base URL of the AI service. Required for a new name; omit to inherit it from the shared catalog and override other fields only."`
	APIKey    string `help:"API key for the AI service. Required for a new name; omit to inherit it from the shared catalog."`
	Model     string `help:"Model name (e.g. gpt-4o-mini, deepseek-chat)."`
	Path      string `name:"config" help:"Config file path. Default: ~/.config/mu/git-config.json"`
	LLMConfig string `name:"llm-config" help:"Shared provider catalog path. Default: ~/.config/mu/llm-config.json"`
}

func (o *ProviderAddCmd) Run() error {
	gc, err := coregit.LoadGitConfigFrom(o.Path)
	if err != nil {
		return err
	}

	if err := llm.ValidateProviderAdd(&gc.Catalog, o.Name, o.BaseURL, o.APIKey, o.LLMConfig); err != nil {
		return err
	}
	if err := gc.Add(llm.Provider{
		Name:    o.Name,
		BaseURL: o.BaseURL,
		APIKey:  o.APIKey,
		Model:   o.Model,
	}); err != nil {
		return err
	}
	return coregit.SaveGitConfigTo(o.Path, gc)
}

type ProviderRmCmd struct {
	Name      string `help:"Provider name to remove." required:""`
	Force     bool   `help:"Remove the provider even while commit or review still references it."`
	Path      string `name:"config" help:"Config file path. Default: ~/.config/mu/git-config.json"`
	LLMConfig string `name:"llm-config" help:"Shared provider catalog path. Default: ~/.config/mu/llm-config.json"`
}

func (o *ProviderRmCmd) Run() error {
	gc, err := coregit.LoadGitConfigFrom(o.Path)
	if err != nil {
		return err
	}

	if err := llm.ExplainInheritedProvider(o.Name, &gc.Catalog, o.LLMConfig); err != nil {
		return err
	}
	if err := gc.Remove(o.Name, o.Force, gc.References()...); err != nil {
		return err
	}
	return coregit.SaveGitConfigTo(o.Path, gc)
}

type ProviderListCmd struct {
	Path      string `name:"config" help:"Config file path. Default: ~/.config/mu/git-config.json"`
	LLMConfig string `name:"llm-config" help:"Shared provider catalog path. Default: ~/.config/mu/llm-config.json"`
}

func (o *ProviderListCmd) Run() error {
	gc, err := coregit.LoadGitConfigFrom(o.Path)
	if err != nil {
		return err
	}

	catalog, err := llm.EffectiveCatalog(o.LLMConfig, &gc.Catalog)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", llm.GlobalConfigFileName, err)
	}

	if len(catalog.Providers) == 0 {
		fmt.Println("No providers configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Name\tBase URL\tModel")
	fmt.Fprintln(w, "----\t---------\t-----")
	for _, p := range catalog.Providers {
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, p.BaseURL, p.Model)
	}
	w.Flush()

	if inherited := llm.InheritedNames(&gc.Catalog, o.LLMConfig); len(inherited) > 0 {
		fmt.Fprintf(os.Stdout, "\nInherited from %s: %s\n", llm.GlobalConfigFileName, strings.Join(inherited, ", "))
	}

	fmt.Fprintln(os.Stderr, "\nModule references:")
	for _, ref := range gc.References() {
		names := ref.Names.Names()
		if len(names) > 0 {
			fmt.Fprintf(os.Stderr, "  %s → %s\n", ref.Module, strings.Join(names, ", "))
		}
	}
	return nil
}

/* ─── Module Config Subcommands ─── */

type CommitModuleCmd struct {
	Provider  string `help:"Provider name(s) to use, comma-separated for fallback (e.g. 'fast,advanced')."`
	Lang      string `help:"Output language (en, cn)."`
	Path      string `name:"config" help:"Config file path. Default: ~/.config/mu/git-config.json"`
	LLMConfig string `name:"llm-config" help:"Shared provider catalog path. Default: ~/.config/mu/llm-config.json"`
}

func validLang(lang string) bool {
	return lang == "en" || lang == "cn"
}

// resolveModuleProviders validates a comma-separated provider list against the
// merged catalog — a name may come from this module's file or from the shared
// llm-config.json — and returns it ready to store in a module reference.
func resolveModuleProviders(gc *coregit.GitConfig, list, llmConfigPath string) (llm.MultiProvider, error) {
	names, err := llm.ParseProviderList(list)
	if err != nil {
		return nil, err
	}
	catalog, err := llm.EffectiveCatalog(llmConfigPath, &gc.Catalog)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", llm.GlobalConfigFileName, err)
	}
	for _, name := range names {
		if _, err := catalog.Resolve(name); err != nil {
			return nil, err
		}
	}
	return names, nil
}

func (o *CommitModuleCmd) Run() error {
	gc, err := coregit.LoadGitConfigFrom(o.Path)
	if err != nil {
		return err
	}
	if o.Provider != "" {
		names, err := resolveModuleProviders(gc, o.Provider, o.LLMConfig)
		if err != nil {
			return err
		}
		gc.Commit.Provider = names
	}
	if o.Lang != "" {
		if !validLang(o.Lang) {
			return fmt.Errorf("invalid language %q, must be 'en' or 'cn'", o.Lang)
		}
		gc.Commit.Lang = o.Lang
	}
	return coregit.SaveGitConfigTo(o.Path, gc)
}

type ReviewModuleCmd struct {
	Provider      string `help:"Provider name(s) to use, comma-separated for fallback." name:"provider"`
	Lang          string `help:"Output language (en, cn)."`
	ReviewsDir    string `help:"Directory to store review reports." name:"reviews-dir"`
	ScipVersion   string `help:"Set SCIP indexer version override as lang=tag (e.g. go=v0.3.0)." name:"scip-version"`
	ScipVersionRm string `help:"Remove SCIP version override for a language (e.g. go)." name:"scip-version-rm"`
	Path          string `name:"config" help:"Config file path. Default: ~/.config/mu/git-config.json"`
	LLMConfig     string `name:"llm-config" help:"Shared provider catalog path. Default: ~/.config/mu/llm-config.json"`
}

func (o *ReviewModuleCmd) Run() error {
	gc, err := coregit.LoadGitConfigFrom(o.Path)
	if err != nil {
		return err
	}
	if o.Provider != "" {
		names, err := resolveModuleProviders(gc, o.Provider, o.LLMConfig)
		if err != nil {
			return err
		}
		gc.Review.Provider = names
	}
	if o.Lang != "" {
		if !validLang(o.Lang) {
			return fmt.Errorf("invalid language %q, must be 'en' or 'cn'", o.Lang)
		}
		gc.Review.Lang = o.Lang
	}
	if o.ReviewsDir != "" {
		gc.Review.ReviewsDir = o.ReviewsDir
	}
	if o.ScipVersion != "" {
		lang, tag, ok := strings.Cut(o.ScipVersion, "=")
		if !ok || lang == "" || tag == "" {
			return fmt.Errorf("invalid --scip-version %q, expected lang=tag (e.g. go=v0.3.0)", o.ScipVersion)
		}
		if gc.Review.Scip.Versions == nil {
			gc.Review.Scip.Versions = map[string]string{}
		}
		gc.Review.Scip.Versions[lang] = tag
	}
	if o.ScipVersionRm != "" {
		if gc.Review.Scip.Versions != nil {
			delete(gc.Review.Scip.Versions, o.ScipVersionRm)
		}
	}
	return coregit.SaveGitConfigTo(o.Path, gc)
}
